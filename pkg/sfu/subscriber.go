package sfu

import (
	"log"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Subscriber handles the subscribing (downstream) connection to a client.
// It sends media tracks from publishers to the client.
type Subscriber struct {
	peer       *Peer
	pc         *webrtc.PeerConnection
	downTracks map[string]*DownTrack
	routers    map[*Router]struct{}
	mu         sync.RWMutex
	closed     bool

	// Negotiation state
	negotiating bool
	needsOffer  bool
	negMu       sync.Mutex
}

func newSubscriber(peer *Peer) (*Subscriber, error) {
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

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		peer.SendCandidate(c, "subscriber")
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			s.Close()
		}
	})

	return s, nil
}

// PeerConnection returns the underlying WebRTC peer connection.
func (s *Subscriber) PeerConnection() *webrtc.PeerConnection {
	return s.pc
}

// Subscribe subscribes to a router to receive its media tracks.
func (s *Subscriber) Subscribe(router *Router) error {
	s.mu.Lock()
	s.routers[router] = struct{}{}
	s.mu.Unlock()

	log.Printf("[Subscriber] Subscribing to router %s", router.ID())

	if err := router.Subscribe(s); err != nil {
		log.Printf("[Subscriber] Error subscribing: %v", err)
		return err
	}

	return s.Negotiate()
}

// AddDownTrack adds a downtrack for a receiver.
func (s *Subscriber) AddDownTrack(receiver *Receiver) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	if _, exists := s.downTracks[receiver.TrackID()]; exists {
		return nil
	}

	dt, err := NewDownTrack(s, receiver)
	if err != nil {
		return err
	}

	s.downTracks[receiver.TrackID()] = dt
	receiver.AddDownTrack(dt)
	return nil
}

// AddICECandidate adds an ICE candidate to the subscriber connection.
func (s *Subscriber) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return s.pc.AddICECandidate(candidate)
}

// Negotiate initiates SDP negotiation with the client.
func (s *Subscriber) Negotiate() error {
	s.negMu.Lock()

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		s.negMu.Unlock()
		return nil
	}
	s.mu.RUnlock()

	// If already negotiating, queue another negotiation
	if s.negotiating {
		s.needsOffer = true
		s.negMu.Unlock()
		log.Printf("[Subscriber] Negotiation in progress, will renegotiate later")
		return nil
	}

	s.negotiating = true
	s.needsOffer = false
	s.negMu.Unlock()

	return s.doNegotiate()
}

func (s *Subscriber) doNegotiate() error {
	log.Printf("[Subscriber] Creating offer")

	offer, err := s.pc.CreateOffer(nil)
	if err != nil {
		s.resetNegotiationState()
		return err
	}

	if err := s.pc.SetLocalDescription(offer); err != nil {
		s.resetNegotiationState()
		return err
	}

	log.Printf("[Subscriber] Sending offer to peer")
	return s.peer.SendOffer(offer)
}

// HandleAnswer processes an SDP answer from the client.
func (s *Subscriber) HandleAnswer(answer webrtc.SessionDescription) error {
	log.Printf("[Subscriber] Setting remote description")

	if err := s.pc.SetRemoteDescription(answer); err != nil {
		return err
	}

	// Check if renegotiation is needed
	s.negMu.Lock()
	s.negotiating = false
	needsOffer := s.needsOffer
	s.needsOffer = false
	s.negMu.Unlock()

	if needsOffer {
		log.Printf("[Subscriber] Pending negotiation, triggering renegotiation")
		go s.Negotiate()
	}

	return nil
}

func (s *Subscriber) resetNegotiationState() {
	s.negMu.Lock()
	s.negotiating = false
	s.negMu.Unlock()
}

// Close closes the subscriber and unsubscribes from all routers.
func (s *Subscriber) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true

	// Copy routers to unsubscribe from
	routers := make([]*Router, 0, len(s.routers))
	for router := range s.routers {
		routers = append(routers, router)
	}
	s.routers = make(map[*Router]struct{})

	// Copy downtracks to close
	downTracks := make([]*DownTrack, 0, len(s.downTracks))
	for _, dt := range s.downTracks {
		downTracks = append(downTracks, dt)
	}
	s.mu.Unlock()

	// Unsubscribe from all routers
	for _, router := range routers {
		router.Unsubscribe(s)
	}

	// Close all downtracks
	for _, dt := range downTracks {
		dt.Close()
	}

	return s.pc.Close()
}
