package sfu

import (
	"io"
	"log/slog"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/gammazero/workerpool"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// ReceiverはPublisherから着信したRTPストリームを管理するための抽象化された構造体です。
// 受信したメディアはDowntrackに分配され、Subscriberに送信されます。
// ReceiverとDownTrackは1対多の関係です。
type Receiver interface {
	TrackID() string
	StreamID() string
	Codec() webrtc.RTPCodecParameters
	Kind() webrtc.RTPCodecType
	SSRC(layer int) uint32
	SetTrackMeta(trackID, streamID string)
	AddUpTrack(track *webrtc.TrackRemote, buffer *buffer.Buffer, bestQualityFirst bool)
	AddDownTrack(track DownTrack, bestQualityFirst bool)
	SwitchDownTrack(track DownTrack, layer int) error
	GetBitrate() [3]uint64
	GetMaxTemporalLayer() [3]int32
	RetransmitPackets(track DownTrack, packets []packetMeta) error
	DeleteDownTrack(layer int, id string)
	OnCloseHandler(fn func())
	SendRTCP(p []rtcp.Packet)
	SetRTCPCh(ch chan []rtcp.Packet)
	GetSenderReportTime(layer int) (rtpTS uint32, ntpTS uint64)
}

type WebRTCReceiver struct {
	mu        sync.Mutex
	closeOnce sync.Once

	peerID         string
	trackID        string
	streamID       string
	kind           webrtc.RTPCodecType
	closed         atomic.Bool
	bandwidth      uint64
	lastPli        int64
	stream         string
	receiver       *webrtc.RTPReceiver
	codec          webrtc.RTPCodecParameters
	rtcpCh         chan []rtcp.Packet
	buffers        [3]*buffer.Buffer
	upTracks       [3]*webrtc.TrackRemote
	available      [3]atomic.Bool
	downTracks     [3]atomic.Value // []*DownTrack
	pending        [3]atomic.Bool
	pendingTracks  [3][]DownTrack
	nackWorker     *workerpool.WorkerPool
	isSimulcast    bool
	onCloseHandler func()
}

var _ Receiver = (*WebRTCReceiver)(nil)

func NewWebRTCReceiver(receiver *webrtc.RTPReceiver, track *webrtc.TrackRemote, pid string) Receiver {
	return &WebRTCReceiver{
		peerID:      pid,
		receiver:    receiver,
		trackID:     track.ID(),
		streamID:    track.StreamID(),
		codec:       track.Codec(),
		kind:        track.Kind(),
		nackWorker:  workerpool.New(1),
		isSimulcast: len(track.RID()) > 0,
	}
}

func (w *WebRTCReceiver) SetTrackMeta(trackID, streamID string) {
	w.streamID = streamID
	w.trackID = trackID
}

func (w *WebRTCReceiver) StreamID() string {
	return w.streamID
}

func (w *WebRTCReceiver) TrackID() string {
	return w.trackID
}

func (w *WebRTCReceiver) SSRC(layer int) uint32 {
	if track := w.upTracks[layer]; track != nil {
		return uint32(track.SSRC())
	}
	return 0
}

func (w *WebRTCReceiver) Codec() webrtc.RTPCodecParameters {
	return w.codec
}

func (w *WebRTCReceiver) Kind() webrtc.RTPCodecType {
	return w.kind
}

// determineTrackLayer determines the simulcast layer based on track RID
func (w *WebRTCReceiver) determineTrackLayer(track *webrtc.TrackRemote) int {
	rid := strings.ToLower(track.RID())
	slog.Debug("determining track layer", "stream_id", track.StreamID(), "track_id", track.ID(), "rid", rid)

	// 1) Map common RID names to layers
	switch rid {
	case fullResolution, "full", "high", "hi", "r2", "2":
		return 2
	case halfResolution, "half", "mid", "m", "r1", "1":
		return 1
	case quarterResolution, "low", "l", "r0", "0":
		return 0
	}

	// 2) Try to parse trailing digit pattern like r0/r1/r2
	if len(rid) > 1 && rid[0] == 'r' {
		if ridIdx := int(rid[1] - '0'); ridIdx >= 0 && ridIdx <= 2 {
			return ridIdx
		}
	}

	// 3) No RID: assign first free layer slot (Plan-B or SSRC-based simulcast)
	for i := range w.upTracks {
		if w.upTracks[i] == nil {
			return i
		}
	}

	// 4) Fallback
	return 0
}

// setupUpTrack configures the up track for the specified layer
func (w *WebRTCReceiver) setupUpTrack(layer int, track *webrtc.TrackRemote, buff *buffer.Buffer) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.upTracks[layer] = track
	w.buffers[layer] = buff
	w.available[layer].Store(true)
	w.downTracks[layer].Store(make([]DownTrack, 0, 10))
	w.pendingTracks[layer] = make([]DownTrack, 0, 10)
}

