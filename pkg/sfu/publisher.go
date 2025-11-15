package sfu

import (
	"github.com/HMasataka/choice/pkg/relay"
	"github.com/pion/webrtc/v4"
)

// Publisherはclientがメディアを送信するための抽象化された構造体です。
// ClientとPublisherは1対1の関係にあり、ClientはPublisherを使用してメディアストリームをsfuに送信します。
type Publisher interface {
	Answer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error)
	GetRouter() Router
	Close()
	OnPublisherTrack(f func(track PublisherTrack))
	OnICECandidate(f func(c *webrtc.ICECandidate))
	OnICEConnectionStateChange(f func(connectionState webrtc.ICEConnectionState))
	SignalingState() webrtc.SignalingState
	PeerConnection() *webrtc.PeerConnection
	Relay(signalFn func(meta relay.PeerMeta, signal []byte) ([]byte, error), options ...func(r *relayPeer)) (*relay.Peer, error)
	PublisherTracks() []PublisherTrack
	AddRelayFanOutDataChannel(label string)
	GetRelayedDataChannels(label string) []*webrtc.DataChannel
	Relayed() bool
	Tracks() []*webrtc.TrackRemote
	AddICECandidate(candidate webrtc.ICECandidateInit) error
}

var _ Publisher = (*publisher)(nil)

type publisher struct {
	userID string
	pc     *webrtc.PeerConnection

	router  Router
	session Session

	cfg *WebRTCTransportConfig
}

func NewPublisher(userID string, session Session, cfg *WebRTCTransportConfig) (*publisher, error) {
	mediaEngine, err := getPublisherMediaEngine()
	if err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine), webrtc.WithSettingEngine(cfg.Setting))
	pc, err := api.NewPeerConnection(cfg.Configuration)
	if err != nil {
		return nil, err
	}

	router := NewRouter(userID, session, cfg)

	return &publisher{
		userID:  userID,
		pc:      pc,
		router:  router,
		session: session,
		cfg:     cfg,
	}, nil
}

type relayPeer struct {
	peer                    *relay.Peer
	dcs                     []*webrtc.DataChannel
	withSRReports           bool
	relayFanOutDataChannels bool
}

type PublisherTrack struct {
	Track    *webrtc.TrackRemote
	Receiver Receiver
	// This will be used in the future for tracks that will be relayed as clients or servers
	// This is for SVC and Simulcast where you will be able to chose if the relayed peer just
	// want a single track (for recording/ processing) or get all the tracks (for load balancing)
	clientRelay bool
}

func (p *publisher) Answer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error)
func (p *publisher) GetRouter() Router
func (p *publisher) Close()
func (p *publisher) OnPublisherTrack(f func(track PublisherTrack))
func (p *publisher) OnICECandidate(f func(c *webrtc.ICECandidate))
func (p *publisher) OnICEConnectionStateChange(f func(connectionState webrtc.ICEConnectionState))
func (p *publisher) SignalingState() webrtc.SignalingState
func (p *publisher) PeerConnection() *webrtc.PeerConnection
func (p *publisher) Relay(signalFn func(meta relay.PeerMeta, signal []byte) ([]byte, error), options ...func(r *relayPeer)) (*relay.Peer, error)
func (p *publisher) PublisherTracks() []PublisherTrack
func (p *publisher) AddRelayFanOutDataChannel(label string)
func (p *publisher) GetRelayedDataChannels(label string) []*webrtc.DataChannel
func (p *publisher) Relayed() bool
func (p *publisher) Tracks() []*webrtc.TrackRemote
func (p *publisher) AddICECandidate(candidate webrtc.ICECandidateInit) error
