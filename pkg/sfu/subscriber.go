package sfu

import (
	"log/slog"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Subscriber handles the subscribing (downstream) connection to a client.
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
		if err := peer.SendCandidate(c, "subscriber"); err != nil {
			slog.Warn("send candidate (subscriber) failed", slog.String("error", err.Error()))
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			if err := s.Close(); err != nil {
				slog.Warn("subscriber close error", slog.String("error", err.Error()))
			}
		}
	})

	return s, nil
}

// PeerConnection returns the underlying WebRTC peer connection.
func (s *Subscriber) PeerConnection() *webrtc.PeerConnection {
	return s.pc
}

// Subscribe subscribes to a router to receive its tracks.
func (s *Subscriber) Subscribe(router *Router) error {
	s.mu.Lock()
	s.routers[router] = struct{}{}
	s.mu.Unlock()

	slog.Info("[Subscriber] Subscribing to router", "routerID", router.ID())

	if err := router.Subscribe(s); err != nil {
		slog.Warn("[Subscriber] Error subscribing to router", "error", err)
		return err
	}

	return s.Negotiate()
}

// AddDownTrack adds a downtrack for a track receiver.
func (s *Subscriber) AddDownTrack(track *TrackReceiver) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	trackID := track.TrackID()
	if _, exists := s.downTracks[trackID]; exists {
		return nil
	}

	// Get codec from the best available layer
	bestLayer := track.GetBestLayer()
	if bestLayer == nil {
		slog.Warn("[Subscriber] No layers available for track", "trackID", trackID)
		return nil
	}

	codec := bestLayer.Receiver().Codec()

	dt, err := NewDownTrack(s, track, codec)
	if err != nil {
		return err
	}

	s.downTracks[trackID] = dt
	track.AddDownTrack(dt)

	slog.Info("[Subscriber] Added downtrack", "trackID", trackID)
	return nil
}

// GetDownTrack returns the downtrack for a track ID.
func (s *Subscriber) GetDownTrack(trackID string) *DownTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.downTracks[trackID]
}

// SetLayer sets the target layer for a track.
func (s *Subscriber) SetLayer(trackID, layer string) {
	s.mu.RLock()
	dt, exists := s.downTracks[trackID]
	s.mu.RUnlock()

	if !exists {
		return
	}

	dt.SetTargetLayer(layer)
}

// GetLayer returns the current and target layer for a track.
func (s *Subscriber) GetLayer(trackID string) (current, target string, ok bool) {
	s.mu.RLock()
	dt, exists := s.downTracks[trackID]
	s.mu.RUnlock()

	if !exists {
		return "", "", false
	}

	return dt.GetCurrentLayer(), dt.GetTargetLayer(), true
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

	if s.negotiating {
		s.needsOffer = true
		s.negMu.Unlock()
		slog.Debug("[Subscriber] Negotiation in progress, will renegotiate later")
		return nil
	}

	s.negotiating = true
	s.needsOffer = false
	s.negMu.Unlock()

	return s.doNegotiate()
}

func (s *Subscriber) doNegotiate() error {
	slog.Debug("[Subscriber] Creating offer")

	offer, err := s.pc.CreateOffer(nil)
	if err != nil {
		s.resetNegotiationState()
		return err
	}

	if err := s.pc.SetLocalDescription(offer); err != nil {
		s.resetNegotiationState()
		return err
	}

	slog.Debug("[Subscriber] Sending offer to peer")
	return s.peer.SendOffer(offer)
}

// HandleAnswer processes an SDP answer from the client.
func (s *Subscriber) HandleAnswer(answer webrtc.SessionDescription) error {
	slog.Debug("[Subscriber] Setting remote description")

	if err := s.pc.SetRemoteDescription(answer); err != nil {
		return err
	}

	s.negMu.Lock()
	s.negotiating = false
	needsOffer := s.needsOffer
	s.needsOffer = false
	s.negMu.Unlock()

	if needsOffer {
		slog.Debug("[Subscriber] Pending negotiation, triggering renegotiation")
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

	routers := make([]*Router, 0, len(s.routers))
	for router := range s.routers {
		routers = append(routers, router)
	}
	s.routers = make(map[*Router]struct{})

	downTracks := make([]*DownTrack, 0, len(s.downTracks))
	for _, dt := range s.downTracks {
		downTracks = append(downTracks, dt)
	}
	s.mu.Unlock()

	for _, router := range routers {
		router.Unsubscribe(s)
	}

	for _, dt := range downTracks {
		if err := dt.Close(); err != nil {
			slog.Warn("downtrack close error", slog.String("error", err.Error()))
		}
	}

	return s.pc.Close()
}
