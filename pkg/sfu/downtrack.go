package sfu

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/transport/v3/packetio"
	"github.com/pion/webrtc/v4"
	"github.com/samber/lo"
)

// DownTrackはSubscriberにメディアを送信するための抽象化された構造体です。
// DownTrackはReceiverから受信したメディアをSubscriberに配信します。
// SubscriberとDownTrackは1対多の関係です。
type DownTrack interface {
	ID() string
	Bind(t webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error)
	Unbind(_ webrtc.TrackLocalContext) error
	RID() string
	StreamID() string
	Kind() webrtc.RTPCodecType

	Codec() webrtc.RTPCodecCapability
	Stop() error
	SetTransceiver(transceiver *webrtc.RTPTransceiver)
	WriteRTP(p *buffer.ExtPacket, layer int) error
	Enabled() bool
	Mute(val bool)
	Close()
	SetInitialLayers(spatialLayer, temporalLayer int32)
	CurrentSpatialLayer() int
	SwitchSpatialLayer(targetLayer int32, setAsMax bool) error
	SwitchSpatialLayerDone(layer int32)
	UptrackLayersChange(availableLayers []uint16) (int64, error)
	SwitchTemporalLayer(targetLayer int32, setAsMax bool)
	OnCloseHandler(fn func())
	OnBind(fn func())
	CreateSourceDescriptionChunks() []rtcp.SourceDescriptionChunk
	CreateSenderReport() *rtcp.SenderReport
	UpdateStats(packetLen uint32)
	GetSimulcast() simulcastTrackHelpers
	GetMime() string
	GetPayloadType() uint8
	SetPayload(payload *[]byte)
	GetWriteStream() webrtc.TrackLocalWriter
	GetSSRC() uint32
	SetLastSSRC(ssrc uint32)
	SetTrackType(trackType DownTrackType)
	SetMaxSpatialLayer(layer int32)
	SetMaxTemporalLayer(layer int32)
	Bound() bool
}

// downtrackはwebrtc.TrackLocalも実装している
var _ webrtc.TrackLocal = (*downTrack)(nil)
var _ DownTrack = (*downTrack)(nil)

type DownTrackType int

const (
	SimpleDownTrack DownTrackType = iota + 1
	SimulcastDownTrack
)

type downTrack struct {
	id            string
	peerID        string
	mime          string
	ssrc          uint32
	streamID      string
	maxTrack      int
	payloadType   uint8
	sequencer     *sequencer
	trackType     DownTrackType
	bufferFactory *buffer.Factory
	payload       *[]byte

	currentSpatialLayer int32
	targetSpatialLayer  int32
	temporalLayer       int32

	enabled  atomic.Bool
	reSync   atomic.Bool
	snOffset uint16
	tsOffset uint32
	lastSSRC uint32
	lastSN   uint16
	lastTS   uint32

	simulcast        simulcastTrackHelpers
	maxSpatialLayer  int32
	maxTemporalLayer int32

	codec          webrtc.RTPCodecCapability
	receiver       Receiver
	transceiver    *webrtc.RTPTransceiver
	writeStream    webrtc.TrackLocalWriter
	onCloseHandler func()
	onBind         func()
	closeOnce      sync.Once

	// Report helpers
	octetCount  uint32
	packetCount uint32
	maxPacketTs uint32

	bound atomic.Bool
}

func NewDownTrack(c webrtc.RTPCodecCapability, r Receiver, bf *buffer.Factory, peerID string, mt int) (*downTrack, error) {
	return &downTrack{
		id:            r.TrackID(),
		peerID:        peerID,
		maxTrack:      mt,
		streamID:      r.StreamID(),
		bufferFactory: bf,
		receiver:      r,
		codec:         c,
	}, nil
}

func (d *downTrack) Bind(t webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	slog.Debug("Bind called", "peer_id", d.peerID, "track_id", d.id)
	parameters := webrtc.RTPCodecParameters{RTPCodecCapability: d.codec}

	if codec, err := codecParametersFuzzySearch(parameters, t.CodecParameters()); err == nil {
		d.ssrc = uint32(t.SSRC())
		d.payloadType = uint8(codec.PayloadType)
		d.writeStream = t.WriteStream()
		d.mime = strings.ToLower(codec.MimeType)
		d.reSync.Store(true)
		d.enabled.Store(true)
		slog.Debug("Bind successful", "peer_id", d.peerID, "track_id", d.id, "ssrc", d.ssrc)

		if rr := d.bufferFactory.GetOrNew(packetio.RTCPBufferPacket, uint32(t.SSRC())).(*buffer.RTCPReader); rr != nil {
			rr.OnPacket(func(pkt []byte) {
				d.handleRTCP(pkt)
			})
		}

		if strings.HasPrefix(d.codec.MimeType, "video/") {
			d.sequencer = newSequencer(d.maxTrack)
		}

		if d.onBind != nil {
			d.onBind()
		}

		d.bound.Store(true)

		return codec, nil
	}

	return webrtc.RTPCodecParameters{}, webrtc.ErrUnsupportedCodec
}

func (d *downTrack) Unbind(_ webrtc.TrackLocalContext) error {
	slog.Debug("Unbind called", "peer_id", d.peerID, "track_id", d.id)
	d.bound.Store(false)
	return nil
}

func (d *downTrack) ID() string { return d.id }

func (d *downTrack) Codec() webrtc.RTPCodecCapability { return d.codec }