// switchDownTracksToTargetQuality switches all down tracks to the best available quality up to targetLayer
func (w *WebRTCReceiver) switchDownTracksToTargetQuality(targetLayer int) {
	for i := range targetLayer {
		dts := w.downTracks[i].Load()
		if dts == nil {
			continue
		}
		for _, dt := range dts.([]DownTrack) {
			_ = dt.SwitchSpatialLayer(int32(targetLayer), false)
		}
	}
}

// switchDownTracksToLowestQuality switches all down tracks to the lowest available quality down to targetLayer
func (w *WebRTCReceiver) switchDownTracksToLowestQuality(targetLayer int) {
	for l := 2; l >= targetLayer; l-- {
		dts := w.downTracks[l].Load()
		if dts == nil {
			continue
		}

		for _, dt := range dts.([]DownTrack) {
			_ = dt.SwitchSpatialLayer(int32(targetLayer), false)
		}
	}
}

// handleSimulcastQualityAdjustment manages simulcast quality switching based on the strategy
func (w *WebRTCReceiver) handleSimulcastQualityAdjustment(layer int, bestQualityFirst bool) {
	if !w.isSimulcast {
		return
	}

	if bestQualityFirst && (!w.available[2].Load() || layer == 2) {
		w.switchDownTracksToTargetQuality(layer)
	} else if !bestQualityFirst && (!w.available[0].Load() || layer == 0) {
		w.switchDownTracksToLowestQuality(layer)
	}
}

// determineDownTrackLayer determines the appropriate layer for a new down track
func (w *WebRTCReceiver) determineDownTrackLayer(bestQualityFirst bool) int {
	if !w.isSimulcast {
		return 0
	}

	layer := 0

	for i, t := range w.available {
		if t.Load() {
			layer = i
			if !bestQualityFirst {
				break
			}
		}
	}

	return layer
}

// configureSimulcastDownTrack configures a down track for simulcast mode
func (w *WebRTCReceiver) configureSimulcastDownTrack(track DownTrack, layer int) {
	track.SetInitialLayers(int32(layer), 2)
	track.SetMaxSpatialLayer(2)
	track.SetMaxTemporalLayer(2)
	track.SetLastSSRC(w.SSRC(layer))
	track.SetTrackType(SimulcastDownTrack)
	track.SetPayload(packetFactory.Get().(*[]byte))
	slog.Info("downtrack configured (simulcast)", "stream_id", w.streamID, "track_id", w.trackID, "start_layer", layer)
}

// configureSimpleDownTrack configures a down track for simple (non-simulcast) mode
func (w *WebRTCReceiver) configureSimpleDownTrack(track DownTrack) {
	track.SetInitialLayers(0, 0)
	track.SetTrackType(SimpleDownTrack)
	slog.Info("downtrack configured (simple)", "stream_id", w.streamID, "track_id", w.trackID)
}

// retrieveAndPreparePacket retrieves a packet from buffer and prepares it for retransmission
func (w *WebRTCReceiver) retrieveAndPreparePacket(meta packetMeta, track DownTrack, pktBuff []byte) (*rtp.Packet, int, error) {
	buff := w.buffers[meta.layer]
	if buff == nil {
		return nil, 0, io.EOF
	}

	i, err := buff.GetPacket(pktBuff, meta.sourceSeqNo)
	if err != nil {
		return nil, 0, err
	}

	var pkt rtp.Packet
	if err = pkt.Unmarshal(pktBuff[:i]); err != nil {
		return nil, 0, err
	}

	// Update packet headers for retransmission
	pkt.SequenceNumber = meta.targetSeqNo
	pkt.Timestamp = meta.timestamp
	pkt.SSRC = track.GetSSRC()
	pkt.PayloadType = track.GetPayloadType()

	return &pkt, i, nil
}

// applyTemporalLayerModifications applies temporal layer modifications to the packet payload
func (w *WebRTCReceiver) applyTemporalLayerModifications(pkt *rtp.Packet, meta packetMeta, track DownTrack) error {
	if !track.GetSimulcast().temporalSupported {
		return nil
	}

	switch track.GetMime() {
	case "video/vp8":
		var vp8 buffer.VP8
		if err := vp8.Unmarshal(pkt.Payload); err != nil {
			return err
		}
		tlzoID, picID := meta.getVP8PayloadMeta()
		modifyVP8TemporalPayload(pkt.Payload, vp8.PicIDIdx, vp8.TlzIdx, picID, tlzoID, vp8.MBit)
	}

	return nil
}

