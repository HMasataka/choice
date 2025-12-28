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
	reportDelta       = 1e9
	minBufferSize     = 100000
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

// Buffer はRTPパケットのバッファリングとRTCPフィードバック生成を行う
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
	lastSenderReportRecv int64
	baseSequenceNumber   uint16
	cycles               uint32
	lastRTCPPacketTime   int64
	lastRTCPSrTime       int64
	lastTransit          uint32
	maxSeqNo             uint16

	stats Stats

	latestTimestamp     uint32
	latestTimestampTime int64

	onClose      func()
	onAudioLevel func(level uint8)
	feedbackCB   func([]rtcp.Packet)
	feedbackTWCC func(sn uint16, timeNS int64, marker bool)
}

type Stats struct {
	LastExpected uint32
	LastReceived uint32
	LostRate     float32
	PacketCount  uint32
	Jitter       float64
	TotalByte    uint64
}

type Options struct {
	MaxBitRate uint64
}

func NewBuffer(ssrc uint32, vp, ap *sync.Pool) *Buffer {
	b := &Buffer{
		mediaSSRC: ssrc,
		videoPool: vp,
		audioPool: ap,
	}

	b.extPackets.SetBaseCap(7)

	return b
}

func (b *Buffer) Bind(params webrtc.RTPParameters, o Options) {
	b.mu.Lock()
	defer b.mu.Unlock()

	codec := params.Codecs[0]
	b.setCodecInfo(codec, o)
	b.initBucket()
	b.findTWCCExtension(params.HeaderExtensions)
	b.setupFeedbackCapabilities(codec, params.HeaderExtensions)
	b.processPendingPackets()
	b.bound = true
}

// setCodecInfo はコーデック情報を設定する
func (b *Buffer) setCodecInfo(codec webrtc.RTPCodecParameters, o Options) {
	b.clockRate = codec.ClockRate
	b.maxBitrate = o.MaxBitRate
	b.mime = strings.ToLower(codec.MimeType)
}

// initBucket はメディアタイプに応じたバケットを初期化する
func (b *Buffer) initBucket() {
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
}

// findTWCCExtension はTransport-Wide CC拡張IDを検索する
func (b *Buffer) findTWCCExtension(extensions []webrtc.RTPHeaderExtensionParameter) {
	for _, ext := range extensions {
		if ext.URI == sdp.TransportCCURI {
			b.twccExt = uint8(ext.ID)
			return
		}
	}
}

// setupFeedbackCapabilities はメディアタイプに応じたフィードバック機能を設定する
func (b *Buffer) setupFeedbackCapabilities(codec webrtc.RTPCodecParameters, extensions []webrtc.RTPHeaderExtensionParameter) {
	switch b.codecType {
	case webrtc.RTPCodecTypeVideo:
		b.setupVideoFeedback(codec.RTCPFeedback)
	case webrtc.RTPCodecTypeAudio:
		b.setupAudioLevel(extensions)
	}
}