func (d *downTrack) StreamID() string { return d.streamID }

func (d *downTrack) RID() string { return "" }

func (d *downTrack) Kind() webrtc.RTPCodecType {
	switch {
	case strings.HasPrefix(d.codec.MimeType, "audio/"):
		return webrtc.RTPCodecTypeAudio
	case strings.HasPrefix(d.codec.MimeType, "video/"):
		return webrtc.RTPCodecTypeVideo
	default:
		return webrtc.RTPCodecType(0)
	}
}

func (d *downTrack) Stop() error {
	if d.transceiver != nil {
		return d.transceiver.Stop()
	}

	return fmt.Errorf("d.transceiver not exists")
}

func (d *downTrack) SetTransceiver(transceiver *webrtc.RTPTransceiver) {
	d.transceiver = transceiver
}

func (d *downTrack) WriteRTP(p *buffer.ExtPacket, layer int) error {
	enabled := d.enabled.Load()
	bound := d.bound.Load()
	if !enabled || !bound {
		slog.Debug("WriteRTP early return", "peer_id", d.peerID, "layer", layer, "enabled", enabled, "bound", bound, "trackType", d.trackType)
		return nil
	}

	switch d.trackType {
	case SimpleDownTrack:
		return d.writeSimpleRTP(p)
	case SimulcastDownTrack:
		return d.writeSimulcastRTP(p, layer)
	}

	return nil
}

func (d *downTrack) Enabled() bool {
	return d.enabled.Load()
}

func (d *downTrack) Mute(val bool) {
	if d.enabled.Load() != val {
		return
	}

	d.enabled.Store(!val)

	if val {
		d.reSync.Store(val)
	}
}

func (d *downTrack) Close() {
	d.closeOnce.Do(func() {
		if d.payload != nil {
			packetFactory.Put(d.payload)
		}

		if d.onCloseHandler != nil {
			d.onCloseHandler()
		}
	})
}

func (d *downTrack) SetInitialLayers(spatialLayer, temporalLayer int32) {
	atomic.StoreInt32(&d.currentSpatialLayer, spatialLayer)
	atomic.StoreInt32(&d.targetSpatialLayer, spatialLayer)
	atomic.StoreInt32(&d.temporalLayer, temporalLayer<<16|temporalLayer)
}

func (d *downTrack) CurrentSpatialLayer() int {
	return int(atomic.LoadInt32(&d.currentSpatialLayer))
}

func (d *downTrack) SwitchSpatialLayer(targetLayer int32, setAsMax bool) error {
	if d.trackType == SimulcastDownTrack {
		// Don't switch until previous switch is done or canceled
		csl := atomic.LoadInt32(&d.currentSpatialLayer)
		if csl != atomic.LoadInt32(&d.targetSpatialLayer) || csl == targetLayer {
			return ErrSpatialLayerBusy
		}

		if err := d.receiver.SwitchDownTrack(d, int(targetLayer)); err == nil {
			slog.Info("simulcast spatial layer switch enqueued", "peer_id", d.peerID, "stream_id", d.streamID, "track_id", d.id, "current_layer", csl, "target_layer", targetLayer)
			atomic.StoreInt32(&d.targetSpatialLayer, targetLayer)
			if setAsMax {
				atomic.StoreInt32(&d.maxSpatialLayer, targetLayer)
			}
		}

		return nil
	}

	return ErrSpatialNotSupported
}

func (d *downTrack) SwitchSpatialLayerDone(layer int32) {
	atomic.StoreInt32(&d.currentSpatialLayer, layer)

	// レイヤー切り替え完了時にシーケンス/タイムスタンプをリセット
	// 新しいレイヤーのSSRCからの同期を開始する
	d.snOffset = 0
	d.tsOffset = 0
	slog.Debug("SwitchSpatialLayerDone",
		"peer_id", d.peerID,
		"new_layer", layer,
		"bound", d.bound.Load(),
	)
}

func (d *downTrack) UptrackLayersChange(availableLayers []uint16) (int64, error) {
	if d.trackType != SimulcastDownTrack {
		return -1, fmt.Errorf("downtrack %s does not support simulcast", d.id)
	}

	currentLayer := uint16(d.currentSpatialLayer)

	targetLayer, err := d.getTargetLayer(availableLayers)
	if err != nil {
		return int64(currentLayer), err
	}

	if currentLayer != targetLayer {
		if err := d.SwitchSpatialLayer(int32(targetLayer), false); err != nil {
			return int64(targetLayer), err
		}
	}

	return int64(targetLayer), nil
}

func (d *downTrack) getTargetLayer(availableLayers []uint16) (uint16, error) {
	maxLayer := uint16(atomic.LoadInt32(&d.maxSpatialLayer))

	// maxLayer以下でフィルタリングして最大値を取得
	validLayers := lo.Filter(availableLayers, func(layer uint16, _ int) bool {
		return layer <= maxLayer
	})

	if len(validLayers) > 0 {
		// maxLayer以下の最大値を取得
		return lo.Max(validLayers), nil
	}

	exceedingLayers := lo.Filter(availableLayers, func(layer uint16, _ int) bool {
		return layer > maxLayer
	})

	if len(exceedingLayers) > 0 {
		// maxLayerを超える層から最小値を取得
		return lo.Min(exceedingLayers), nil
	}

	return 0, fmt.Errorf("no suitable layer found")
}

