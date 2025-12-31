package sfu

import (
	"log"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Subscriber handles the subscribing (downstream) connection to a client.
// It sends media tracks from publishers to the client.
type Subscriber struct {
	peer              *Peer
	pc                *webrtc.PeerConnection
	downTracks        map[string]*DownTrack
	simulcastTracks   map[string]*SimulcastDownTrack
	routers           map[*Router]struct{}
	mu                sync.RWMutex
	closed            bool

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
		peer:            peer,
		pc:              pc,
		downTracks:      make(map[string]*DownTrack),
		simulcastTracks: make(map[string]*SimulcastDownTrack),
		routers:         make(map[*Router]struct{}),
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

// AddSimulcastDownTrack adds a simulcast downtrack for a simulcast receiver.
func (s *Subscriber) AddSimulcastDownTrack(simulcastRecv *SimulcastReceiver) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	trackID := simulcastRecv.TrackID()
	if _, exists := s.simulcastTracks[trackID]; exists {
		return nil
	}

	// Get codec from the best available layer
	bestLayer := simulcastRecv.GetBestLayer()
	if bestLayer == nil {
		log.Printf("[Subscriber] No layers available for simulcast track %s", trackID)
		return nil
	}

	codec := bestLayer.Receiver().Codec()

	dt, err := NewSimulcastDownTrack(s, simulcastRecv, codec)
	if err != nil {
		return err
	}

	s.simulcastTracks[trackID] = dt
	simulcastRecv.AddDownTrack(dt)

	log.Printf("[Subscriber] Added simulcast downtrack for %s", trackID)
	return nil
}

// SetSimulcastLayer sets the target layer for a simulcast track.
// If trackID is not found, it tries to set the layer on all simulcast tracks (for client compatibility).
func (s *Subscriber) SetSimulcastLayer(trackID, layer string) {
	s.mu.RLock()
	dt, exists := s.simulcastTracks[trackID]

	// If not found by exact ID, set layer on all simulcast tracks
	// This handles the case where client sends a different track ID
	if !exists && len(s.simulcastTracks) > 0 {
		log.Printf("[Subscriber] Track %s not found, applying layer %s to all %d simulcast tracks",
			trackID, layer, len(s.simulcastTracks))
		for id, track := range s.simulcastTracks {
			track.SetTargetLayer(layer)
			log.Printf("[Subscriber] Set target layer %s for track %s", layer, id)
		}
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	if !exists {
		log.Printf("[Subscriber] No simulcast tracks available")
		return
	}

	dt.SetTargetLayer(layer)
	log.Printf("[Subscriber] Set target layer %s for track %s", layer, trackID)
}

// GetSimulcastLayer returns the current layer for a simulcast track.
func (s *Subscriber) GetSimulcastLayer(trackID string) (current, target string, ok bool) {
	s.mu.RLock()
	dt, exists := s.simulcastTracks[trackID]
	s.mu.RUnlock()

	if !exists {
		return "", "", false
	}

	return dt.GetCurrentLayer(), dt.GetTargetLayer(), true
}

// GetSimulcastDownTrack returns the simulcast downtrack for a track ID.
func (s *Subscriber) GetSimulcastDownTrack(trackID string) *SimulcastDownTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.simulcastTracks[trackID]
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

	// Copy simulcast tracks to close
	simulcastTracks := make([]*SimulcastDownTrack, 0, len(s.simulcastTracks))
	for _, dt := range s.simulcastTracks {
		simulcastTracks = append(simulcastTracks, dt)
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

	// Close all simulcast tracks
	for _, dt := range simulcastTracks {
		dt.Close()
	}

	return s.pc.Close()
}