// setupVideoFeedback はビデオ用RTCPフィードバック（REMB、TWCC、NACK）を設定する
func (b *Buffer) setupVideoFeedback(feedbacks []webrtc.RTCPFeedback) {
	for _, feedback := range feedbacks {
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
}

// setupAudioLevel はオーディオレベル拡張を設定する
func (b *Buffer) setupAudioLevel(extensions []webrtc.RTPHeaderExtensionParameter) {
	for _, h := range extensions {
		if h.URI == sdp.AudioLevelURI {
			b.audioLevel = true
			b.audioExt = uint8(h.ID)
			return
		}
	}
}

// processPendingPackets は保留中のパケットを処理する
func (b *Buffer) processPendingPackets() {
	for _, pp := range b.pendingPackets {
		b.calc(pp.packet, pp.arrivalTime)
	}
	b.pendingPackets = nil
}

func (b *Buffer) Write(pkt []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed.get() {
		err = io.EOF
		return
	}

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
			b.videoPool.Put(&b.bucket.buf)
		}

		if b.bucket != nil && b.codecType == webrtc.RTPCodecTypeAudio {
			b.audioPool.Put(&b.bucket.buf)
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
	sn := binary.BigEndian.Uint16(pkt[2:4])

	b.updateSequenceAndNACK(sn, arrivalTime)

	isHead := sn == b.maxSeqNo

	packet, ok := b.addAndUnmarshal(pkt, sn, isHead)
	if !ok {
		return
	}

	b.updateReceiveStats(len(pkt))

	ep, ok := b.buildExtPacket(packet, arrivalTime, isHead)
	if !ok {
		return
	}

	b.initialProbe(sn, &ep)
	b.extPackets.PushBack(&ep)
	b.updateLatestTimestamp(packet.Timestamp, arrivalTime)
	b.updateJitter(packet.Timestamp, arrivalTime)
	b.handleTWCC(packet, arrivalTime)
	b.handleAudioLevel(packet)
	b.handleFeedbacksAndReports(arrivalTime)
}

func (b *Buffer) updateSequenceAndNACK(sn uint16, arrivalTime int64) {
	if b.stats.PacketCount == 0 {
		b.baseSequenceNumber = sn
		b.maxSeqNo = sn
		b.lastReport = arrivalTime
		return
	}

	if isSequenceNumberLater(sn, b.maxSeqNo) {
		if sn < b.maxSeqNo {
			b.cycles += maxSequenceNumber
		}
		b.handleMissingForNACK(sn)
		b.maxSeqNo = sn
		return
	}

	if b.nack && isSequenceNumberEarlier(sn, b.maxSeqNo) {
		b.nacker.remove(b.extendedSequenceNumber(sn))
	}
}

func (b *Buffer) addAndUnmarshal(pkt []byte, sn uint16, head bool) (rtp.Packet, bool) {
	pb, err := b.bucket.AddPacket(pkt, sn, head)
	if err != nil {
		return rtp.Packet{}, false
	}

	var packet rtp.Packet
	if err = packet.Unmarshal(pb); err != nil {
		return rtp.Packet{}, false
	}

	return packet, true
}

func (b *Buffer) updateReceiveStats(size int) {
	b.stats.TotalByte += uint64(size)
	b.bitrateHelper += uint64(size)
	b.stats.PacketCount++
}

func (b *Buffer) buildExtPacket(packet rtp.Packet, arrivalTime int64, head bool) (ExtPacket, bool) {
	ep := ExtPacket{
		Head:    head,
		Cycle:   b.cycles,
		Packet:  packet,
		Arrival: arrivalTime,
	}

	switch b.mime {
	case "video/vp8":
		vp8Packet := VP8{}
		if err := vp8Packet.Unmarshal(packet.Payload); err != nil {
			return ExtPacket{}, false
		}
		ep.Payload = vp8Packet
		ep.KeyFrame = vp8Packet.IsKeyFrame
	case "video/h264":
		ep.KeyFrame = isH264Keyframe(packet.Payload)
	}

	return ep, true
}

func (b *Buffer) initialProbe(sn uint16, ep *ExtPacket) {
	if b.minPacketProbe >= 25 {
		return
	}

	if sn < b.baseSequenceNumber {
		b.baseSequenceNumber = sn
	}

	if b.mime == "video/vp8" {
		pld := ep.Payload.(VP8)
		mtl := atomic.LoadInt32(&b.maxTemporalLayer)
		if mtl < int32(pld.TID) {
			atomic.StoreInt32(&b.maxTemporalLayer, int32(pld.TID))
		}
	}

	b.minPacketProbe++
}

// updateJitter はRFC 3550 A.8に基づいてジッターを更新する
func (b *Buffer) updateJitter(ts uint32, arrivalTime int64) {
	arrival := uint32(arrivalTime / 1e6 * int64(b.clockRate/1e3))
	transit := arrival - ts
	if b.lastTransit != 0 {
		diff := int32(transit - b.lastTransit)
		if diff < 0 {
			diff = -diff
		}
		b.stats.Jitter += (float64(diff) - b.stats.Jitter) / 16
	}
	b.lastTransit = transit
}

func (b *Buffer) handleTWCC(packet rtp.Packet, arrivalTime int64) {
	if !b.twcc {
		return
	}
	if ext := packet.GetExtension(b.twccExt); len(ext) > 1 {
		b.feedbackTWCC(binary.BigEndian.Uint16(ext[0:2]), arrivalTime, packet.Marker)
	}
}

func (b *Buffer) handleAudioLevel(packet rtp.Packet) {
	if !b.audioLevel {
		return
	}
	if e := packet.GetExtension(b.audioExt); e != nil && b.onAudioLevel != nil {
		ext := rtp.AudioLevelExtension{}
		if err := ext.Unmarshal(e); err == nil {
			b.onAudioLevel(ext.Level)
		}
	}
}

func (b *Buffer) handleFeedbacksAndReports(arrivalTime int64) {
	if b.nacker != nil {
		if r := b.buildNACKPacket(); r != nil {
			b.feedbackCB(r)
		}
	}

	diff := arrivalTime - b.lastReport
	if diff >= reportDelta {
		bitrate := (8 * b.bitrateHelper * uint64(reportDelta)) / uint64(diff)
		atomic.StoreUint64(&b.bitrate, bitrate)
		b.feedbackCB(b.getRTCP())
		b.lastReport = arrivalTime
		b.bitrateHelper = 0
	}
}

func (b *Buffer) buildNACKPacket() []rtcp.Packet {
	if nacks, askKeyframe := b.nacker.pairs(b.cycles | uint32(b.maxSeqNo)); len(nacks) > 0 || askKeyframe {
		var packets []rtcp.Packet
		if len(nacks) > 0 {
			packets = []rtcp.Packet{&rtcp.TransportLayerNack{
				MediaSSRC: b.mediaSSRC,
				Nacks:     nacks,
			}}
		}

		if askKeyframe {
			packets = append(packets, &rtcp.PictureLossIndication{
				MediaSSRC: b.mediaSSRC,
			})
		}
		return packets
	}
	return nil
}

func (b *Buffer) extendedSequenceNumber(sn uint16) uint32 {
	if isCrossingWrapAroundBoundary(sn, b.maxSeqNo) {
		return (b.cycles - maxSequenceNumber) | uint32(sn)
	}
	return b.cycles | uint32(sn)
}

func (b *Buffer) handleMissingForNACK(sn uint16) {
	if !b.nack {
		return
	}
	diff := sn - b.maxSeqNo
	for i := uint16(1); i < diff; i++ {
		msn := sn - i
		b.nacker.push(b.extendedSequenceNumber(msn))
	}
}

// buildREMBPacket はGCC(Google Congestion Control)に基づいてREMBパケットを生成する
func (b *Buffer) buildREMBPacket() *rtcp.ReceiverEstimatedMaximumBitrate {
	bitrate := b.adjustBitrateByLossRate(b.bitrate)
	bitrate = b.clampBitrate(bitrate)
	b.stats.TotalByte = 0

	return &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: float32(bitrate),
		SSRCs:   []uint32{b.mediaSSRC},
	}
}

