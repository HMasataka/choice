package sfu

import (
	"github.com/pion/webrtc/v4"
)

// subscriberはDownTrackから受信したメディアをクライアントに送信するための抽象化された構造体です。
// subscriberはクライアントと1対1の関係にあります。

type Subscriber interface {
	GetUserID() string
	GetPeerConnection() *webrtc.PeerConnection
	AddDatachannel(peer Peer, dc *Datachannel) error
	DataChannel(label string) *webrtc.DataChannel
	OnNegotiationNeeded(f func())
	CreateOffer() (webrtc.SessionDescription, error)
	OnICECandidate(f func(c *webrtc.ICECandidate))
	AddICECandidate(candidate webrtc.ICECandidateInit) error
	AddDownTrack(streamID string, downTrack DownTrack)
	RemoveDownTrack(streamID string, downTrack DownTrack)
	AddDataChannel(label string) (*webrtc.DataChannel, error)
	SetRemoteDescription(desc webrtc.SessionDescription) error
	RegisterDatachannel(label string, dc *webrtc.DataChannel)
	GetDatachannel(label string) *webrtc.DataChannel
	DownTracks() []*DownTrack
	GetDownTracks(streamID string) []DownTrack
	Negotiate()
	Close() error
	IsAutoSubscribe() bool
	GetMediaEngine() *webrtc.MediaEngine
	SendStreamDownTracksReports(streamID string)
}

type subscriber struct {
	userID    string
	negotiate func()

	mediaEngine *webrtc.MediaEngine
	pc          *webrtc.PeerConnection

	isAutoSubscribe bool
}

func NewSubscriber(isAutoSubscribe bool) *subscriber {
	return &subscriber{
		isAutoSubscribe: isAutoSubscribe,
	}
}

func (s *subscriber) GetPeerConnection() *webrtc.PeerConnection {
	return s.pc
}

func (s *subscriber) GetUserID() string {
	return s.userID
}

func (s *subscriber) AddDatachannel(peer Peer, dc *Datachannel) error {
	return nil
}

// DataChannel returns the channel for a label
func (s *subscriber) DataChannel(label string) *webrtc.DataChannel {
	return nil
}

func (s *subscriber) OnNegotiationNeeded(f func()) {
}

func (s *subscriber) CreateOffer() (webrtc.SessionDescription, error) {
	return webrtc.SessionDescription{}, nil
}

// OnICECandidate handler
func (s *subscriber) OnICECandidate(f func(c *webrtc.ICECandidate)) {
}

// AddICECandidate to peer connection
func (s *subscriber) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return nil
}

func (s *subscriber) AddDownTrack(streamID string, downTrack DownTrack) {
}

func (s *subscriber) RemoveDownTrack(streamID string, downTrack DownTrack) {
}

func (s *subscriber) AddDataChannel(label string) (*webrtc.DataChannel, error) {
	return nil, nil
}

// SetRemoteDescription sets the SessionDescription of the remote peer
func (s *subscriber) SetRemoteDescription(desc webrtc.SessionDescription) error {
	return nil
}

func (s *subscriber) RegisterDatachannel(label string, dc *webrtc.DataChannel) {
}

func (s *subscriber) GetDatachannel(label string) *webrtc.DataChannel {
	return nil
}

func (s *subscriber) DownTracks() []*DownTrack {
	return nil
}

func (s *subscriber) GetDownTracks(streamID string) []DownTrack {
	return nil
}

// Negotiate fires a debounced negotiation request
func (s *subscriber) Negotiate() {
	if s.negotiate != nil {
		s.negotiate()
	}
}

// Close peer
func (s *subscriber) Close() error {
	return nil
}

func (s *subscriber) downTracksReports() {
}

func (s *subscriber) SendStreamDownTracksReports(streamID string) {
}

func (s *subscriber) IsAutoSubscribe() bool {
	return s.isAutoSubscribe
}

func (s *subscriber) GetMediaEngine() *webrtc.MediaEngine {
	return s.mediaEngine
}
