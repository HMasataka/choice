package sfu

import (
	"sync"
	"sync/atomic"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/HMasataka/choice/pkg/twcc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// RouterはReceiverから受信したメディアを適切なDowntrackにルーティングするための抽象化された構造体です。
type Router interface {
	UserID() string
	AddReceiver(receiver *webrtc.RTPReceiver, track *webrtc.TrackRemote, trackID, streamID string) (Receiver, bool)
	AddDownTracks(s *Subscriber, r Receiver) error
	SetRTCPWriter(func([]rtcp.Packet) error)
	AddDownTrack(s *Subscriber, r Receiver) (*DownTrack, error)
	Stop()
	GetReceiver() map[string]Receiver
	OnAddReceiverTrack(f func(receiver Receiver))
	OnDelReceiverTrack(f func(receiver Receiver))
}

var _ Router = (*router)(nil)

type router struct {
	sync.RWMutex

	twcc *twcc.Responder

	userID    string
	rtcpCh    chan []rtcp.Packet
	stopCh    chan struct{}
	receivers map[string]Receiver

	bufferFactory *buffer.Factory

	config RouterConfig

	session Session

	onAddTrack atomic.Value // func(Receiver)
	onDelTrack atomic.Value // func(Receiver)

	writeRTCP func([]rtcp.Packet) error
}

func NewRouter(userID string, session Session, cfg *WebRTCTransportConfig) *router {
	ch := make(chan []rtcp.Packet, 10)

	return &router{
		userID:        userID,
		rtcpCh:        ch,
		stopCh:        make(chan struct{}),
		receivers:     make(map[string]Receiver),
		bufferFactory: cfg.BufferFactory,
		config:        cfg.RouterConfig,
		session:       session,
	}
}

type RouterConfig struct {
	MaxBandwidth        uint64          `mapstructure:"maxbandwidth"`
	MaxPacketTrack      int             `mapstructure:"maxpackettrack"`
	AudioLevelInterval  int             `mapstructure:"audiolevelinterval"`
	AudioLevelThreshold uint8           `mapstructure:"audiolevelthreshold"`
	AudioLevelFilter    int             `mapstructure:"audiolevelfilter"`
	Simulcast           SimulcastConfig `mapstructure:"simulcast"`
}

func (r *router) GetReceiver() map[string]Receiver {
	return r.receivers
}

func (r *router) OnAddReceiverTrack(f func(receiver Receiver)) {
	r.onAddTrack.Store(f)
}

func (r *router) OnDelReceiverTrack(f func(receiver Receiver)) {
	r.onDelTrack.Store(f)
}

func (r *router) UserID() string {
	return r.userID
}

func (r *router) Stop() {
	r.stopCh <- struct{}{}
}

func (r *router) AddReceiver(receiver *webrtc.RTPReceiver, track *webrtc.TrackRemote, trackID, streamID string) (Receiver, bool) {
	r.Lock()
	defer r.Unlock()

	buff := r.setupBuffer(track)

	switch track.Kind() {
	case webrtc.RTPCodecTypeAudio:
		buff.OnAudioLevel(func(level uint8) {
			r.session.AudioObserver().observe(streamID, level)
		})
		r.session.AudioObserver().addStream(streamID)

	case webrtc.RTPCodecTypeVideo:
		if r.twcc == nil {
			r.twcc = twcc.NewTransportWideCCResponder(uint32(track.SSRC()))
			r.twcc.OnFeedback(func(p rtcp.RawPacket) {
				r.rtcpCh <- []rtcp.Packet{&p}
			})
		}
		buff.OnTransportWideCC(func(sn uint16, timeNS int64, marker bool) {
			r.twcc.Push(sn, timeNS, marker)
		})
	}

	recv, publish := r.getReceiver(receiver, track, trackID)
	recv.AddUpTrack(track, buff, r.config.Simulcast.BestQualityFirst)

	buff.Bind(receiver.GetParameters(), buffer.Options{
		MaxBitRate: r.config.MaxBandwidth,
	})

	return recv, publish
}

func (r *router) setupBuffer(track *webrtc.TrackRemote) *buffer.Buffer {
	buff, rtcpReader := r.bufferFactory.GetBufferPair(uint32(track.SSRC()))

	buff.OnFeedback(func(fb []rtcp.Packet) {
		r.rtcpCh <- fb
	})

	rtcpReader.OnPacket(func(bytes []byte) {
		packets, err := rtcp.Unmarshal(bytes)
		if err != nil {
			return
		}

		for _, packet := range packets {
			switch packetType := packet.(type) {
			case *rtcp.SourceDescription:
			case *rtcp.SenderReport:
				buff.SetSenderReportData(packetType.RTPTime, packetType.NTPTime)
			}
		}
	})

	return buff
}

func (r *router) getReceiver(receiver *webrtc.RTPReceiver, track *webrtc.TrackRemote, trackID string) (recv Receiver, publish bool) {
	recv, ok := r.receivers[trackID]
	if ok {
		return recv, false
	}

	recv = NewWebRTCReceiver(receiver, track, r.userID)

	r.receivers[trackID] = recv

	recv.SetRTCPCh(r.rtcpCh)

	recv.OnCloseHandler(func() {
		if recv.Kind() == webrtc.RTPCodecTypeAudio {
			r.session.AudioObserver().removeStream(track.StreamID())
		}

		r.deleteReceiver(trackID, uint32(track.SSRC()))
	})

	if handler, ok := r.onAddTrack.Load().(func(Receiver)); ok && handler != nil {
		handler(recv)
	}

	return recv, true
}

func (r *router) deleteReceiver(track string, ssrc uint32) {
	r.Lock()
	defer r.Unlock()

	if handler, ok := r.onDelTrack.Load().(func(Receiver)); ok && handler != nil {
		handler(r.receivers[track])
	}

	delete(r.receivers, track)
}

func (r *router) AddDownTracks(s *Subscriber, recv Receiver) error {
	return nil
}

func (r *router) SetRTCPWriter(fn func(packet []rtcp.Packet) error) {
}

func (r *router) AddDownTrack(sub *Subscriber, recv Receiver) (*DownTrack, error) {
	return nil, nil
}