func (d *downTrack) SwitchTemporalLayer(targetLayer int32, setAsMax bool) {
	if d.trackType == SimulcastDownTrack {
		layer := atomic.LoadInt32(&d.temporalLayer)
		currentLayer := uint16(layer)
		currentTargetLayer := uint16(layer >> 16)

		// Don't switch until previous switch is done or canceled
		if currentLayer != currentTargetLayer {
			return
		}

		atomic.StoreInt32(&d.temporalLayer, targetLayer<<16|int32(currentLayer))
		if setAsMax {
			atomic.StoreInt32(&d.maxTemporalLayer, targetLayer)
		}
	}
}

func (d *downTrack) OnCloseHandler(fn func()) {
	d.onCloseHandler = fn
}

func (d *downTrack) OnBind(fn func()) {
	d.onBind = fn
}

func (d *downTrack) CreateSourceDescriptionChunks() []rtcp.SourceDescriptionChunk {
	if !d.bound.Load() {
		return nil
	}

	return []rtcp.SourceDescriptionChunk{
		{
			Source: d.ssrc,
			Items: []rtcp.SourceDescriptionItem{{
				Type: rtcp.SDESCNAME,
				Text: d.streamID,
			}},
		}, {
			Source: d.ssrc,
			Items: []rtcp.SourceDescriptionItem{{
				Type: rtcp.SDESType(15),
				Text: d.transceiver.Mid(),
			}},
		},
	}
}

func (d *downTrack) CreateSenderReport() *rtcp.SenderReport {
	if !d.bound.Load() {
		return nil
	}

	currentLayer := int(atomic.LoadInt32(&d.currentSpatialLayer))
	srRTP, srNTP := d.receiver.GetSenderReportTime(currentLayer)
	if srRTP == 0 {
		return nil
	}

	now := time.Now()
	rtpTime := d.calculateAdjustedRTPTime(srRTP, srNTP, now)
	octets, packets := d.getSRStats()

	return &rtcp.SenderReport{
		SSRC:        d.ssrc,
		NTPTime:     uint64(toNtpTime(now)),
		RTPTime:     rtpTime,
		PacketCount: packets,
		OctetCount:  octets,
	}
}

func (d *downTrack) calculateAdjustedRTPTime(srRTP uint32, srNTP uint64, now time.Time) uint32 {
	srTime := ntpTime(srNTP).Time()
	timeDiff := now.Sub(srTime)

	if timeDiff < 0 {
		return srRTP
	}

	// Convert to RTP time units
	rtpDiff := uint32(timeDiff * time.Duration(d.codec.ClockRate) / time.Second)
	return srRTP + rtpDiff
}

func (d *downTrack) UpdateStats(packetLen uint32) {
	atomic.AddUint32(&d.octetCount, packetLen)
	atomic.AddUint32(&d.packetCount, 1)
}

// writeSimpleRTP は単一ストリーム用のRTPパケット書き込み処理を行います。
// Simulcastではない通常のメディアストリームの配信に使用されます。
func (d *downTrack) writeSimpleRTP(extPkt *buffer.ExtPacket) error {
	slog.Debug("writeSimpleRTP", "peer_id", d.peerID, "pkt_ssrc", extPkt.Packet.SSRC, "pkt_sn", extPkt.Packet.SequenceNumber)

	// 再同期が必要な場合の処理
	if needsReSync := d.reSync.Load(); needsReSync {
		if shouldSkipPacket := d.handleVideoReSync(extPkt); shouldSkipPacket {
			return nil
		}

		d.performReSync(extPkt)
	}

	// 統計情報を更新
	d.UpdateStats(uint32(len(extPkt.Packet.Payload)))

	// RTPヘッダーを調整して書き込み
	return d.writeAdjustedRTPPacket(extPkt)
}

// handleVideoReSync はビデオストリームの再同期処理を行います。
// キーフレームでない場合はPLIを送信してパケットをスキップします。
func (d *downTrack) handleVideoReSync(extPkt *buffer.ExtPacket) bool {
	if d.Kind() != webrtc.RTPCodecTypeVideo {
		return false
	}

	if !extPkt.KeyFrame {
		// キーフレームでない場合はPLIを送信してスキップ
		d.receiver.SendRTCP([]rtcp.Packet{
			&rtcp.PictureLossIndication{
				SenderSSRC: d.ssrc,
				MediaSSRC:  extPkt.Packet.SSRC,
			},
		})
		return true
	}
	return false
}

// performReSync は実際の再同期処理を実行します。
// シーケンス番号とタイムスタンプのオフセットを計算し、SSRCを更新します。
func (d *downTrack) performReSync(extPkt *buffer.ExtPacket) {
	if d.lastSN != 0 {
		d.snOffset = extPkt.Packet.SequenceNumber - d.lastSN - 1
		d.tsOffset = extPkt.Packet.Timestamp - d.lastTS - 1
	}
	atomic.StoreUint32(&d.lastSSRC, extPkt.Packet.SSRC)
	d.reSync.Store(false)
}

