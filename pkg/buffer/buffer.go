package buffer

import (
	"encoding/binary"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gammazero/deque"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

const (
	maxSequenceNumber = 1 << 16

	reportDelta = 1e9

	minBufferSize = 100000
)

type PendingPackets struct {
	arrivalTime int64
	packet      []byte
}

type ExtPacket struct {
	Head     bool
	Cycle    uint32
	Arrival  int64
	Packet   rtp.Packet
	Payload  any
	KeyFrame bool
}

// Buffer contains all packets
type Buffer struct {
	mu             sync.Mutex
	bucket         *Bucket
	nacker         *nackQueue
	videoPool      *sync.Pool
	audioPool      *sync.Pool
	codecType      webrtc.RTPCodecType
	extPackets     deque.Deque[*ExtPacket]
	pendingPackets []PendingPackets
	closeOnce      sync.Once
	mediaSSRC      uint32
	clockRate      uint32
	maxBitrate     uint64
	lastReport     int64
	twccExt        uint8
	audioExt       uint8
	bound          bool
	closed         atomicBool
	mime           string

	// supported feedbacks
	remb       bool
	nack       bool
	twcc       bool
	audioLevel bool

	minPacketProbe       int
	lastPacketRead       int
	maxTemporalLayer     int32
	bitrate              uint64
	bitrateHelper        uint64
	lastSRNTPTime        uint64
	lastSRRTPTime        uint32
	lastSenderReportRecv int64 // Represents wall clock of the most recent sender report arrival
	baseSequenceNumber   uint16
	cycles               uint32
	lastRtcpPacketTime   int64 // Time the last RTCP packet was received.
	lastRtcpSrTime       int64 // Time the last RTCP SR was received. Required for DLSR computation.
	lastTransit          uint32
	maxSeqNo             uint16 // The highest sequence number received in an RTP data packet

	stats Stats

	latestTimestamp     uint32 // latest received RTP timestamp on packet
	latestTimestampTime int64  // Time of the latest timestamp (in nanos since unix epoch)

	// callbacks
	onClose      func()
	onAudioLevel func(level uint8)
	feedbackCB   func([]rtcp.Packet)
	feedbackTWCC func(sn uint16, timeNS int64, marker bool)
}

type Stats struct {
	LastExpected uint32
	LastReceived uint32
	LostRate     float32
	PacketCount  uint32  // Number of packets received from this source.
	Jitter       float64 // An estimate of the statistical variance of the RTP data packet inter-arrival time.
	TotalByte    uint64
}

// BufferOptions provides configuration options for the buffer
type Options struct {
	MaxBitRate uint64
}

// NewBuffer は新しいBufferインスタンスを作成します。
// ssrc: メディアストリームのSSRC（同期ソース識別子）
// vp: ビデオバッファ用のメモリプール
// ap: 音声バッファ用のメモリプール
// 戻り値: 初期化されたBuffer
func NewBuffer(ssrc uint32, vp, ap *sync.Pool) *Buffer {
	b := &Buffer{
		mediaSSRC: ssrc,
		videoPool: vp,
		audioPool: ap,
	}

	// ExtPacketキューの初期容量を設定（パフォーマンス最適化）
	// 7は経験的に決定された値で、通常の遅延に対応
	b.extPackets.SetBaseCap(7)

	return b
}

func (b *Buffer) Bind(params webrtc.RTPParameters, o Options) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// WebRTCネゴシエーション時に決定されたコーデックパラメータ（最初のコーデックを使用）
	codec := params.Codecs[0]

	b.clockRate = codec.ClockRate
	b.maxBitrate = o.MaxBitRate
	b.mime = strings.ToLower(codec.MimeType)

	switch {
	case strings.HasPrefix(b.mime, "audio/"):
		b.codecType = webrtc.RTPCodecTypeAudio
		b.bucket = NewBucket(b.audioPool.Get().(*[]byte))
	case strings.HasPrefix(b.mime, "video/"):
		b.codecType = webrtc.RTPCodecTypeVideo
		b.bucket = NewBucket(b.videoPool.Get().(*[]byte))
	default:
		b.codecType = webrtc.RTPCodecType(0)
	}

	// Transport-Wide Congestion Control拡張を検索
	// TWCC: パケットごとの到着時刻をフィードバックして輻輳制御
	for _, ext := range params.HeaderExtensions {
		if ext.URI == sdp.TransportCCURI {
			b.twccExt = uint8(ext.ID)
			break
		}
	}

	// ビデオの場合、RTCPフィードバック機能を設定
	if b.codecType == webrtc.RTPCodecTypeVideo {
		// SDPで合意されたRTCPフィードバック機能を有効化
		for _, feedback := range codec.RTCPFeedback {
			switch feedback.Type {
			case webrtc.TypeRTCPFBGoogREMB:
				b.remb = true
			case webrtc.TypeRTCPFBTransportCC:
				b.twcc = true
			case webrtc.TypeRTCPFBNACK:
				b.nacker = newNACKQueue()
				b.nack = true
			}
		}
	} else if b.codecType == webrtc.RTPCodecTypeAudio {
		for _, h := range params.HeaderExtensions {
			if h.URI == sdp.AudioLevelURI {
				b.audioLevel = true
				b.audioExt = uint8(h.ID)
			}
		}
	}

	// Bind前に到着した保留中のパケットを処理
	for _, pp := range b.pendingPackets {
		b.calc(pp.packet, pp.arrivalTime)
	}
	b.pendingPackets = nil

	b.bound = true
}

