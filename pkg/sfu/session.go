package sfu

/*
Sessionはsfu内でメディアを共有するための抽象化されたインターフェースです。
Sessionは複数のPeerを保持し、Peer間でメディアを交換します。
*/
type Session interface {
	ID() string
	AudioObserver() *AudioObserver
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

func (s *sessionLocal) AudioObserver() *AudioObserver {
	return s.audioObs
}
