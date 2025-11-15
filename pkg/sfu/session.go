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

func (s *sessionLocal) Publish(router Router, r Receiver) {
	// TODO: Implement publish logic
}

func (s *sessionLocal) Subscribe(peer Peer) {
	// TODO: Implement subscribe logic
}

func (s *sessionLocal) AddPeer(peer Peer) {
	// TODO: Implement add peer logic
}

func (s *sessionLocal) GetPeer(peerID string) Peer {
	// TODO: Implement get peer logic
	return nil
}

func (s *sessionLocal) RemovePeer(peer Peer) {
	// TODO: Implement remove peer logic
}

func (s *sessionLocal) AddRelayPeer(peerID string, signalData []byte) ([]byte, error) {
	// TODO: Implement relay peer logic
	return nil, nil
}

func (s *sessionLocal) AudioObserver() *AudioObserver {
	return s.audioObs
}

func (s *sessionLocal) AddDatachannel(owner string, dc *webrtc.DataChannel) {
	// TODO: Implement add data channel logic
}

func (s *sessionLocal) GetDCMiddlewares() []*Datachannel {
	// TODO: Implement get DC middlewares logic
	return nil
}

func (s *sessionLocal) GetFanOutDataChannelLabels() []string {
	// TODO: Implement get fan out data channel labels logic
	return nil
}

func (s *sessionLocal) GetDataChannels(peerID, label string) (dcs []*webrtc.DataChannel) {
	// TODO: Implement get data channels logic
	return nil
}

func (s *sessionLocal) FanOutMessage(origin, label string, msg webrtc.DataChannelMessage) {
	// TODO: Implement fan out message logic
}

func (s *sessionLocal) Peers() []Peer {
	// TODO: Implement get peers logic
	return nil
}

func (s *sessionLocal) RelayPeers() []*RelayPeer {
	// TODO: Implement get relay peers logic
	return nil
}
