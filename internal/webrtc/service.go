package webrtc

import (
	"context"
	"fmt"
	"sync"

	pion "github.com/pion/webrtc/v4"
)

// EventHandler handles WebRTC events and state changes.
// This interface is implemented by the signaling layer to send notifications to clients.
type EventHandler interface {
	// OnICEConnectionStateChange is called when the ICE connection state changes.
	OnICEConnectionStateChange(participantID string, state pion.ICEConnectionState)
	// OnPeerConnectionStateChange is called when the peer connection state changes.
	OnPeerConnectionStateChange(participantID string, state pion.PeerConnectionState)
	// OnICECandidate is called when a new ICE candidate is available (for trickle ICE).
	OnICECandidate(participantID string, candidate pion.ICECandidateInit)
	// OnTrack is called when a new track is received from the peer.
	OnTrack(participantID string, track *pion.TrackRemote, receiver *pion.RTPReceiver)
	// OnNegotiationNeeded is called when renegotiation is needed.
	OnNegotiationNeeded(participantID string)
}

// Service manages WebRTC peer connections for all participants.
// This is the main coordination layer between signaling and WebRTC.
type Service struct {
	mu     sync.RWMutex
	peers  map[string]*Peer
	events EventHandler

	// Configuration for creating new peers
	config      PeerConfig
	mediaEngine *pion.MediaEngine
}

// NewService creates a new WebRTC service.
func NewService(config PeerConfig, mediaEngine *pion.MediaEngine, events EventHandler) *Service {
	return &Service{
		peers:       make(map[string]*Peer),
		events:      events,
		config:      config,
		mediaEngine: mediaEngine,
	}
}

// HandleOffer processes an SDP offer from a client and returns an SDP answer.
// This method creates a new peer connection if one doesn't exist (lazy creation).
func (s *Service) HandleOffer(ctx context.Context, participantID string, sdp string) (string, error) {
	peer, err := s.getOrCreatePeer(participantID)
	if err != nil {
		return "", err
	}

	return peer.HandleOffer(ctx, sdp)
}

// HandleAnswer processes an SDP answer from a client.
func (s *Service) HandleAnswer(ctx context.Context, participantID string, sdp string) error {
	peer, err := s.getPeer(participantID)
	if err != nil {
		return err
	}

	return peer.HandleAnswer(ctx, sdp)
}

// HandleCandidate processes an ICE candidate from a client.
// The peer must already exist (created via HandleOffer) before candidates can be added.
func (s *Service) HandleCandidate(ctx context.Context, participantID string, candidate string, sdpMid string, sdpMLineIndex *int) error {
	peer, err := s.getPeer(participantID)
	if err != nil {
		return err
	}

	return peer.HandleCandidate(ctx, candidate, sdpMid, sdpMLineIndex)
}

// GetPeer returns the peer for the given participant ID.
// Returns nil if no peer exists.
func (s *Service) GetPeer(participantID string) *Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.peers[participantID]
}

// RemovePeer removes and closes the peer for the given participant ID.
func (s *Service) RemovePeer(participantID string) error {
	s.mu.Lock()
	peer := s.peers[participantID]
	delete(s.peers, participantID)
	s.mu.Unlock()

	if peer != nil {
		return peer.Close()
	}
	return nil
}

// getPeer returns the peer for the given participant ID.
// Returns an error if no peer exists.
func (s *Service) getPeer(participantID string) (*Peer, error) {
	s.mu.RLock()
	peer := s.peers[participantID]
	s.mu.RUnlock()

	if peer == nil {
		return nil, fmt.Errorf("peer not found: %s", participantID)
	}

	return peer, nil
}

// getOrCreatePeer returns the peer for the given participant ID, creating it if necessary.
// Peer creation is lazy - peers are only created when an offer is received.
func (s *Service) getOrCreatePeer(participantID string) (*Peer, error) {
	// Fast path: peer already exists
	s.mu.RLock()
	peer := s.peers[participantID]
	s.mu.RUnlock()

	if peer != nil {
		return peer, nil
	}

	// Slow path: create new peer
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	peer = s.peers[participantID]
	if peer != nil {
		return peer, nil
	}

	// Create new peer
	peer, err := NewPeer(participantID, s.config, s.mediaEngine)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer: %w", err)
	}

	// Register event handlers
	s.registerPeerEventHandlers(peer, participantID)

	// Store peer
	s.peers[participantID] = peer

	return peer, nil
}

// registerPeerEventHandlers registers event handlers for the peer.
func (s *Service) registerPeerEventHandlers(peer *Peer, participantID string) {
	if s.events == nil {
		return
	}

	// ICE connection state changes
	peer.OnICEConnectionStateChange(func(state pion.ICEConnectionState) {
		s.events.OnICEConnectionStateChange(participantID, state)
	})

	// Peer connection state changes
	peer.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		s.events.OnPeerConnectionStateChange(participantID, state)

		// Debug: Check DTLS state when connection state changes
		if state == pion.PeerConnectionStateConnected {
			sctp := peer.SCTP()
			if sctp != nil && sctp.Transport() != nil {
				fmt.Printf("[DEBUG] DTLS state for %s: %s\n", participantID, sctp.Transport().State().String())
			}
			// Also check via senders
			senders := peer.GetSenders()
			for i, sender := range senders {
				if sender.Transport() != nil {
					fmt.Printf("[DEBUG] Sender[%d] DTLS state: %s\n", i, sender.Transport().State().String())
				}
			}
		}
	})

	// ICE candidates (for trickle ICE)
	peer.OnICECandidate(func(candidate *pion.ICECandidate) {
		if candidate != nil {
			s.events.OnICECandidate(participantID, candidate.ToJSON())
		}
	})

	// Track received
	peer.OnTrack(func(track *pion.TrackRemote, receiver *pion.RTPReceiver) {
		fmt.Printf("[DEBUG] OnTrack callback fired: participantID=%s, track_id=%s, kind=%s, ssrc=%d\n",
			participantID, track.ID(), track.Kind().String(), track.SSRC())
		s.events.OnTrack(participantID, track, receiver)
	})

	// Negotiation needed
	peer.OnNegotiationNeeded(func() {
		s.events.OnNegotiationNeeded(participantID)
	})
}

// Close closes all peer connections.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastErr error
	for participantID, peer := range s.peers {
		if err := peer.Close(); err != nil {
			lastErr = err
		}
		delete(s.peers, participantID)
	}

	return lastErr
}
