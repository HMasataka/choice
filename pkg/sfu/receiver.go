package sfu

import (
	"sync"
	"sync/atomic"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/gammazero/workerpool"
	"github.com/pion/rtcp"
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
	RetransmitPackets(track *DownTrack, packets []packetMeta) error
	DeleteDownTrack(layer int, id string)
	OnCloseHandler(fn func())
	SendRTCP(p []rtcp.Packet)
	SetRTCPCh(ch chan []rtcp.Packet)
	GetSenderReportTime(layer int) (rtpTS uint32, ntpTS uint64)
}

type WebRTCReceiver struct {
	sync.Mutex
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
	pendingTracks  [3][]*DownTrack
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

func (w *WebRTCReceiver) AddUpTrack(track *webrtc.TrackRemote, buff *buffer.Buffer, bestQualityFirst bool) {
}

func (w *WebRTCReceiver) AddDownTrack(track DownTrack, bestQualityFirst bool) {
}

func (w *WebRTCReceiver) SwitchDownTrack(track DownTrack, layer int) error {
	return nil
}

func (w *WebRTCReceiver) GetBitrate() [3]uint64 {
	var br [3]uint64
	return br
}

func (w *WebRTCReceiver) GetMaxTemporalLayer() [3]int32 {
	var tls [3]int32
	return tls
}

// OnCloseHandler method to be called on remote tracked removed
func (w *WebRTCReceiver) OnCloseHandler(fn func()) {
	w.onCloseHandler = fn
}

// DeleteDownTrack removes a DownTrack from a Receiver
func (w *WebRTCReceiver) DeleteDownTrack(layer int, id string) {
}

func (w *WebRTCReceiver) deleteDownTrack(layer int, id string) {
}

func (w *WebRTCReceiver) SendRTCP(p []rtcp.Packet) {
}

func (w *WebRTCReceiver) SetRTCPCh(ch chan []rtcp.Packet) {
	w.rtcpCh = ch
}

func (w *WebRTCReceiver) GetSenderReportTime(layer int) (rtpTS uint32, ntpTS uint64) {
	return
}

func (w *WebRTCReceiver) RetransmitPackets(track *DownTrack, packets []packetMeta) error {
	return nil
}

func (w *WebRTCReceiver) writeRTP(layer int) {
}

// closeTracks close all tracks from Receiver
func (w *WebRTCReceiver) closeTracks() {
}

func (w *WebRTCReceiver) isDownTrackSubscribed(layer int, dt *DownTrack) bool {
	return false
}

func (w *WebRTCReceiver) storeDownTrack(layer int, dt *DownTrack) {
}
