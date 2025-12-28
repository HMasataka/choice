package twcc

import (
	"encoding/binary"
	"math"
	"math/rand"
	"sort"
	"sync"

	"github.com/gammazero/deque"
	"github.com/pion/rtcp"
)

// TWCCフィードバックパケットのヘッダフィールドオフセット
const (
	baseSequenceNumberOffset = 8
	packetStatusCountOffset  = 10
	referenceTimeOffset      = 12

	tccReportDelta          = 1e8  // 通常のフィードバック送信間隔（100ms）
	tccReportDeltaAfterMark = 50e6 // RTPマーカビット後の送信間隔（50ms）
)

// rtpExtInfo は拡張シーケンス番号と受信時刻のペアを保持する
type rtpExtInfo struct {
	ExtTSN    uint32 // 拡張シーケンス番号（サイクルカウント + 16ビットSN）
	Timestamp int64  // 受信時刻（マイクロ秒）
}

// Responder はTWCC(Transport Wide Congestion Control)フィードバックを生成する
// 仕様: https://tools.ietf.org/html/draft-holmer-rmcat-transport-wide-cc-extensions-01
type Responder struct {
	mu sync.Mutex

	extInfo     []rtpExtInfo
	lastReport  int64
	cycles      uint32
	lastExtSN   uint32
	pktCtn      uint8
	lastSn      uint16
	lastExtInfo uint16
	mSSRC       uint32 // フィードバック対象のメディアソースSSRC
	sSSRC       uint32 // 送信側SSRC

	len      uint16
	deltaLen uint16
	payload  [100]byte
	deltas   [200]byte
	chunk    uint16

	onFeedback func(packet rtcp.RawPacket)
}

func NewTransportWideCCResponder(ssrc uint32) *Responder {
	return &Responder{
		extInfo: make([]rtpExtInfo, 0, 101),
		sSSRC:   rand.Uint32(),
		mSSRC:   ssrc,
	}
}

// Push はRTPパケットの受信情報を登録し、条件を満たせばフィードバックを送信する
func (t *Responder) Push(sn uint16, timeNS int64, marker bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.updateCycles(sn)
	t.addPacketInfo(sn, timeNS)

	if t.lastReport == 0 {
		t.lastReport = timeNS
	}
	t.lastSn = sn

	if t.shouldSendFeedback(timeNS, marker) {
		t.sendFeedback(timeNS)
	}
}

// updateCycles は16ビットシーケンス番号のオーバーフローを検出しサイクルカウントを更新する
func (t *Responder) updateCycles(sn uint16) {
	if sn < 0x0fff && t.lastSn > 0xf000 {
		t.cycles += 1 << 16
	}
}

// addPacketInfo は拡張シーケンス番号と受信時刻をバッファに追加する
func (t *Responder) addPacketInfo(sn uint16, timeNS int64) {
	t.extInfo = append(t.extInfo, rtpExtInfo{
		ExtTSN:    t.cycles | uint32(sn),
		Timestamp: timeNS / 1e3,
	})
}

// shouldSendFeedback はフィードバック送信条件を判定する
// 条件: 20パケット以上 かつ (100ms経過 or 100パケット超 or マーカービット+50ms経過)
func (t *Responder) shouldSendFeedback(timeNS int64, marker bool) bool {
	if len(t.extInfo) <= 20 || t.mSSRC == 0 {
		return false
	}

	delta := timeNS - t.lastReport
	return delta >= tccReportDelta || len(t.extInfo) > 100 || (marker && delta >= tccReportDeltaAfterMark)
}

// sendFeedback はTWCCフィードバックパケットを生成しコールバックで送信する
func (t *Responder) sendFeedback(timeNS int64) {
	if pkt := t.buildTransportCCPacket(); pkt != nil {
		t.onFeedback(pkt)
	}
	t.lastReport = timeNS
}

func (t *Responder) OnFeedback(f func(p rtcp.RawPacket)) {
	t.onFeedback = f
}

// statusEncoder はパケットステータスの圧縮エンコーディング状態を管理する
type statusEncoder struct {
	resp       *Responder
	statusList deque.Deque[uint16]
	timestamp  int64
	lastStatus uint16
	maxStatus  uint16
	same       bool
	firstRecv  bool
}

func newStatusEncoder(resp *Responder) *statusEncoder {
	enc := &statusEncoder{
		resp:       resp,
		lastStatus: rtcp.TypeTCCPacketReceivedWithoutDelta,
		maxStatus:  rtcp.TypeTCCPacketNotReceived,
		same:       true,
	}
	enc.statusList.SetBaseCap(3)
	return enc
}

// buildTransportCCPacket は蓄積されたパケット情報からTWCCフィードバックを構築する
func (t *Responder) buildTransportCCPacket() rtcp.RawPacket {
	if len(t.extInfo) == 0 {
		return nil
	}

	tccPackets := t.buildPacketList()
	t.extInfo = t.extInfo[:0]

	enc := newStatusEncoder(t)
	enc.encodePackets(tccPackets)
	enc.flushRemaining()

	return t.buildRTCPPacket()
}