// writeAdjustedRTPPacket はオフセット調整されたRTPパケットを書き込みます。
func (d *downTrack) writeAdjustedRTPPacket(extPkt *buffer.ExtPacket) error {
	// 調整された値を計算
	newSN := extPkt.Packet.SequenceNumber - d.snOffset
	newTS := extPkt.Packet.Timestamp - d.tsOffset

	// シーケンサーに追加（ビデオの場合のみ）
	if d.sequencer != nil {
		d.sequencer.push(extPkt.Packet.SequenceNumber, newSN, newTS, 0, extPkt.Head)
	}

	// 最後の値を更新（先頭パケットの場合のみ）
	if extPkt.Head {
		d.lastSN = newSN
		d.lastTS = newTS
	}

	// RTPヘッダーを調整
	hdr := d.buildAdjustedHeader(extPkt.Packet.Header, newSN, newTS)

	// パケット書き込み
	_, err := d.writeStream.WriteRTP(&hdr, extPkt.Packet.Payload)
	return err
}

// buildAdjustedHeader は調整されたRTPヘッダーを構築します。
func (d *downTrack) buildAdjustedHeader(originalHdr rtp.Header, newSN uint16, newTS uint32) rtp.Header {
	hdr := originalHdr
	hdr.PayloadType = d.payloadType
	hdr.Timestamp = newTS
	hdr.SequenceNumber = newSN
	hdr.SSRC = d.ssrc
	return hdr
}

// writeSimulcastRTP はSimulcast用のRTPパケット書き込み処理を行います。
// 複数の品質レイヤーを持つメディアストリームの配信に使用されます。
func (d *downTrack) writeSimulcastRTP(extPkt *buffer.ExtPacket, layer int) error {
	currentLayer := d.CurrentSpatialLayer()
	if currentLayer != layer {
		slog.Debug("writeSimulcastRTP layer mismatch", "peer_id", d.peerID, "layer", layer, "currentLayer", currentLayer)
		return nil
	}

	lastSSRC := atomic.LoadUint32(&d.lastSSRC)

	slog.Debug("writeSimulcastRTP",
		"peer_id", d.peerID,
		"layer", layer,
		"currentLayer", currentLayer,
		"pkt_ssrc", extPkt.Packet.SSRC,
		"last_ssrc", lastSSRC,
		"keyframe", extPkt.KeyFrame,
		"reSync", d.reSync.Load(),
		"snOffset", d.snOffset,
		"tsOffset", d.tsOffset,
		"lastSN", d.lastSN,
		"lastTS", d.lastTS,
		"pkt_sn", extPkt.Packet.SequenceNumber,
		"pkt_ts", extPkt.Packet.Timestamp,
	)

	// SSRC変更や再同期が必要な場合の処理
	if shouldSkip := d.handleSimulcastSync(extPkt); shouldSkip {
		slog.Debug("writeSimulcastRTP skipped by handleSimulcastSync", "peer_id", d.peerID)
		return nil
	}

	// タイムスタンプオフセットの計算と調整
	d.calculateSimulcastTimestampOffset(extPkt)

	// VP8テンポラルレイヤー処理
	payload, picID, tlz0Idx, shouldDrop := d.processVP8TemporalLayer(extPkt)
	if shouldDrop {
		d.snOffset++ // シーケンス番号の隙間を避けるためオフセット調整
		return nil
	}

	// 調整された値を計算してパケット書き込み
	return d.writeSimulcastPacket(extPkt, payload, picID, tlz0Idx, layer)
}

// handleSimulcastSync はSimulcast用の同期処理を行います。
// SSRC変更やキーフレーム待機を処理し、スキップが必要な場合はtrueを返します。
func (d *downTrack) handleSimulcastSync(extPkt *buffer.ExtPacket) bool {
	reSync := d.reSync.Load()
	lastSSRC := atomic.LoadUint32(&d.lastSSRC)

	if lastSSRC == extPkt.Packet.SSRC && !reSync {
		return false // 同期不要
	}

	// キーフレームでない場合はPLIを送信してスキップ
	if reSync && !extPkt.KeyFrame {
		d.receiver.SendRTCP([]rtcp.Packet{
			&rtcp.PictureLossIndication{
				SenderSSRC: d.ssrc,
				MediaSSRC:  extPkt.Packet.SSRC,
			},
		})

		return true
	}

	// 再同期時のタイムスタンプ調整
	if reSync && d.simulcast.lTSCalc != 0 {
		d.simulcast.lTSCalc = extPkt.Arrival
	}

	// VP8テンポラルサポートの参照値を更新
	d.updateVP8References(extPkt)

	d.reSync.Store(false)

	return false
}

// updateVP8References はVP8テンポラルレイヤーの参照値を更新します。
func (d *downTrack) updateVP8References(extPkt *buffer.ExtPacket) {
	if !d.simulcast.temporalSupported || d.mime != "video/vp8" {
		return
	}

	if vp8, ok := extPkt.Payload.(buffer.VP8); ok {
		d.simulcast.pRefPicID = d.simulcast.lPicID
		d.simulcast.refPicID = vp8.PictureID
		d.simulcast.pRefTlZIdx = d.simulcast.lTlZIdx
		d.simulcast.refTlZIdx = vp8.TL0PICIDX
	}
}

