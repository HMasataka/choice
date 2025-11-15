package sfu

import "github.com/pion/webrtc/v4"

/*
Sessionはsfu内でメディアを共有するための抽象化されたインターフェースです。
Sessionは複数のPeerを保持し、Peer間でメディアを交換します。
*/
type Session interface {
	ID() string
	Publish(router Router, r Receiver)
	Subscribe(peer Peer)
	AddPeer(peer Peer)
	GetPeer(peerID string) Peer
	RemovePeer(peer Peer)
	AddRelayPeer(peerID string, signalData []byte) ([]byte, error)
	AudioObserver() *AudioObserver
	AddDatachannel(owner string, dc *webrtc.DataChannel)
	GetDCMiddlewares() []*Datachannel
	GetFanOutDataChannelLabels() []string
	GetDataChannels(peerID, label string) (dcs []*webrtc.DataChannel)
	FanOutMessage(origin, label string, msg webrtc.DataChannelMessage)
	Peers() []Peer
	RelayPeers() []*RelayPeer
}

var _ Session = (*sessionLocal)(nil)

func NewSession(id string) Session {
	return &sessionLocal{id: id}
}

type sessionLocal struct {
	id       string
	audioObs *AudioObserver
}

func (s *sessionLocal) ID() string {
	return s.id
}

func (s *sessionLocal) Publish(router Router, r Receiver)
func (s *sessionLocal) Subscribe(peer Peer)
func (s *sessionLocal) AddPeer(peer Peer)
func (s *sessionLocal) GetPeer(peerID string) Peer
func (s *sessionLocal) RemovePeer(peer Peer)
func (s *sessionLocal) AddRelayPeer(peerID string, signalData []byte) ([]byte, error)

func (s *sessionLocal) AudioObserver() *AudioObserver {
	return s.audioObs
}

func (s *sessionLocal) AddDatachannel(owner string, dc *webrtc.DataChannel)
func (s *sessionLocal) GetDCMiddlewares() []*Datachannel
func (s *sessionLocal) GetFanOutDataChannelLabels() []string
func (s *sessionLocal) GetDataChannels(peerID, label string) (dcs []*webrtc.DataChannel)
func (s *sessionLocal) FanOutMessage(origin, label string, msg webrtc.DataChannelMessage)
func (s *sessionLocal) Peers() []Peer
func (s *sessionLocal) RelayPeers() []*RelayPeer