// buildPacketList はソート済みパケットリストを構築し、ロストパケットを挿入する
func (t *Responder) buildPacketList() []rtpExtInfo {
	sort.Slice(t.extInfo, func(i, j int) bool {
		return t.extInfo[i].ExtTSN < t.extInfo[j].ExtTSN
	})

	tccPackets := make([]rtpExtInfo, 0, int(float64(len(t.extInfo))*1.2))
	for _, info := range t.extInfo {
		if info.ExtTSN < t.lastExtSN {
			continue
		}

		// ギャップ検出: ロストパケットのエントリを挿入
		if t.lastExtSN != 0 {
			for j := t.lastExtSN + 1; j < info.ExtTSN; j++ {
				tccPackets = append(tccPackets, rtpExtInfo{ExtTSN: j})
			}
		}

		t.lastExtSN = info.ExtTSN
		tccPackets = append(tccPackets, info)
	}

	return tccPackets
}

// encodePackets は各パケットのステータスとデルタをエンコードする
func (e *statusEncoder) encodePackets(packets []rtpExtInfo) {
	for _, pkt := range packets {
		status := e.processPacket(pkt, packets)
		e.updateStatusList(status)
		e.flushIfNeeded()
	}
}

// processPacket は単一パケットを処理しステータスを返す
func (e *statusEncoder) processPacket(pkt rtpExtInfo, allPackets []rtpExtInfo) uint16 {
	if pkt.Timestamp == 0 {
		return rtcp.TypeTCCPacketNotReceived
	}

	if !e.firstRecv {
		e.initFirstPacket(pkt, allPackets)
	}

	return e.computeDeltaStatus(pkt)
}

// initFirstPacket は最初の受信パケットでヘッダを初期化する
func (e *statusEncoder) initFirstPacket(pkt rtpExtInfo, allPackets []rtpExtInfo) {
	e.firstRecv = true
	refTime := pkt.Timestamp / 64e3
	e.timestamp = refTime * 64e3
	e.resp.writeHeader(
		uint16(allPackets[0].ExtTSN),
		uint16(len(allPackets)),
		uint32(refTime),
	)
	e.resp.pktCtn++
}

// computeDeltaStatus はデルタを計算しステータスを決定する
func (e *statusEncoder) computeDeltaStatus(pkt rtpExtInfo) uint16 {
	delta := (pkt.Timestamp - e.timestamp) / 250
	e.timestamp = pkt.Timestamp

	if delta >= 0 && delta <= 255 {
		e.resp.writeDelta(rtcp.TypeTCCPacketReceivedSmallDelta, uint16(delta))
		return rtcp.TypeTCCPacketReceivedSmallDelta
	}

	rDelta := clampInt16(delta)
	e.resp.writeDelta(rtcp.TypeTCCPacketReceivedLargeDelta, uint16(rDelta))

	return rtcp.TypeTCCPacketReceivedLargeDelta
}

// clampInt16 はint64をint16の範囲にクランプする
func clampInt16(v int64) int16 {
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	if v < math.MinInt16 {
		return math.MinInt16
	}
	return int16(v)
}

// updateStatusList はステータスリストを更新しRLE判定を行う
func (e *statusEncoder) updateStatusList(status uint16) {
	// RLEで出力可能か判定
	if e.same && status != e.lastStatus && e.lastStatus != rtcp.TypeTCCPacketReceivedWithoutDelta {
		if e.statusList.Len() > 7 {
			e.resp.writeRunLengthChunk(e.lastStatus, uint16(e.statusList.Len()))
			e.reset()
		} else {
			e.same = false
		}
	}

	e.statusList.PushBack(status)
	if status > e.maxStatus {
		e.maxStatus = status
	}
	e.lastStatus = status
}

// flushIfNeeded はStatus Symbol Chunkの出力条件を満たしていれば出力する
func (e *statusEncoder) flushIfNeeded() {
	if !e.same && e.maxStatus == rtcp.TypeTCCPacketReceivedLargeDelta && e.statusList.Len() > 6 {
		e.writeSymbolChunk(rtcp.TypeTCCSymbolSizeTwoBit, 7)
		e.recalculateState()
	} else if !e.same && e.statusList.Len() > 13 {
		e.writeSymbolChunk(rtcp.TypeTCCSymbolSizeOneBit, 14)
	}
}

// writeSymbolChunk は指定数のシンボルをチャンクとして出力する
func (e *statusEncoder) writeSymbolChunk(symbolSize uint16, count int) {
	for i := range count {
		e.resp.createStatusSymbolChunk(symbolSize, e.statusList.PopFront(), i)
	}
	e.resp.writeStatusSymbolChunk(symbolSize)
	e.reset()
}

// reset はエンコーダの状態をリセットする
func (e *statusEncoder) reset() {
	e.statusList.Clear()
	e.lastStatus = rtcp.TypeTCCPacketReceivedWithoutDelta
	e.maxStatus = rtcp.TypeTCCPacketNotReceived
	e.same = true
}