// sendRetransmitPacket sends a single retransmitted packet to the track
func (w *WebRTCReceiver) sendRetransmitPacket(pkt *rtp.Packet, track DownTrack, packetSize uint32) error {
	if _, err := track.GetWriteStream().WriteRTP(&pkt.Header, pkt.Payload); err != nil {
		return err
	}
	track.UpdateStats(packetSize)
	return nil
}

// processRetransmitPacket processes a single packet for retransmission
func (w *WebRTCReceiver) processRetransmitPacket(meta packetMeta, track DownTrack, pktBuff []byte) error {
	pkt, packetSize, err := w.retrieveAndPreparePacket(meta, track, pktBuff)
	if err != nil {
		return err
	}

	if err := w.applyTemporalLayerModifications(pkt, meta, track); err != nil {
		return err
	}

	return w.sendRetransmitPacket(pkt, track, uint32(packetSize))
}

// createPLIPacket creates a Picture Loss Indication packet for the given layer
func (w *WebRTCReceiver) createPLIPacket(layer int) []rtcp.Packet {
	return []rtcp.Packet{
		&rtcp.PictureLossIndication{
			SenderSSRC: rand.Uint32(),
			MediaSSRC:  w.SSRC(layer),
		},
	}
}

// handlePendingLayerSwitch handles pending simulcast layer switches when keyframe is received
func (w *WebRTCReceiver) handlePendingLayerSwitch(layer int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for idx, dt := range w.pendingTracks[layer] {
		prev := dt.CurrentSpatialLayer()
		w.deleteDownTrack(prev, dt.ID())
		w.storeDownTrack(layer, dt)
		dt.SwitchSpatialLayerDone(int32(layer))
		slog.Info("simulcast spatial layer switched", "peer_id", w.peerID, "stream_id", w.streamID, "track_id", w.trackID, "from_layer", prev, "to_layer", layer)
		w.pendingTracks[layer][idx] = nil
	}
	w.pendingTracks[layer] = w.pendingTracks[layer][:0]
	w.pending[layer].Store(false)
}

// processSimulcastLayerSwitching processes simulcast layer switching based on packet type
func (w *WebRTCReceiver) processSimulcastLayerSwitching(layer int, pkt *buffer.ExtPacket, pli []rtcp.Packet) {
	if !w.isSimulcast || !w.pending[layer].Load() {
		return
	}

	if pkt.KeyFrame {
		w.handlePendingLayerSwitch(layer)
	} else {
		w.SendRTCP(pli)
	}
}

// distributePacketToDownTracks distributes a packet to all down tracks for the given layer
func (w *WebRTCReceiver) distributePacketToDownTracks(layer int, pkt *buffer.ExtPacket) {
	downTracks := w.downTracks[layer].Load().([]DownTrack)

	for _, dt := range downTracks {
		if err := dt.WriteRTP(pkt, layer); err != nil {
			if err == io.EOF || err == io.ErrClosedPipe {
				w.mu.Lock()
				w.deleteDownTrack(layer, dt.ID())
				w.mu.Unlock()
			}
		}
	}
}

func (w *WebRTCReceiver) AddUpTrack(track *webrtc.TrackRemote, buff *buffer.Buffer, bestQualityFirst bool) {
	if w.closed.Load() {
		return
	}

	layer := w.determineTrackLayer(track)
	slog.Info("receiver uptrack added", "stream_id", track.StreamID(), "track_id", track.ID(), "rid", track.RID(), "ssrc", track.SSRC(), "layer", layer)

	w.setupUpTrack(layer, track, buff)
	w.handleSimulcastQualityAdjustment(layer, bestQualityFirst)

	go w.writeRTP(layer)
}

func (w *WebRTCReceiver) AddDownTrack(track DownTrack, bestQualityFirst bool) {
	if w.closed.Load() {
		return
	}

	layer := w.determineDownTrackLayer(bestQualityFirst)

	if w.isDownTrackSubscribed(layer, track) {
		return
	}

	if w.isSimulcast {
		w.configureSimulcastDownTrack(track, layer)
	} else {
		w.configureSimpleDownTrack(track)
	}

	w.mu.Lock()
	w.storeDownTrack(layer, track)
	w.mu.Unlock()
}

