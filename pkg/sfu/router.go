package sfu

import (
	"sync"
	"sync/atomic"

	"github.com/HMasataka/choice/pkg/buffer"
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

	userID    string
	rtcpCh    chan []rtcp.Packet
	stopCh    chan struct{}
	receivers map[string]Receiver

	bufferFactory *buffer.Factory

	config RouterConfig

	session Session

	onAddTrack atomic.Value // func(Receiver)
	onDelTrack atomic.Value // func(Receiver)

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
	WithStats           bool            `mapstructure:"withstats"`
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
	return Receiver{}, false
}

func (r *router) AddDownTracks(s *Subscriber, recv Receiver) error {
	return nil
}

func (r *router) SetRTCPWriter(fn func(packet []rtcp.Packet) error) {
}

func (r *router) AddDownTrack(sub *Subscriber, recv Receiver) (*DownTrack, error) {
	return nil, nil
}
