package sfu

import (
	"github.com/pion/webrtc/v4"
)

// SubscriberはDownTrackから受信したメディアをクライアントに送信するための抽象化された構造体です。
// Subscriberはクライアントと1対1の関係にあります。
type Subscriber struct {
	userID    string
	negotiate func()

	mediaEngine *webrtc.MediaEngine
	pc          *webrtc.PeerConnection

	NoAutoSubscribe bool
}

func NewSubscriber() *Subscriber {
	return &Subscriber{}
}

func (s *Subscriber) AddDatachannel(peer Peer, dc *Datachannel) error {
	return nil
}

// DataChannel returns the channel for a label
func (s *Subscriber) DataChannel(label string) *webrtc.DataChannel {
	return nil
}

func (s *Subscriber) OnNegotiationNeeded(f func()) {
}

func (s *Subscriber) CreateOffer() (webrtc.SessionDescription, error) {
	return webrtc.SessionDescription{}, nil
}

// OnICECandidate handler
func (s *Subscriber) OnICECandidate(f func(c *webrtc.ICECandidate)) {
}

// AddICECandidate to peer connection
func (s *Subscriber) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return nil
}

func (s *Subscriber) AddDownTrack(streamID string, downTrack DownTrack) {
}

func (s *Subscriber) RemoveDownTrack(streamID string, downTrack DownTrack) {
}

func (s *Subscriber) AddDataChannel(label string) (*webrtc.DataChannel, error) {
	return nil, nil
}

// SetRemoteDescription sets the SessionDescription of the remote peer
func (s *Subscriber) SetRemoteDescription(desc webrtc.SessionDescription) error {
	return nil
}

func (s *Subscriber) RegisterDatachannel(label string, dc *webrtc.DataChannel) {
}

func (s *Subscriber) GetDatachannel(label string) *webrtc.DataChannel {
	return nil
}

func (s *Subscriber) DownTracks() []*DownTrack {
	return nil
}

func (s *Subscriber) GetDownTracks(streamID string) []DownTrack {
	return nil
}

// Negotiate fires a debounced negotiation request
func (s *Subscriber) Negotiate() {
	s.negotiate()
}

// Close peer
func (s *Subscriber) Close() error {
	return nil
}

func (s *Subscriber) downTracksReports() {
}

func (s *Subscriber) sendStreamDownTracksReports(streamID string) {
}