// calculateSimulcastTimestampOffset はSimulcast用のタイムスタンプオフセットを計算します。
func (d *downTrack) calculateSimulcastTimestampOffset(extPkt *buffer.ExtPacket) {
	lastSSRC := atomic.LoadUint32(&d.lastSSRC)

	if d.simulcast.lTSCalc != 0 && lastSSRC != extPkt.Packet.SSRC {
		// SSRC変更時のタイムスタンプ差分計算
		atomic.StoreUint32(&d.lastSSRC, extPkt.Packet.SSRC)

		timeDiffMs := (extPkt.Arrival - d.simulcast.lTSCalc) / 1e6
		timestampDiff := uint32((timeDiffMs * 90) / 1000) // 90kHz clock rate
		if timestampDiff == 0 {
			timestampDiff = 1 // 最小値は1
		}

		d.tsOffset = extPkt.Packet.Timestamp - (d.lastTS + timestampDiff)
		d.snOffset = extPkt.Packet.SequenceNumber - d.lastSN - 1

	} else if d.simulcast.lTSCalc == 0 {
		// 初回パケット処理
		d.lastTS = extPkt.Packet.Timestamp
		d.lastSN = extPkt.Packet.SequenceNumber
		d.initializeVP8Support(extPkt)
	}
}

// initializeVP8Support はVP8テンポラルサポートを初期化します。
func (d *downTrack) initializeVP8Support(extPkt *buffer.ExtPacket) {
	if d.mime != "video/vp8" {
		return
	}

	// テンポラルレイヤーフィルタリングを無効化
	// TODO: 設定からEnableTemporalLayerを参照するように変更する
	d.simulcast.temporalSupported = false
}

// processVP8TemporalLayer はVP8テンポラルレイヤー処理を行います。
func (d *downTrack) processVP8TemporalLayer(extPkt *buffer.ExtPacket) (payload []byte, picID uint16, tlz0Idx uint8, shouldDrop bool) {
	payload = extPkt.Packet.Payload

	if !d.simulcast.temporalSupported || d.mime != "video/vp8" {
		return payload, 0, 0, false
	}

	payload, picID, tlz0Idx, shouldDrop = setVP8TemporalLayer(extPkt, d)
	if shouldDrop {
		slog.Debug("processVP8TemporalLayer: dropping packet",
			"peer_id", d.peerID,
			"pkt_sn", extPkt.Packet.SequenceNumber,
			"temporalLayer", atomic.LoadInt32(&d.temporalLayer),
		)
	}
	return
}

// writeSimulcastPacket は調整されたSimulcastパケットを書き込みます。
func (d *downTrack) writeSimulcastPacket(extPkt *buffer.ExtPacket, payload []byte, picID uint16, tlz0Idx uint8, layer int) error {
	// 調整された値を計算
	newSN := extPkt.Packet.SequenceNumber - d.snOffset
	newTS := extPkt.Packet.Timestamp - d.tsOffset

	slog.Debug("writeSimulcastPacket",
		"peer_id", d.peerID,
		"layer", layer,
		"orig_sn", extPkt.Packet.SequenceNumber,
		"new_sn", newSN,
		"orig_ts", extPkt.Packet.Timestamp,
		"new_ts", newTS,
		"snOffset", d.snOffset,
		"tsOffset", d.tsOffset,
		"keyframe", extPkt.KeyFrame,
	)

	// シーケンサーに追加
	if d.sequencer != nil {
		if meta := d.sequencer.push(extPkt.Packet.SequenceNumber, newSN, newTS, uint8(layer), extPkt.Head); meta != nil &&
			d.simulcast.temporalSupported && d.mime == "video/vp8" {
			meta.setVP8PayloadMeta(tlz0Idx, picID)
		}
	}

	// 統計情報を更新
	d.UpdateStats(uint32(len(extPkt.Packet.Payload)))

	// 最後の値を更新
	if extPkt.Head {
		d.lastSN = newSN
		d.lastTS = newTS
	}

	// タイムスタンプベースを更新
	d.simulcast.lTSCalc = extPkt.Arrival

	// RTPヘッダーを構築して書き込み
	hdr := d.buildAdjustedHeader(extPkt.Packet.Header, newSN, newTS)
	_, err := d.writeStream.WriteRTP(&hdr, payload)

	return err
}

// RTCPネットワーク統計情報を格納する構造体
type rtcpNetworkStats struct {
	maxPacketLoss       uint8
	minEstimatedBitrate uint64
}

// handleRTCP はRTCPパケットを処理し、適切なフィードバックやレイヤー調整を行います。
// WebRTCでは、RTCPを通じてネットワーク品質やメディア要求の制御を行います。
func (d *downTrack) handleRTCP(bytes []byte) {
	if !d.enabled.Load() {
		return
	}

	// RTCPパケットをパース
	packets, err := rtcp.Unmarshal(bytes)
	if err != nil {
		slog.Debug("failed to unmarshal RTCP", "error", err)
		return
	}

	// SSRCが設定されていない場合は処理不可
	ssrc := atomic.LoadUint32(&d.lastSSRC)
	if ssrc == 0 {
		return
	}

	// 各タイプのRTCPパケットを処理
	forwardPackets := d.processRTCPPackets(packets, ssrc)
	networkStats := d.collectNetworkStats(packets)

	// Simulcastでネットワーク状況に応じたレイヤー調整
	d.handleSimulcastLayerAdjustment(networkStats)

	// 上流に転送すべきRTCPパケットがあれば送信
	if len(forwardPackets) > 0 {
		d.receiver.SendRTCP(forwardPackets)
	}
}