// adjustBitrateByLossRate は損失率に基づいてビットレートを調整する
func (b *Buffer) adjustBitrateByLossRate(bitrate uint64) uint64 {
	switch {
	case b.stats.LostRate < 0.02:
		return uint64(float64(bitrate)*1.09) + 2000
	case b.stats.LostRate > 0.1:
		return uint64(float64(bitrate) * float64(1-0.5*b.stats.LostRate))
	default:
		return bitrate
	}
}

// clampBitrate はビットレートを最小/最大値に制限する
func (b *Buffer) clampBitrate(bitrate uint64) uint64 {
	if bitrate > b.maxBitrate {
		return b.maxBitrate
	}
	if bitrate < minBufferSize {
		return minBufferSize
	}
	return bitrate
}

// buildReceptionReport はRFC 3550に基づいてReceiver Reportを構築する
func (b *Buffer) buildReceptionReport() rtcp.ReceptionReport {
	extMaxSeq := b.cycles | uint32(b.maxSeqNo)
	expected := extMaxSeq - uint32(b.baseSequenceNumber) + 1
	lost := b.calcTotalLost(expected)
	fractionLost := b.calcFractionLost(expected)

	return rtcp.ReceptionReport{
		SSRC:               b.mediaSSRC,
		FractionLost:       fractionLost,
		TotalLost:          lost,
		LastSequenceNumber: extMaxSeq,
		Jitter:             uint32(b.stats.Jitter),
		LastSenderReport:   uint32(atomic.LoadUint64(&b.lastSRNTPTime) >> 16),
		Delay:              b.calcDLSR(),
	}
}

