package sfu

import (
	"log"
	"sync"

	"github.com/pion/webrtc/v4"
)

type Subscriber struct {
	peer        *PeerConnection
	pc          *webrtc.PeerConnection
	downTracks  map[string]*DownTrack
	routers     map[*Router]struct{}
	mu          sync.RWMutex
	closed      bool
	negotiating bool
	needsOffer  bool
	negMu       sync.Mutex
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
		routers:    make(map[*Router]struct{}),
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

	// Note: We handle negotiation explicitly in Subscribe(), not via OnNegotiationNeeded
	// to avoid race conditions with multiple tracks being added

	return s, nil
}

func (s *Subscriber) PeerConnection() *webrtc.PeerConnection {
	return s.pc
}

func (s *Subscriber) Subscribe(router *Router) error {
	s.mu.Lock()
	s.routers[router] = struct{}{}
	s.mu.Unlock()

	log.Printf("Subscribe: subscribing to router %s", router.ID())

	// Use router's Subscribe method which tracks the subscriber
	if err := router.Subscribe(s); err != nil {
		log.Printf("Subscribe: error subscribing to router: %v", err)
		return err
	}

	// Explicitly trigger negotiation after adding tracks
	log.Printf("Subscribe: triggering negotiation")
	return s.Negotiate()
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
	s.negMu.Lock()

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		s.negMu.Unlock()
		log.Printf("Negotiate: subscriber is closed, skipping")
		return nil
	}
	s.mu.RUnlock()

	// If already negotiating, mark that we need another offer
	if s.negotiating {
		s.needsOffer = true
		s.negMu.Unlock()
		log.Printf("Negotiate: already negotiating, will renegotiate later")
		return nil
	}

	s.negotiating = true
	s.needsOffer = false
	s.negMu.Unlock()

	return s.doNegotiate()
}

func (s *Subscriber) doNegotiate() error {
	log.Printf("Negotiate: creating offer")
	offer, err := s.pc.CreateOffer(nil)
	if err != nil {
		s.negMu.Lock()
		s.negotiating = false
		s.negMu.Unlock()
		log.Printf("Negotiate: error creating offer: %v", err)
		return err
	}

	log.Printf("Negotiate: setting local description")
	if err := s.pc.SetLocalDescription(offer); err != nil {
		s.negMu.Lock()
		s.negotiating = false
		s.negMu.Unlock()
		log.Printf("Negotiate: error setting local description: %v", err)
		return err
	}

	log.Printf("Negotiate: sending offer to peer")
	return s.peer.SendOffer(offer)
}

func (s *Subscriber) HandleAnswer(answer webrtc.SessionDescription) error {
	log.Printf("HandleAnswer: setting remote description")
	if err := s.pc.SetRemoteDescription(answer); err != nil {
		log.Printf("HandleAnswer: error setting remote description: %v", err)
		return err
	}

	// Check if we need to renegotiate
	s.negMu.Lock()
	s.negotiating = false
	needsOffer := s.needsOffer
	s.needsOffer = false
	s.negMu.Unlock()

	if needsOffer {
		log.Printf("HandleAnswer: pending negotiation, triggering renegotiation")
		go s.Negotiate()
	}

	return nil
}

func (s *Subscriber) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true

	// Copy routers to unsubscribe from
	routersToUnsubscribe := make([]*Router, 0, len(s.routers))
	for router := range s.routers {
		routersToUnsubscribe = append(routersToUnsubscribe, router)
	}
	s.routers = make(map[*Router]struct{})
	s.mu.Unlock()

	// Unsubscribe from all routers
	for _, router := range routersToUnsubscribe {
		router.Unsubscribe(s)
	}

	for _, dt := range s.downTracks {
		dt.Close()
	}

	return s.pc.Close()
}