// recalculateState は残りのステータスリストから状態を再計算する
func (e *statusEncoder) recalculateState() {
	for i := 0; i < e.statusList.Len(); i++ {
		status := e.statusList.At(i)
		if status > e.maxStatus {
			e.maxStatus = status
		}
		if e.same && e.lastStatus != rtcp.TypeTCCPacketReceivedWithoutDelta && status != e.lastStatus {
			e.same = false
		}
		e.lastStatus = status
	}
}

// flushRemaining は残りのステータスを出力する
func (e *statusEncoder) flushRemaining() {
	if e.statusList.Len() == 0 {
		return
	}

	if e.same {
		e.resp.writeRunLengthChunk(e.lastStatus, uint16(e.statusList.Len()))
		return
	}

	symbolSize := uint16(rtcp.TypeTCCSymbolSizeOneBit)
	if e.maxStatus == rtcp.TypeTCCPacketReceivedLargeDelta {
		symbolSize = rtcp.TypeTCCSymbolSizeTwoBit
	}

	for i := 0; i < e.statusList.Len(); i++ {
		e.resp.createStatusSymbolChunk(symbolSize, e.statusList.PopFront(), i)
	}
	e.resp.writeStatusSymbolChunk(symbolSize)
}

// buildRTCPPacket はペイロードとデルタからRTCPパケットを構築する
func (t *Responder) buildRTCPPacket() rtcp.RawPacket {
	pLen, padSize := t.calculatePadding()

	hdr := rtcp.Header{
		Padding: padSize > 0,
		Length:  (pLen / 4) - 1,
		Count:   rtcp.FormatTCC,
		Type:    rtcp.TypeTransportSpecificFeedback,
	}
	hb, _ := hdr.Marshal()

	pkt := make(rtcp.RawPacket, pLen)
	copy(pkt, hb)
	copy(pkt[4:], t.payload[:t.len])
	copy(pkt[4+t.len:], t.deltas[:t.deltaLen])
	if padSize > 0 {
		pkt[len(pkt)-1] = padSize
	}

	t.deltaLen = 0
	return pkt
}

// calculatePadding は4バイト境界アライメント用のパディングを計算する
func (t *Responder) calculatePadding() (pLen uint16, padSize uint8) {
	pLen = t.len + t.deltaLen + 4
	for pLen%4 != 0 {
		padSize++
		pLen++
	}
	return
}

// writeHeader はTWCCフィードバックのヘッダ部（16バイト）を構築する
//
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|                     SSRC of packet sender                     |
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|                     SSRC of media source                      |
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|     base sequence number      |      packet status count      |
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|         reference time (24bits)           | fb pkt count (8)  |
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
func (t *Responder) writeHeader(bSN, packetCount uint16, refTime uint32) {
	binary.BigEndian.PutUint32(t.payload[0:], t.sSSRC)
	binary.BigEndian.PutUint32(t.payload[4:], t.mSSRC)
	binary.BigEndian.PutUint16(t.payload[baseSequenceNumberOffset:], bSN)
	binary.BigEndian.PutUint16(t.payload[packetStatusCountOffset:], packetCount)
	binary.BigEndian.PutUint32(t.payload[referenceTimeOffset:], refTime<<8|uint32(t.pktCtn))
	t.len = 16
}

// writeRunLengthChunk はRLE形式のチャンクを書き込む
// フォーマット: |0|S(2bits)|Run Length(13bits)|
func (t *Responder) writeRunLengthChunk(symbol uint16, runLength uint16) {
	binary.BigEndian.PutUint16(t.payload[t.len:], symbol<<13|runLength)
	t.len += 2
}

// createStatusSymbolChunk はStatus Symbol Chunkにシンボルを追加する
func (t *Responder) createStatusSymbolChunk(symbolSize, symbol uint16, i int) {
	numOfBits := symbolSize + 1
	t.chunk = setNBitsOfUint16(t.chunk, numOfBits, numOfBits*uint16(i)+2, symbol)
}

// writeStatusSymbolChunk は構築済みのStatus Symbol Chunkをペイロードに書き込む
// フォーマット: |1|S(1bit)|symbol list...|
func (t *Responder) writeStatusSymbolChunk(symbolSize uint16) {
	t.chunk = setNBitsOfUint16(t.chunk, 1, 0, 1)
	t.chunk = setNBitsOfUint16(t.chunk, 1, 1, symbolSize)
	binary.BigEndian.PutUint16(t.payload[t.len:], t.chunk)
	t.chunk = 0
	t.len += 2
}

// writeDelta はタイムスタンプデルタをバッファに書き込む
// 小デルタ: 1バイト(0-255)、大デルタ: 2バイト符号付き
func (t *Responder) writeDelta(deltaType, delta uint16) {
	if deltaType == rtcp.TypeTCCPacketReceivedSmallDelta {
		t.deltas[t.deltaLen] = byte(delta)
		t.deltaLen++
		return
	}
	binary.BigEndian.PutUint16(t.deltas[t.deltaLen:], delta)
	t.deltaLen += 2
}

// setNBitsOfUint16 は指定ビット位置にビット列を設定する
func setNBitsOfUint16(src, size, startIndex, val uint16) uint16 {
	if startIndex+size > 16 {
		return 0
	}
	val &= (1 << size) - 1
	return src | (val << (16 - size - startIndex))
}