// processRTCPPackets は各種RTCPパケットを処理し、転送すべきパケットを返します。
func (d *downTrack) processRTCPPackets(packets []rtcp.Packet, ssrc uint32) []rtcp.Packet {
	var forwardPackets []rtcp.Packet
	processedTypes := make(map[string]bool) // 重複制御

	for _, packet := range packets {
		switch p := packet.(type) {
		case *rtcp.PictureLossIndication:
			if processed := d.processPLIPacket(p, ssrc, processedTypes); processed != nil {
				forwardPackets = append(forwardPackets, processed)
			}

		case *rtcp.FullIntraRequest:
			if processed := d.processFIRPacket(p, ssrc, processedTypes); processed != nil {
				forwardPackets = append(forwardPackets, processed)
			}

		case *rtcp.TransportLayerNack:
			d.processNACKPacket(p)

		// 統計系パケットはcollectNetworkStatsで処理
		case *rtcp.ReceiverEstimatedMaximumBitrate, *rtcp.ReceiverReport:
			// 統計収集のみ、転送不要
		}
	}

	return forwardPackets
}

// processPLIPacket はPicture Loss Indicationパケットを処理します。
// PLI: キーフレーム要求パケット
func (d *downTrack) processPLIPacket(p *rtcp.PictureLossIndication, ssrc uint32, processed map[string]bool) rtcp.Packet {
	if processed["pli"] {
		return nil // 重複防止
	}

	p.MediaSSRC = ssrc
	p.SenderSSRC = d.ssrc
	processed["pli"] = true
	return p
}

// processFIRPacket はFull Intra Requestパケットを処理します。
// FIR: 完全なキーフレーム要求パケット
func (d *downTrack) processFIRPacket(p *rtcp.FullIntraRequest, ssrc uint32, processed map[string]bool) rtcp.Packet {
	if processed["fir"] {
		return nil // 重複防止
	}

	p.MediaSSRC = ssrc
	p.SenderSSRC = d.ssrc
	processed["fir"] = true
	return p
}

// processNACKPacket はNACK（Negative Acknowledgment）パケットを処理します。
// NACK: パケット再送要求
func (d *downTrack) processNACKPacket(p *rtcp.TransportLayerNack) {
	if d.sequencer == nil {
		return
	}

	var nackedPackets []packetMeta
	for _, pair := range p.Nacks {
		nackedPackets = append(nackedPackets, d.sequencer.getSeqNoPairs(pair.PacketList())...)
	}

	if len(nackedPackets) > 0 {
		if err := d.receiver.RetransmitPackets(d, nackedPackets); err != nil {
			slog.Error("retransmit packets failed", "error", err, "nacked_count", len(nackedPackets))
		}
	}
}

// collectNetworkStats はネットワーク統計情報を収集します。
func (d *downTrack) collectNetworkStats(packets []rtcp.Packet) rtcpNetworkStats {
	stats := rtcpNetworkStats{}

	for _, packet := range packets {
		switch p := packet.(type) {
		case *rtcp.ReceiverEstimatedMaximumBitrate:
			// 最小のビットレート推定値を記録（最も制約の厳しい条件）
			bitrate := uint64(p.Bitrate)
			if stats.minEstimatedBitrate == 0 || stats.minEstimatedBitrate > bitrate {
				stats.minEstimatedBitrate = bitrate
			}

		case *rtcp.ReceiverReport:
			// 最大のパケットロス率を記録（最も悪い条件）
			for _, report := range p.Reports {
				if stats.maxPacketLoss < report.FractionLost {
					stats.maxPacketLoss = report.FractionLost
				}
			}
		}
	}

	return stats
}

// handleSimulcastLayerAdjustment はSimulcast時のレイヤー調整を処理します。
func (d *downTrack) handleSimulcastLayerAdjustment(stats rtcpNetworkStats) {
	if d.trackType != SimulcastDownTrack {
		return
	}

	// ネットワーク統計に基づいてレイヤー調整が必要かチェック
	if stats.maxPacketLoss > 0 || stats.minEstimatedBitrate > 0 {
		d.handleLayerChange(stats.maxPacketLoss, stats.minEstimatedBitrate)
	}
}

// ネットワーク品質に基づくレイヤー調整の定数
const (
	// パケットロス率のしきい値
	packetLossLowThreshold  = 5  // 5%以下は良好
	packetLossHighThreshold = 25 // 25%以上は深刻

	// ビットレート調整の倍率
	bitrateUpgradeRatio   = 0.75  // テンポラルレイヤー上げに必要な帯域（3/4）
	spatialUpgradeRatio   = 1.5   // スペーシャルレイヤー上げに必要な帯域（3/2）
	bitrateDowngradeRatio = 0.625 // レイヤー下げを考慮する帯域（5/8）

	// レイヤー切り替え後の待機時間
	temporalSwitchDelay = 3 * time.Second  // テンポラルレイヤー変更後の待機
	spatialUpDelay      = 5 * time.Second  // スペーシャルレイヤー上げ後の待機
	spatialDownDelay    = 10 * time.Second // スペーシャルレイヤー下げ後の待機

	// スペーシャルレイヤーの上限
	maxSpatialLayerIndex = 2
)

// layerSwitchContext はレイヤー切り替えに必要な情報をまとめる構造体
type layerSwitchContext struct {
	currentSpatial    int32
	targetSpatial     int32
	currentTemporal   int32
	targetTemporal    int32
	maxTemporal       int32
	maxSpatial        int32
	currentBitrate    uint64
	maxTemporalLayer  int32
	bitrates          []uint64
	maxTemporalLayers []int32
}