// calcTotalLost は総損失パケット数を計算する
func (b *Buffer) calcTotalLost(expected uint32) uint32 {
	if b.stats.PacketCount < expected && b.stats.PacketCount != 0 {
		return expected - b.stats.PacketCount
	}
	return 0
}

// calcFractionLost は直近の損失率を計算しstatsを更新する
func (b *Buffer) calcFractionLost(expected uint32) uint8 {
	expectedInterval := expected - b.stats.LastExpected
	b.stats.LastExpected = expected

	receivedInterval := b.stats.PacketCount - b.stats.LastReceived
	b.stats.LastReceived = b.stats.PacketCount

	lostInterval := expectedInterval - receivedInterval
	if expectedInterval != 0 {
		b.stats.LostRate = float32(lostInterval) / float32(expectedInterval)
	}

	if expectedInterval != 0 && lostInterval > 0 {
		return uint8((lostInterval << 8) / expectedInterval)
	}
	return 0
}

// calcDLSR はDelay Since Last SRを計算する
func (b *Buffer) calcDLSR() uint32 {
	lastSRRecv := atomic.LoadInt64(&b.lastSenderReportRecv)
	if lastSRRecv == 0 {
		return 0
	}
	delayMS := uint32((time.Now().UnixNano() - lastSRRecv) / 1e6)
	dlsr := (delayMS / 1e3) << 16
	dlsr |= (delayMS % 1e3) * 65536 / 1000
	return dlsr
}

func (b *Buffer) SetSenderReportData(rtpTime uint32, ntpTime uint64) {
	atomic.StoreUint64(&b.lastSRNTPTime, ntpTime)
	atomic.StoreUint32(&b.lastSRRTPTime, rtpTime)
	atomic.StoreInt64(&b.lastSenderReportRecv, time.Now().UnixNano())
}

func (b *Buffer) getRTCP() []rtcp.Packet {
	var packets []rtcp.Packet

	packets = append(packets, &rtcp.ReceiverReport{
		Reports: []rtcp.ReceptionReport{b.buildReceptionReport()},
	})

	if b.remb && !b.twcc {
		packets = append(packets, b.buildREMBPacket())
	}

	return packets
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
		return !IsTimestampWrapAround(timestamp2, timestamp1)
	}

	if IsTimestampWrapAround(timestamp1, timestamp2) {
		return true
	}

	return false
}

func (b *Buffer) updateLatestTimestamp(timestamp uint32, arrivalTime int64) {
	latest := atomic.LoadUint32(&b.latestTimestamp)
	latestTime := atomic.LoadInt64(&b.latestTimestampTime)

	if (latestTime == 0) || IsLaterTimestamp(timestamp, latest) {
		atomic.StoreUint32(&b.latestTimestamp, timestamp)
		atomic.StoreInt64(&b.latestTimestampTime, arrivalTime)
	}
}