// Write はRTPパケットをBufferに書き込みます
// pkt: 着信RTPパケットのバイト列
// 戻り値: 書き込みバイト数、エラー
func (b *Buffer) Write(pkt []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed.get() {
		err = io.EOF
		return
	}

	// まだBindされていない場合、パケットを保留
	if !b.bound {
		packet := make([]byte, len(pkt))
		copy(packet, pkt)

		b.pendingPackets = append(b.pendingPackets, PendingPackets{
			packet:      packet,
			arrivalTime: time.Now().UnixNano(),
		})

		return
	}

	b.calc(pkt, time.Now().UnixNano())

	return
}

// Read は次のパケットを読み取ります
// buff: パケットを読み込むバッファ
// 戻り値: 読み込んだバイト数、エラー
// 注: このメソッドはパケットが利用可能になるまでブロックします。
func (b *Buffer) Read(buff []byte) (int, error) {
	for {
		if b.closed.get() {
			return 0, io.EOF
		}

		b.mu.Lock()
		if b.pendingPackets != nil && len(b.pendingPackets) > b.lastPacketRead {
			if len(buff) < len(b.pendingPackets[b.lastPacketRead].packet) {
				b.mu.Unlock()
				return 0, errBufferTooSmall
			}

			n := len(b.pendingPackets[b.lastPacketRead].packet)
			copy(buff, b.pendingPackets[b.lastPacketRead].packet)

			b.lastPacketRead++

			b.mu.Unlock()

			return n, nil
		}
		b.mu.Unlock()

		time.Sleep(25 * time.Millisecond)
	}
}

// ReadExtended は拡張メタデータ付きのパケットを読み取ります。
// ExtPacket（キーフレーム情報などメタデータ付きパケット）
func (b *Buffer) ReadExtended() (*ExtPacket, error) {
	for {
		if b.closed.get() {
			return nil, io.EOF
		}
		b.mu.Lock()

		if b.extPackets.Len() > 0 {
			extPkt := b.extPackets.PopFront()
			b.mu.Unlock()
			return extPkt, nil
		}
		b.mu.Unlock()

		time.Sleep(10 * time.Millisecond)
	}
}

func (b *Buffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closeOnce.Do(func() {
		if b.bucket != nil && b.codecType == webrtc.RTPCodecTypeVideo {
			b.videoPool.Put(b.bucket.src)
		}

		if b.bucket != nil && b.codecType == webrtc.RTPCodecTypeAudio {
			b.audioPool.Put(b.bucket.src)
		}

		b.closed.set(true)

		b.onClose()
	})

	return nil
}