func (w *WebRTCReceiver) SwitchDownTrack(track DownTrack, layer int) error {
	if w.closed.Load() {
		return errNoReceiverFound
	}
	if w.available[layer].Load() {
		slog.Info("simulcast spatial layer switch requested", "peer_id", w.peerID, "stream_id", w.streamID, "track_id", w.trackID, "current_layer", track.CurrentSpatialLayer(), "target_layer", layer)
		w.mu.Lock()
		w.pending[layer].Store(true)
		w.pendingTracks[layer] = append(w.pendingTracks[layer], track)
		w.mu.Unlock()
		return nil
	}
	slog.Info("simulcast spatial layer switch requested but layer unavailable", "peer_id", w.peerID, "stream_id", w.streamID, "track_id", w.trackID, "target_layer", layer)
	return errNoReceiverFound
}

func (w *WebRTCReceiver) GetBitrate() [3]uint64 {
	var br [3]uint64
	for i, buff := range w.buffers {
		if buff != nil {
			br[i] = buff.Bitrate()
		}
	}
	return br
}

func (w *WebRTCReceiver) GetMaxTemporalLayer() [3]int32 {
	var tls [3]int32
	for i, a := range w.available {
		if a.Load() {
			tls[i] = w.buffers[i].MaxTemporalLayer()
		}
	}
	return tls
}

// OnCloseHandler method to be called on remote tracked removed
func (w *WebRTCReceiver) OnCloseHandler(fn func()) {
	w.onCloseHandler = fn
}

// DeleteDownTrack removes a DownTrack from a Receiver
func (w *WebRTCReceiver) DeleteDownTrack(layer int, id string) {
	if w.closed.Load() {
		return
	}
	w.mu.Lock()
	w.deleteDownTrack(layer, id)
	w.mu.Unlock()
}

func (w *WebRTCReceiver) deleteDownTrack(layer int, id string) {
	dts := w.downTracks[layer].Load().([]DownTrack)
	ndts := make([]DownTrack, 0, len(dts))

	for _, dt := range dts {
		if dt.ID() != id {
			ndts = append(ndts, dt)
		} else {
			dt.Close()
		}
	}

	w.downTracks[layer].Store(ndts)
}

func (w *WebRTCReceiver) SendRTCP(p []rtcp.Packet) {
	if _, ok := p[0].(*rtcp.PictureLossIndication); ok {
		if time.Now().UnixNano()-atomic.LoadInt64(&w.lastPli) < 500e6 {
			return
		}
		atomic.StoreInt64(&w.lastPli, time.Now().UnixNano())
	}

	w.rtcpCh <- p
}

func (w *WebRTCReceiver) SetRTCPCh(ch chan []rtcp.Packet) {
	w.rtcpCh = ch
}

func (w *WebRTCReceiver) GetSenderReportTime(layer int) (rtpTS uint32, ntpTS uint64) {
	rtpTS, ntpTS, _ = w.buffers[layer].GetSenderReportData()
	return
}

func (w *WebRTCReceiver) RetransmitPackets(track DownTrack, packets []packetMeta) error {
	if w.nackWorker.Stopped() {
		return io.ErrClosedPipe
	}

	w.nackWorker.Submit(func() {
		src := packetFactory.Get().(*[]byte)
		defer packetFactory.Put(src)

		for _, meta := range packets {
			pktBuff := *src
			if err := w.processRetransmitPacket(meta, track, pktBuff); err != nil {
				if err == io.EOF {
					break
				}
				// Continue processing other packets on error
				continue
			}
		}
	})

	return nil
}

func (w *WebRTCReceiver) writeRTP(layer int) {
	defer func() {
		w.closeOnce.Do(func() {
			w.closed.Store(true)
			w.closeTracks()
		})
	}()

	pli := w.createPLIPacket(layer)

	for {
		pkt, err := w.buffers[layer].ReadExtended()
		if err == io.EOF {
			return
		}

		w.processSimulcastLayerSwitching(layer, pkt, pli)
		w.distributePacketToDownTracks(layer, pkt)
	}
}

// closeTracks close all tracks from Receiver
func (w *WebRTCReceiver) closeTracks() {
	for i := range w.available {
		if !w.available[i].Load() {
			continue
		}

		for _, dt := range w.downTracks[i].Load().([]DownTrack) {
			dt.Close()
		}
	}
	w.nackWorker.StopWait()
	if w.onCloseHandler != nil {
		w.onCloseHandler()
	}
}

func (w *WebRTCReceiver) isDownTrackSubscribed(layer int, dt DownTrack) bool {
	dts := w.downTracks[layer].Load().([]DownTrack)
	return slices.Contains(dts, dt)
}

func (w *WebRTCReceiver) storeDownTrack(layer int, dt DownTrack) {
	dts := w.downTracks[layer].Load().([]DownTrack)
	ndts := make([]DownTrack, len(dts)+1)
	copy(ndts, dts)
	ndts[len(ndts)-1] = dt
	w.downTracks[layer].Store(ndts)
}
