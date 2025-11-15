package sfu

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/transport/packetio"
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
	return &downTrack{}, nil
}

func (d *downTrack) Bind(t webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	parameters := webrtc.RTPCodecParameters{RTPCodecCapability: d.codec}

	if codec, err := codecParametersFuzzySearch(parameters, t.CodecParameters()); err == nil {
		d.ssrc = uint32(t.SSRC())
		d.payloadType = uint8(codec.PayloadType)
		d.writeStream = t.WriteStream()
		d.mime = strings.ToLower(codec.MimeType)
		d.reSync.Store(true)
		d.enabled.Store(true)

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
	if !d.enabled.Load() || !d.bound.Load() {
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
}

func (d *downTrack) UptrackLayersChange(availableLayers []uint16) (int64, error) {
	if d.trackType != SimulcastDownTrack {
		return -1, fmt.Errorf("downtrack %s does not support simulcast", d.id)
	}

	currentLayer := uint16(d.currentSpatialLayer)

	targetLayer, err := d.getTargetLayer(currentLayer, availableLayers)
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

func (d *downTrack) getTargetLayer(currentLayer uint16, availableLayers []uint16) (uint16, error) {
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

func (d *downTrack) writeSimulcastRTP(extPkt *buffer.ExtPacket, layer int) error {
	// Check if packet SSRC is different from before
	// if true, the video source changed
	reSync := d.reSync.Load()
	csl := d.CurrentSpatialLayer()

	if csl != layer {
		return nil
	}

	lastSSRC := atomic.LoadUint32(&d.lastSSRC)
	if lastSSRC != extPkt.Packet.SSRC || reSync {
		// Wait for a keyframe to sync new source
		if reSync && !extPkt.KeyFrame {
			// Packet is not a keyframe, discard it
			d.receiver.SendRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{SenderSSRC: d.ssrc, MediaSSRC: extPkt.Packet.SSRC},
			})
			return nil
		}

		if reSync && d.simulcast.lTSCalc != 0 {
			d.simulcast.lTSCalc = extPkt.Arrival
		}

		if d.simulcast.temporalSupported {
			if d.mime == "video/vp8" {
				if vp8, ok := extPkt.Payload.(buffer.VP8); ok {
					d.simulcast.pRefPicID = d.simulcast.lPicID
					d.simulcast.refPicID = vp8.PictureID
					d.simulcast.pRefTlZIdx = d.simulcast.lTlZIdx
					d.simulcast.refTlZIdx = vp8.TL0PICIDX
				}
			}
		}
		d.reSync.Store(false)
	}

	// Compute how much time passed between the old RTP extPkt
	// and the current packet, and fix timestamp on source change
	if d.simulcast.lTSCalc != 0 && lastSSRC != extPkt.Packet.SSRC {
		atomic.StoreUint32(&d.lastSSRC, extPkt.Packet.SSRC)
		tDiff := (extPkt.Arrival - d.simulcast.lTSCalc) / 1e6
		td := uint32((tDiff * 90) / 1000)
		if td == 0 {
			td = 1
		}
		d.tsOffset = extPkt.Packet.Timestamp - (d.lastTS + td)
		d.snOffset = extPkt.Packet.SequenceNumber - d.lastSN - 1
	} else if d.simulcast.lTSCalc == 0 {
		d.lastTS = extPkt.Packet.Timestamp
		d.lastSN = extPkt.Packet.SequenceNumber
		if d.mime == "video/vp8" {
			if vp8, ok := extPkt.Payload.(buffer.VP8); ok {
				d.simulcast.temporalSupported = vp8.TemporalSupported
			}
		}
	}
	newSN := extPkt.Packet.SequenceNumber - d.snOffset
	newTS := extPkt.Packet.Timestamp - d.tsOffset
	payload := extPkt.Packet.Payload

	var (
		picID   uint16
		tlz0Idx uint8
	)
	if d.simulcast.temporalSupported {
		if d.mime == "video/vp8" {
			drop := false
			if payload, picID, tlz0Idx, drop = setVP8TemporalLayer(extPkt, d); drop {
				// Pkt not in temporal getLayer update sequence number offset to avoid gaps
				d.snOffset++
				return nil
			}
		}
	}

	if d.sequencer != nil {
		if meta := d.sequencer.push(extPkt.Packet.SequenceNumber, newSN, newTS, uint8(csl), extPkt.Head); meta != nil &&
			d.simulcast.temporalSupported && d.mime == "video/vp8" {
			meta.setVP8PayloadMeta(tlz0Idx, picID)
		}
	}

	atomic.AddUint32(&d.octetCount, uint32(len(extPkt.Packet.Payload)))
	atomic.AddUint32(&d.packetCount, 1)

	if extPkt.Head {
		d.lastSN = newSN
		d.lastTS = newTS
	}
	// Update base
	d.simulcast.lTSCalc = extPkt.Arrival
	// Update extPkt headers
	hdr := extPkt.Packet.Header
	hdr.SequenceNumber = newSN
	hdr.Timestamp = newTS
	hdr.SSRC = d.ssrc
	hdr.PayloadType = d.payloadType

	_, err := d.writeStream.WriteRTP(&hdr, payload)
	return err
}

func (d *downTrack) handleRTCP(bytes []byte) {
	if !d.enabled.Load() {
		return
	}

	packets, err := rtcp.Unmarshal(bytes)
	if err != nil {
		// TODO log
	}

	var fwdPkts []rtcp.Packet
	pliOnce := true
	firOnce := true

	var (
		maxRatePacketLoss  uint8
		expectedMinBitrate uint64
	)

	ssrc := atomic.LoadUint32(&d.lastSSRC)
	if ssrc == 0 {
		return
	}

	for _, packet := range packets {
		switch p := packet.(type) {
		case *rtcp.PictureLossIndication:
			if pliOnce {
				p.MediaSSRC = ssrc
				p.SenderSSRC = d.ssrc
				fwdPkts = append(fwdPkts, p)
				pliOnce = false
			}
		case *rtcp.FullIntraRequest:
			if firOnce {
				p.MediaSSRC = ssrc
				p.SenderSSRC = d.ssrc
				fwdPkts = append(fwdPkts, p)
				firOnce = false
			}
		case *rtcp.ReceiverEstimatedMaximumBitrate:
			if expectedMinBitrate == 0 || expectedMinBitrate > uint64(p.Bitrate) {
				expectedMinBitrate = uint64(p.Bitrate)
			}
		case *rtcp.ReceiverReport:
			for _, r := range p.Reports {
				if maxRatePacketLoss == 0 || maxRatePacketLoss < r.FractionLost {
					maxRatePacketLoss = r.FractionLost
				}
			}
		case *rtcp.TransportLayerNack:
			if d.sequencer != nil {
				var nackedPackets []packetMeta
				for _, pair := range p.Nacks {
					nackedPackets = append(nackedPackets, d.sequencer.getSeqNoPairs(pair.PacketList())...)
				}
				if err = d.receiver.RetransmitPackets(d, nackedPackets); err != nil {
					return
				}
			}
		}
	}
	if d.trackType == SimulcastDownTrack && (maxRatePacketLoss != 0 || expectedMinBitrate != 0) {
		d.handleLayerChange(maxRatePacketLoss, expectedMinBitrate)
	}

	if len(fwdPkts) > 0 {
		d.receiver.SendRTCP(fwdPkts)
	}
}

func (d *downTrack) handleLayerChange(maxRatePacketLoss uint8, expectedMinBitrate uint64) {
	currentSpatialLayer := atomic.LoadInt32(&d.currentSpatialLayer)
	targetSpatialLayer := atomic.LoadInt32(&d.targetSpatialLayer)

	temporalLayer := atomic.LoadInt32(&d.temporalLayer)
	currentTemporalLayer := temporalLayer & 0x0f
	targetTemporalLayer := temporalLayer >> 16

	if targetSpatialLayer == currentSpatialLayer && currentTemporalLayer == targetTemporalLayer {
		if time.Now().After(d.simulcast.switchDelay) {
			brs := d.receiver.GetBitrate()
			cbr := brs[currentSpatialLayer]
			mtl := d.receiver.GetMaxTemporalLayer()
			mctl := mtl[currentSpatialLayer]

			if maxRatePacketLoss <= 5 {
				if currentTemporalLayer < mctl && currentTemporalLayer+1 <= atomic.LoadInt32(&d.maxTemporalLayer) &&
					expectedMinBitrate >= 3*cbr/4 {
					d.SwitchTemporalLayer(currentTemporalLayer+1, false)
					d.simulcast.switchDelay = time.Now().Add(3 * time.Second)
				}
				if currentTemporalLayer >= mctl && expectedMinBitrate >= 3*cbr/2 && currentSpatialLayer+1 <= atomic.LoadInt32(&d.maxSpatialLayer) &&
					currentSpatialLayer+1 <= 2 {
					if err := d.SwitchSpatialLayer(currentSpatialLayer+1, false); err == nil {
						d.SwitchTemporalLayer(0, false)
					}
					d.simulcast.switchDelay = time.Now().Add(5 * time.Second)
				}
			}
			if maxRatePacketLoss >= 25 {
				if (expectedMinBitrate <= 5*cbr/8 || currentTemporalLayer == 0) &&
					currentSpatialLayer > 0 &&
					brs[currentSpatialLayer-1] != 0 {
					if err := d.SwitchSpatialLayer(currentSpatialLayer-1, false); err != nil {
						d.SwitchTemporalLayer(mtl[currentSpatialLayer-1], false)
					}
					d.simulcast.switchDelay = time.Now().Add(10 * time.Second)
				} else {
					d.SwitchTemporalLayer(currentTemporalLayer-1, false)
					d.simulcast.switchDelay = time.Now().Add(5 * time.Second)
				}
			}
		}
	}

}

func (d *downTrack) getSRStats() (octets, packets uint32) {
	octets = atomic.LoadUint32(&d.octetCount)
	packets = atomic.LoadUint32(&d.packetCount)
	return
}

func (d *downTrack) Bound() bool {
	return d.bound.Load()
}