func (b *Buffer) OnClose(fn func()) {
	b.onClose = fn
}

func (b *Buffer) calc(pkt []byte, arrivalTime int64) {
	// RTPパケットヘッダーからシーケンス番号を抽出（バイト2-3）
	// RFC 3550: シーケンス番号は16ビットでパケットごとに1増加
	sn := binary.BigEndian.Uint16(pkt[2:4])

	if b.stats.PacketCount == 0 {
		b.baseSequenceNumber = sn
		b.maxSeqNo = sn
		b.lastReport = arrivalTime
	} else if isSequenceNumberLater(sn, b.maxSeqNo) { // パケットが順序通りまたは最新の場合
		// シーケンス番号がラップアラウンドした場合を検出
		// maxSeqNoが65535でsnが0の場合などに発生
		if sn < b.maxSeqNo {
			b.cycles += maxSequenceNumber
		}

		// NACK有効時、損失パケットを検出してキューに追加
		if b.nack {
			diff := sn - b.maxSeqNo
			for i := uint16(1); i < diff; i++ {
				var extSN uint32
				msn := sn - i

				if isCrossingWrapAroundBoundary(msn, b.maxSeqNo) {
					// 前のサイクルの拡張シーケンス番号を使用
					// msnは前のサイクルに属する（65535→0の境界をまたいだ）
					extSN = (b.cycles - maxSequenceNumber) | uint32(msn)
				} else {
					// 現在のサイクルの拡張シーケンス番号を使用
					// msnは現在のサイクルに属する
					extSN = b.cycles | uint32(msn)
				}

				b.nacker.push(extSN)
			}
		}

		b.maxSeqNo = sn
	} else if b.nack && isSequenceNumberEarlier(sn, b.maxSeqNo) { // 遅延パケット（順序が乱れて到着したパケット）の処理
		var extSN uint32

		if isCrossingWrapAroundBoundary(sn, b.maxSeqNo) {
			// snは前のサイクルに属する（65535→0の境界をまたいだ遅延パケット）
			extSN = (b.cycles - maxSequenceNumber) | uint32(sn)
		} else {
			// snは現在のサイクルに属する（通常の遅延パケット）
			extSN = b.cycles | uint32(sn)
		}

		// NACKキューから削除（既に到着したため）
		b.nacker.remove(extSN)
	}

	pb, err := b.bucket.AddPacket(pkt, sn, sn == b.maxSeqNo)
	if err != nil {
		if err == errRTXPacket {
			return
		}
		return
	}

	var packet rtp.Packet
	if err = packet.Unmarshal(pb); err != nil {
		return
	}

	// 統計情報の更新
	// 総バイト数（ビットレート計算に使用）
	b.stats.TotalByte += uint64(len(pkt))
	// 現在のレポート間隔のビットレート計算用ヘルパー
	b.bitrateHelper += uint64(len(pkt))
	// 受信パケット総数
	b.stats.PacketCount++

	// 拡張パケット（メタデータ付き）を作成
	ep := ExtPacket{
		Head:    sn == b.maxSeqNo,
		Cycle:   b.cycles,
		Packet:  packet,
		Arrival: arrivalTime,
	}

	// コーデック固有の処理
	switch b.mime {
	case "video/vp8":
		vp8Packet := VP8{}
		if err := vp8Packet.Unmarshal(packet.Payload); err != nil {
			return
		}
		ep.Payload = vp8Packet
		ep.KeyFrame = vp8Packet.IsKeyFrame
	case "video/h264":
		ep.KeyFrame = isH264Keyframe(packet.Payload)
	}

	// 初期プローブ期間（最初の25パケット）
	// ストリーム特性を学習する期間
	if b.minPacketProbe < 25 {
		if sn < b.baseSequenceNumber {
			b.baseSequenceNumber = sn
		}

		// VP8の場合、最大Temporal Layerを検出
		// Temporal Layer: 時間的なスケーラビリティ
		// TID=0（ベースレイヤー）から始まる
		if b.mime == "video/vp8" {
			pld := ep.Payload.(VP8)
			mtl := atomic.LoadInt32(&b.maxTemporalLayer)
			if mtl < int32(pld.TID) {
				atomic.StoreInt32(&b.maxTemporalLayer, int32(pld.TID))
			}
		}

		b.minPacketProbe++
	}

	b.extPackets.PushBack(&ep)

	latestTimestamp := atomic.LoadUint32(&b.latestTimestamp)
	latestTimestampTimeInNanosSinceEpoch := atomic.LoadInt64(&b.latestTimestampTime)
	if (latestTimestampTimeInNanosSinceEpoch == 0) || IsLaterTimestamp(packet.Timestamp, latestTimestamp) {
		atomic.StoreUint32(&b.latestTimestamp, packet.Timestamp)
		atomic.StoreInt64(&b.latestTimestampTime, arrivalTime)
	}

	// ジッター計算（RFC 3550 A.8）
	// ジッター: パケット到着時間のばらつき

	// 到着時刻をRTPタイムスタンプ単位に変換
	// 1e6: ナノ秒からミリ秒へ
	// clockRate/1e3: クロックレートをミリ秒単位に
	arrival := uint32(arrivalTime / 1e6 * int64(b.clockRate/1e3))
	transit := arrival - packet.Timestamp
	if b.lastTransit != 0 {
		diff := int32(transit - b.lastTransit)
		if diff < 0 {
			diff = -diff
		}

		// 指数移動平均でジッターを計算
		// 16: RFC 3550で推奨されるスムージング係数
		b.stats.Jitter += (float64(diff) - b.stats.Jitter) / 16
	}

	b.lastTransit = transit

	if b.twcc {
		if ext := packet.GetExtension(b.twccExt); len(ext) > 1 {
			// TWCCシーケンス番号と到着時刻をフィードバック
			// これによりGoogle Congestion Control (GCC)が動作
			b.feedbackTWCC(binary.BigEndian.Uint16(ext[0:2]), arrivalTime, packet.Marker)
		}
	}

	// 音声レベル処理（RFC 6464）
	if b.audioLevel {
		// 音声レベル拡張を取得
		if e := packet.GetExtension(b.audioExt); e != nil && b.onAudioLevel != nil {
			ext := rtp.AudioLevelExtension{}
			// 音声レベルをパース（dBov単位）
			if err := ext.Unmarshal(e); err == nil {
				b.onAudioLevel(ext.Level)
			}
		}
	}

	diff := arrivalTime - b.lastReport

	// NACK処理: パケット損失に対する再送要求
	if b.nacker != nil {
		if r := b.buildNACKPacket(); r != nil {
			b.feedbackCB(r)
		}
	}

	// RTCP定期レポート送信
	// reportDelta（1秒）以上経過した場合
	if diff >= reportDelta {
		// ビットレート計算
		// 8: バイトからビットへの変換
		// formula: (bits * reportDelta) / actual_time
		bitrate := (8 * b.bitrateHelper * uint64(reportDelta)) / uint64(diff)
		atomic.StoreUint64(&b.bitrate, bitrate)
		b.feedbackCB(b.getRTCP())
		b.lastReport = arrivalTime
		b.bitrateHelper = 0
	}
}

