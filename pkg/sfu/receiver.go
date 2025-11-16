package sfu

import (
	"io"
	"math/rand"
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

func (w *WebRTCReceiver) AddUpTrack(track *webrtc.TrackRemote, buff *buffer.Buffer, bestQualityFirst bool) {
	if w.closed.Load() {
		return
	}

	var layer int
	switch track.RID() {
	case fullResolution:
		layer = 2
	case halfResolution:
		layer = 1
	default:
		layer = 0
	}

	w.Lock()
	w.upTracks[layer] = track
	w.buffers[layer] = buff
	w.available[layer].Store(true)
	w.downTracks[layer].Store(make([]*DownTrack, 0, 10))
	w.pendingTracks[layer] = make([]DownTrack, 0, 10)
	w.Unlock()

	subBestQuality := func(targetLayer int) {
		for l := 0; l < targetLayer; l++ {
			dts := w.downTracks[l].Load()
			if dts == nil {
				continue
			}
			for _, dt := range dts.([]DownTrack) {
				_ = dt.SwitchSpatialLayer(int32(targetLayer), false)
			}
		}
	}

	subLowestQuality := func(targetLayer int) {
		for l := 2; l != targetLayer; l-- {
			dts := w.downTracks[l].Load()
			if dts == nil {
				continue
			}
			for _, dt := range dts.([]DownTrack) {
				_ = dt.SwitchSpatialLayer(int32(targetLayer), false)
			}
		}
	}

	if w.isSimulcast {
		if bestQualityFirst && (!w.available[2].Load() || layer == 2) {
			subBestQuality(layer)
		} else if !bestQualityFirst && (!w.available[0].Load() || layer == 0) {
			subLowestQuality(layer)
		}
	}

	go w.writeRTP(layer)
}

func (w *WebRTCReceiver) AddDownTrack(track DownTrack, bestQualityFirst bool) {
	if w.closed.Load() {
		return
	}

	layer := 0
	if w.isSimulcast {
		for i, t := range w.available {
			if t.Load() {
				layer = i
				if !bestQualityFirst {
					break
				}
			}
		}
		if w.isDownTrackSubscribed(layer, track) {
			return
		}
		track.SetInitialLayers(int32(layer), 2)
		track.SetMaxSpatialLayer(2)
		track.SetMaxTemporalLayer(2)
		track.SetLastSSRC(w.SSRC(layer))
		track.SetTrackType(SimulcastDownTrack)
		track.SetPayload(packetFactory.Get().(*[]byte))
	} else {
		if w.isDownTrackSubscribed(layer, track) {
			return
		}
		track.SetInitialLayers(0, 0)
		track.SetTrackType(SimpleDownTrack)
	}
	w.Lock()
	w.storeDownTrack(layer, track)
	w.Unlock()
}

func (w *WebRTCReceiver) SwitchDownTrack(track DownTrack, layer int) error {
	if w.closed.Load() {
		return errNoReceiverFound
	}
	if w.available[layer].Load() {
		w.Lock()
		w.pending[layer].Store(true)
		w.pendingTracks[layer] = append(w.pendingTracks[layer], track)
		w.Unlock()
		return nil
	}
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
	w.Lock()
	w.deleteDownTrack(layer, id)
	w.Unlock()
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
		for _, meta := range packets {
			pktBuff := *src
			buff := w.buffers[meta.layer]
			if buff == nil {
				break
			}
			i, err := buff.GetPacket(pktBuff, meta.sourceSeqNo)
			if err != nil {
				if err == io.EOF {
					break
				}
				continue
			}
			var pkt rtp.Packet
			if err = pkt.Unmarshal(pktBuff[:i]); err != nil {
				continue
			}
			pkt.Header.SequenceNumber = meta.targetSeqNo
			pkt.Header.Timestamp = meta.timestamp
			pkt.Header.SSRC = track.GetSSRC()
			pkt.Header.PayloadType = track.GetPayloadType()
			if track.GetSimulcast().temporalSupported {
				switch track.GetMime() {
				case "video/vp8":
					var vp8 buffer.VP8
					if err = vp8.Unmarshal(pkt.Payload); err != nil {
						continue
					}
					tlzoID, picID := meta.getVP8PayloadMeta()
					modifyVP8TemporalPayload(pkt.Payload, vp8.PicIDIdx, vp8.TlzIdx, picID, tlzoID, vp8.MBit)
				}
			}

			if _, err = track.GetWriteStream().WriteRTP(&pkt.Header, pkt.Payload); err != nil {
				// TODO log
			} else {
				track.UpdateStats(uint32(i))
			}
		}
		packetFactory.Put(src)
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

	pli := []rtcp.Packet{
		&rtcp.PictureLossIndication{SenderSSRC: rand.Uint32(), MediaSSRC: w.SSRC(layer)},
	}

	for {
		pkt, err := w.buffers[layer].ReadExtended()
		if err == io.EOF {
			return
		}

		if w.isSimulcast {
			if w.pending[layer].Load() {
				if pkt.KeyFrame {
					w.Lock()
					for idx, dt := range w.pendingTracks[layer] {
						w.deleteDownTrack(dt.CurrentSpatialLayer(), dt.ID())
						w.storeDownTrack(layer, dt)
						dt.SwitchSpatialLayerDone(int32(layer))
						w.pendingTracks[layer][idx] = nil
					}
					w.pendingTracks[layer] = w.pendingTracks[layer][:0]
					w.pending[layer].Store(false)
					w.Unlock()
				} else {
					w.SendRTCP(pli)
				}
			}
		}

		for _, dt := range w.downTracks[layer].Load().([]DownTrack) {
			if err = dt.WriteRTP(pkt, layer); err != nil {
				if err == io.EOF || err == io.ErrClosedPipe {
					w.Lock()
					w.deleteDownTrack(layer, dt.ID())
					w.Unlock()
				}
			}
		}
	}

}

// closeTracks close all tracks from Receiver
func (w *WebRTCReceiver) closeTracks() {
	for idx, a := range w.available {
		if !a.Load() {
			continue
		}

		for _, dt := range w.downTracks[idx].Load().([]DownTrack) {
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

	for _, cdt := range dts {
		if cdt == dt {
			return true
		}
	}

	return false
}

func (w *WebRTCReceiver) storeDownTrack(layer int, dt DownTrack) {
	dts := w.downTracks[layer].Load().([]DownTrack)
	ndts := make([]DownTrack, len(dts)+1)
	copy(ndts, dts)
	ndts[len(ndts)-1] = dt
	w.downTracks[layer].Store(ndts)
}
