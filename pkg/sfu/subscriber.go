package sfu

import (
	"sync"

	"github.com/pion/webrtc/v4"
)

type Subscriber struct {
	peer       *PeerConnection
	pc         *webrtc.PeerConnection
	downTracks map[string]*DownTrack
	mu         sync.RWMutex
	closed     bool
	negotiate  chan struct{}
}

func NewSubscriber(peer *PeerConnection) (*Subscriber, error) {
	pc, err := peer.session.sfu.NewPeerConnection()
	if err != nil {
		return nil, err
	}

	s := &Subscriber{
		peer:       peer,
		pc:         pc,
		downTracks: make(map[string]*DownTrack),
		negotiate:  make(chan struct{}, 1),
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		peer.SendCandidate(candidate, "subscriber")
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			s.Close()
		}
	})

	pc.OnNegotiationNeeded(func() {
		s.Negotiate()
	})

	return s, nil
}

func (s *Subscriber) PeerConnection() *webrtc.PeerConnection {
	return s.pc
}

func (s *Subscriber) Subscribe(router *Router) error {
	receivers := router.GetReceivers()

	for _, receiver := range receivers {
		if err := s.AddDownTrack(receiver); err != nil {
			return err
		}
	}

	return nil
}

func (s *Subscriber) AddDownTrack(receiver *Receiver) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	if _, exists := s.downTracks[receiver.TrackID()]; exists {
		return nil
	}

	downTrack, err := NewDownTrack(s, receiver)
	if err != nil {
		return err
	}

	s.downTracks[receiver.TrackID()] = downTrack
	receiver.AddDownTrack(downTrack)

	return nil
}

func (s *Subscriber) RemoveDownTrack(trackID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if dt, exists := s.downTracks[trackID]; exists {
		dt.Close()
		delete(s.downTracks, trackID)
	}
}

func (s *Subscriber) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return s.pc.AddICECandidate(candidate)
}

func (s *Subscriber) SetRemoteDescription(sdp webrtc.SessionDescription) error {
	return s.pc.SetRemoteDescription(sdp)
}

func (s *Subscriber) Negotiate() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	offer, err := s.pc.CreateOffer(nil)
	if err != nil {
		return err
	}

	if err := s.pc.SetLocalDescription(offer); err != nil {
		return err
	}

	return s.peer.SendOffer(offer)
}

func (s *Subscriber) HandleAnswer(answer webrtc.SessionDescription) error {
	return s.pc.SetRemoteDescription(answer)
}

func (s *Subscriber) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	for _, dt := range s.downTracks {
		dt.Close()
	}

	return s.pc.Close()
}