func (b *Buffer) buildNACKPacket() []rtcp.Packet {
	if nacks, askKeyframe := b.nacker.pairs(b.cycles | uint32(b.maxSeqNo)); len(nacks) > 0 || askKeyframe {
		var pkts []rtcp.Packet
		if len(nacks) > 0 {
			pkts = []rtcp.Packet{&rtcp.TransportLayerNack{
				MediaSSRC: b.mediaSSRC,
				Nacks:     nacks,
			}}
		}

		if askKeyframe {
			pkts = append(pkts, &rtcp.PictureLossIndication{
				MediaSSRC: b.mediaSSRC,
			})
		}
		return pkts
	}
	return nil
}

func (b *Buffer) buildREMBPacket() *rtcp.ReceiverEstimatedMaximumBitrate {
	bitrate := b.bitrate

	// パケット損失率に基づいてビットレートを調整
	// Google Congestion Control (GCC)のアルゴリズムに基づく

	// 低いパケット損失率（2%未満）
	// ネットワークに余裕があると判断
	if b.stats.LostRate < 0.02 {
		// 9%増加 + 2000bps（固定増加分）
		bitrate = uint64(float64(bitrate)*1.09) + 2000
	}

	// 高いパケット損失率（10%以上）
	// ネットワーク輻輳が発生していると判断
	if b.stats.LostRate > .1 {
		// 損失率に比例して減少（最大50%減少）
		// 例: 損失率20%の場合、ビットレートを10%減少
		// formula: bitrate * (1 - 0.5 * lossRate)
		bitrate = uint64(float64(bitrate) * float64(1-0.5*b.stats.LostRate))
	}

	if bitrate > b.maxBitrate {
		bitrate = b.maxBitrate
	}

	if bitrate < minBufferSize {
		bitrate = minBufferSize
	}

	b.stats.TotalByte = 0

	// REMB（Receiver Estimated Maximum Bitrate）パケットを生成
	// RFC draft-alvestrand-rmcat-remb: Google拡張RTCP
	return &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: float32(bitrate),
		SSRCs:   []uint32{b.mediaSSRC},
	}
}