// handleLayerChange はネットワーク品質に基づいてSimulcastレイヤーを調整します。
// パケットロス率と推定ビットレートを監視し、適切な品質レイヤーに自動調整します。
func (d *downTrack) handleLayerChange(maxRatePacketLoss uint8, expectedMinBitrate uint64) {
	// レイヤー切り替えの実行条件をチェック
	if !d.canPerformLayerSwitch() {
		slog.Debug("handleLayerChange: canPerformLayerSwitch returned false",
			"peer_id", d.peerID,
			"currentSpatial", atomic.LoadInt32(&d.currentSpatialLayer),
			"targetSpatial", atomic.LoadInt32(&d.targetSpatialLayer),
			"switchDelay", d.simulcast.switchDelay,
			"now", time.Now(),
		)
		return
	}

	// 切り替えコンテキストを構築
	ctx := d.buildLayerSwitchContext()

	// ネットワーク品質に基づいてレイヤー調整を決定・実行
	if maxRatePacketLoss <= packetLossLowThreshold {
		d.handleGoodNetworkConditions(ctx, expectedMinBitrate)
	} else if maxRatePacketLoss >= packetLossHighThreshold {
		d.handlePoorNetworkConditions(ctx, expectedMinBitrate)
	}
}

// canPerformLayerSwitch はレイヤー切り替えが実行可能かチェックします。
func (d *downTrack) canPerformLayerSwitch() bool {
	// 現在切り替え中でないことを確認
	currentSpatial := atomic.LoadInt32(&d.currentSpatialLayer)
	targetSpatial := atomic.LoadInt32(&d.targetSpatialLayer)

	temporalLayer := atomic.LoadInt32(&d.temporalLayer)
	currentTemporal := d.extractCurrentTemporalLayer(temporalLayer)
	targetTemporal := d.extractTargetTemporalLayer(temporalLayer)

	// 切り替え中でない かつ 待機時間が経過していること
	return currentSpatial == targetSpatial && currentTemporal == targetTemporal && time.Now().After(d.simulcast.switchDelay)
}

// extractCurrentTemporalLayer はテンポラルレイヤー値から現在のレイヤーを抽出します。
func (d *downTrack) extractCurrentTemporalLayer(temporalLayer int32) int32 {
	return temporalLayer & 0x0f // 下位4ビットが現在のテンポラルレイヤー
}

// extractTargetTemporalLayer はテンポラルレイヤー値から目標レイヤーを抽出します。
func (d *downTrack) extractTargetTemporalLayer(temporalLayer int32) int32 {
	return temporalLayer >> 16 // 上位16ビットが目標テンポラルレイヤー
}

// buildLayerSwitchContext は切り替えに必要な情報を収集します。
func (d *downTrack) buildLayerSwitchContext() layerSwitchContext {
	currentSpatial := atomic.LoadInt32(&d.currentSpatialLayer)
	temporalLayer := atomic.LoadInt32(&d.temporalLayer)

	bitrates := d.receiver.GetBitrate()
	maxTemporalLayers := d.receiver.GetMaxTemporalLayer()

	return layerSwitchContext{
		currentSpatial:    currentSpatial,
		targetSpatial:     atomic.LoadInt32(&d.targetSpatialLayer),
		currentTemporal:   d.extractCurrentTemporalLayer(temporalLayer),
		targetTemporal:    d.extractTargetTemporalLayer(temporalLayer),
		maxTemporal:       atomic.LoadInt32(&d.maxTemporalLayer),
		maxSpatial:        atomic.LoadInt32(&d.maxSpatialLayer),
		currentBitrate:    bitrates[currentSpatial],
		maxTemporalLayer:  maxTemporalLayers[currentSpatial],
		bitrates:          bitrates[:],
		maxTemporalLayers: maxTemporalLayers[:],
	}
}

// handleGoodNetworkConditions は良好なネットワーク条件での品質向上処理を行います。
func (d *downTrack) handleGoodNetworkConditions(ctx layerSwitchContext, expectedBitrate uint64) {
	slog.Debug("handleGoodNetworkConditions",
		"peer_id", d.peerID,
		"currentSpatial", ctx.currentSpatial,
		"maxSpatial", ctx.maxSpatial,
		"currentTemporal", ctx.currentTemporal,
		"maxTemporal", ctx.maxTemporal,
		"maxTemporalLayer", ctx.maxTemporalLayer,
		"expectedBitrate", expectedBitrate,
		"currentBitrate", ctx.currentBitrate,
		"bitrates", ctx.bitrates,
		"canUpgradeTemporal", d.canUpgradeTemporalLayer(ctx, expectedBitrate),
		"canUpgradeSpatial", d.canUpgradeSpatialLayer(ctx, expectedBitrate),
	)

	// テンポラルレイヤーの向上を試行
	if d.canUpgradeTemporalLayer(ctx, expectedBitrate) {
		d.upgradeTemporalLayer(ctx)
		return
	}

	// スペーシャルレイヤーの向上を試行
	if d.canUpgradeSpatialLayer(ctx, expectedBitrate) {
		d.upgradeSpatialLayer(ctx)
	}
}

// handlePoorNetworkConditions は劣悪なネットワーク条件での品質低下処理を行います。
func (d *downTrack) handlePoorNetworkConditions(ctx layerSwitchContext, expectedBitrate uint64) {
	// スペーシャルレイヤーの低下を試行
	if d.canDowngradeSpatialLayer(ctx, expectedBitrate) {
		d.downgradeSpatialLayer(ctx)
	} else {
		// テンポラルレイヤーの低下を実行
		d.downgradeTemporalLayer(ctx)
	}
}