func (b *Buffer) buildReceptionReport() rtcp.ReceptionReport {
	// 拡張シーケンス番号を計算（32ビット）
	// サイクル数（上位16ビット）と最大シーケンス番号（下位16ビット）を結合
	// これにより65535を超えるパケット数を追跡可能
	extMaxSeq := b.cycles | uint32(b.maxSeqNo)

	// 期待されるパケット総数を計算
	// = (最新の拡張シーケンス番号 - ベースシーケンス番号) + 1
	// RFC 3550 A.3: パケット損失計算の基礎
	expected := extMaxSeq - uint32(b.baseSequenceNumber) + 1

	lost := uint32(0)

	if b.stats.PacketCount < expected && b.stats.PacketCount != 0 {
		lost = expected - b.stats.PacketCount
	}

	expectedInterval := expected - b.stats.LastExpected
	b.stats.LastExpected = expected

	receivedInterval := b.stats.PacketCount - b.stats.LastReceived
	b.stats.LastReceived = b.stats.PacketCount

	lostInterval := expectedInterval - receivedInterval

	b.stats.LostRate = float32(lostInterval) / float32(expectedInterval)

	// RFC 3550の分数損失（Fraction Lost）を計算
	// 8ビット固定小数点形式（0 ~ 255 = 0% ~ 100%）
	var fractionLost uint8
	if expectedInterval != 0 && lostInterval > 0 {
		// 左シフト8ビット（256倍）してから除算
		// これにより小数部分を整数で表現
		// 例: 損失率10% → (10 << 8) / 100 = 25.6 → 25
		fractionLost = uint8((lostInterval << 8) / expectedInterval)
	}

	// DLSR（Delay since Last Sender Report）を計算
	// RFC 3550 6.4.1: 最後のSR受信からの遅延
	// 送信側がRTT（往復遅延時間）を計算するために使用
	var dlsr uint32

	lastSRRecv := atomic.LoadInt64(&b.lastSenderReportRecv)
	if lastSRRecv != 0 {
		delayMS := uint32((time.Now().UnixNano() - lastSRRecv) / 1e6)

		// NTPショートフォーマットに変換（16.16固定小数点）
		// 上位16ビット: 秒の整数部
		// 下位16ビット: 秒の小数部（1/65536秒単位）

		// 秒の整数部を上位16ビットに配置
		dlsr = (delayMS / 1e3) << 16
		// ミリ秒の端数を1/65536秒単位に変換して下位16ビットに配置
		// (ms % 1000) * 65536 / 1000 = 秒の小数部
		dlsr |= (delayMS % 1e3) * 65536 / 1000
	}

	// RTCP Receiver Reportを構築
	// RFC 3550 6.4.2: このレポートはRTCP RRパケットに含まれる
	rr := rtcp.ReceptionReport{
		SSRC:               b.mediaSSRC,
		FractionLost:       fractionLost,
		TotalLost:          lost,
		LastSequenceNumber: extMaxSeq,
		Jitter:             uint32(b.stats.Jitter),
		LastSenderReport:   uint32(atomic.LoadUint64(&b.lastSRNTPTime) >> 16),
		Delay:              dlsr,
	}

	return rr
}

func (b *Buffer) SetSenderReportData(rtpTime uint32, ntpTime uint64) {
	atomic.StoreUint64(&b.lastSRNTPTime, ntpTime)
	atomic.StoreUint32(&b.lastSRRTPTime, rtpTime)
	atomic.StoreInt64(&b.lastSenderReportRecv, time.Now().UnixNano())
}

func (b *Buffer) getRTCP() []rtcp.Packet {
	var pkts []rtcp.Packet

	pkts = append(pkts, &rtcp.ReceiverReport{
		Reports: []rtcp.ReceptionReport{b.buildReceptionReport()},
	})

	if b.remb && !b.twcc {
		pkts = append(pkts, b.buildREMBPacket())
	}

	return pkts
}

func (b *Buffer) GetPacket(buff []byte, sn uint16) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed.get() {
		return 0, io.EOF
	}

	return b.bucket.GetPacket(buff, sn)
}

func (b *Buffer) Bitrate() uint64 {
	return atomic.LoadUint64(&b.bitrate)
}

func (b *Buffer) MaxTemporalLayer() int32 {
	return atomic.LoadInt32(&b.maxTemporalLayer)
}

func (b *Buffer) OnTransportWideCC(fn func(sn uint16, timeNS int64, marker bool)) {
	b.feedbackTWCC = fn
}

func (b *Buffer) OnFeedback(fn func(fb []rtcp.Packet)) {
	b.feedbackCB = fn
}

func (b *Buffer) OnAudioLevel(fn func(level uint8)) {
	b.onAudioLevel = fn
}

func (b *Buffer) GetMediaSSRC() uint32 {
	return b.mediaSSRC
}

func (b *Buffer) GetClockRate() uint32 {
	return b.clockRate
}

func (b *Buffer) GetSenderReportData() (rtpTime uint32, ntpTime uint64, lastReceivedTimeInNanosSinceEpoch int64) {
	rtpTime = atomic.LoadUint32(&b.lastSRRTPTime)
	ntpTime = atomic.LoadUint64(&b.lastSRNTPTime)
	lastReceivedTimeInNanosSinceEpoch = atomic.LoadInt64(&b.lastSenderReportRecv)

	return rtpTime, ntpTime, lastReceivedTimeInNanosSinceEpoch
}

func (b *Buffer) GetStats() Stats {
	var stats Stats

	b.mu.Lock()
	stats = b.stats
	b.mu.Unlock()

	return stats
}

func (b *Buffer) GetLatestTimestamp() (latestTimestamp uint32, latestTimestampTimeInNanosSinceEpoch int64) {
	latestTimestamp = atomic.LoadUint32(&b.latestTimestamp)
	latestTimestampTimeInNanosSinceEpoch = atomic.LoadInt64(&b.latestTimestampTime)

	return latestTimestamp, latestTimestampTimeInNanosSinceEpoch
}

func IsTimestampWrapAround(timestamp1 uint32, timestamp2 uint32) bool {
	return (timestamp1&0xC000000 == 0) && (timestamp2&0xC000000 == 0xC000000)
}

func IsLaterTimestamp(timestamp1 uint32, timestamp2 uint32) bool {
	if timestamp1 > timestamp2 {
		if IsTimestampWrapAround(timestamp2, timestamp1) {
			return false
		}
		return true
	}

	if IsTimestampWrapAround(timestamp1, timestamp2) {
		return true
	}

	return false
}