// canUpgradeTemporalLayer はテンポラルレイヤー向上が可能かチェックします。
func (d *downTrack) canUpgradeTemporalLayer(ctx layerSwitchContext, expectedBitrate uint64) bool {
	return ctx.currentTemporal < ctx.maxTemporalLayer &&
		ctx.currentTemporal+1 <= ctx.maxTemporal &&
		expectedBitrate >= uint64(float64(ctx.currentBitrate)*bitrateUpgradeRatio)
}

// canUpgradeSpatialLayer はスペーシャルレイヤー向上が可能かチェックします。
func (d *downTrack) canUpgradeSpatialLayer(ctx layerSwitchContext, expectedBitrate uint64) bool {
	return ctx.currentTemporal >= ctx.maxTemporalLayer &&
		expectedBitrate >= uint64(float64(ctx.currentBitrate)*spatialUpgradeRatio) &&
		ctx.currentSpatial+1 <= ctx.maxSpatial &&
		ctx.currentSpatial+1 <= maxSpatialLayerIndex
}

// canDowngradeSpatialLayer はスペーシャルレイヤー低下が必要かチェックします。
func (d *downTrack) canDowngradeSpatialLayer(ctx layerSwitchContext, expectedBitrate uint64) bool {
	return (expectedBitrate <= uint64(float64(ctx.currentBitrate)*bitrateDowngradeRatio) || ctx.currentTemporal == 0) &&
		ctx.currentSpatial > 0 &&
		ctx.bitrates[ctx.currentSpatial-1] != 0
}

// upgradeTemporalLayer はテンポラルレイヤーを向上させます。
func (d *downTrack) upgradeTemporalLayer(ctx layerSwitchContext) {
	d.SwitchTemporalLayer(ctx.currentTemporal+1, false)
	d.simulcast.switchDelay = time.Now().Add(temporalSwitchDelay)
}

// upgradeSpatialLayer はスペーシャルレイヤーを向上させます。
func (d *downTrack) upgradeSpatialLayer(ctx layerSwitchContext) {
	if err := d.SwitchSpatialLayer(ctx.currentSpatial+1, false); err == nil {
		slog.Info("simulcast spatial layer upgrade requested", "peer_id", d.peerID, "stream_id", d.streamID, "track_id", d.id, "from_layer", ctx.currentSpatial, "to_layer", ctx.currentSpatial+1)
		d.SwitchTemporalLayer(0, false) // 新しいスペーシャルレイヤーではテンポラルレイヤーを0から開始
	}
	d.simulcast.switchDelay = time.Now().Add(spatialUpDelay)
}

// downgradeSpatialLayer はスペーシャルレイヤーを低下させます。
func (d *downTrack) downgradeSpatialLayer(ctx layerSwitchContext) {
	if err := d.SwitchSpatialLayer(ctx.currentSpatial-1, false); err != nil {
		// スペーシャルレイヤー変更失敗時は、低いレイヤーの最大テンポラルレイヤーに設定
		d.SwitchTemporalLayer(ctx.maxTemporalLayers[ctx.currentSpatial-1], false)
	} else {
		slog.Info("simulcast spatial layer downgrade requested", "peer_id", d.peerID, "stream_id", d.streamID, "track_id", d.id, "from_layer", ctx.currentSpatial, "to_layer", ctx.currentSpatial-1)
	}

	d.simulcast.switchDelay = time.Now().Add(spatialDownDelay)
}

// downgradeTemporalLayer はテンポラルレイヤーを低下させます。
func (d *downTrack) downgradeTemporalLayer(ctx layerSwitchContext) {
	if ctx.currentTemporal > 0 {
		d.SwitchTemporalLayer(ctx.currentTemporal-1, false)
		d.simulcast.switchDelay = time.Now().Add(temporalSwitchDelay)
	}
}

func (d *downTrack) getSRStats() (octets, packets uint32) {
	octets = atomic.LoadUint32(&d.octetCount)
	packets = atomic.LoadUint32(&d.packetCount)
	return
}

func (d *downTrack) GetSimulcast() simulcastTrackHelpers {
	return d.simulcast
}

func (d *downTrack) GetMime() string {
	return d.mime
}

func (d *downTrack) GetPayloadType() uint8 {
	return d.payloadType
}

func (d *downTrack) SetPayload(payload *[]byte) {
	d.payload = payload
}

func (d *downTrack) GetSSRC() uint32 {
	return d.ssrc
}

func (d *downTrack) SetLastSSRC(ssrc uint32) {
	atomic.StoreUint32(&d.lastSSRC, ssrc)
}

func (d *downTrack) GetWriteStream() webrtc.TrackLocalWriter {
	return d.writeStream
}

func (d *downTrack) SetTrackType(trackType DownTrackType) {
	d.trackType = trackType
}

func (d *downTrack) SetMaxSpatialLayer(layer int32) {
	atomic.StoreInt32(&d.maxSpatialLayer, layer)
}

func (d *downTrack) SetMaxTemporalLayer(layer int32) {
	atomic.StoreInt32(&d.maxTemporalLayer, layer)
}

func (d *downTrack) Bound() bool {
	return d.bound.Load()
}
