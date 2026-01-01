package sfu

import (
	"log/slog"
	"sync"
)

// Session represents a room where multiple peers can join and share media.
type Session struct {
	id      string
	sfu     *SFU
	peers   map[string]*Peer
	routers map[string]*Router
	mu      sync.RWMutex
}

func newSession(id string, sfu *SFU) *Session {
	return &Session{
		id:      id,
		sfu:     sfu,
		peers:   make(map[string]*Peer),
		routers: make(map[string]*Router),
	}
}

// ID returns the session identifier.
func (s *Session) ID() string {
	return s.id
}

// AddPeer creates and adds a new peer to the session.
func (s *Session) AddPeer(peerID string, conn *wsConn) (*Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	peer, err := newPeer(peerID, s, conn)
	if err != nil {
		return nil, err
	}

	s.peers[peerID] = peer
	return peer, nil
}

// GetPeer returns a peer by ID.
func (s *Session) GetPeer(peerID string) (*Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peer, ok := s.peers[peerID]
	if !ok {
		return nil, ErrPeerNotFound
	}
	return peer, nil
}

// RemovePeer removes a peer and its associated router from the session.
func (s *Session) RemovePeer(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if peer, ok := s.peers[peerID]; ok {
		if err := peer.Close(); err != nil {
			slog.Warn("peer close error", slog.String("peerID", peerID), slog.String("error", err.Error()))
		}
		delete(s.peers, peerID)
	}

	if router, ok := s.routers[peerID]; ok {
		if err := router.Close(); err != nil {
			slog.Warn("router close error", slog.String("peerID", peerID), slog.String("error", err.Error()))
		}
		delete(s.routers, peerID)
	}
}

// AddRouter registers a router for a peer.
func (s *Session) AddRouter(peerID string, router *Router) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routers[peerID] = router
}

// GetRouter returns the router for a peer.
func (s *Session) GetRouter(peerID string) (*Router, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	router, ok := s.routers[peerID]
	return router, ok
}

// Subscribe connects a subscriber to a publisher's router.
func (s *Session) Subscribe(subscriberID, publisherID string) error {
	s.mu.RLock()
	subscriber, ok := s.peers[subscriberID]
	if !ok {
		s.mu.RUnlock()
		return ErrPeerNotFound
	}

	router, ok := s.routers[publisherID]
	if !ok {
		s.mu.RUnlock()
		return ErrPeerNotFound
	}
	s.mu.RUnlock()

	return subscriber.Subscribe(router)
}

// NotifyExistingTracks sends trackAdded notifications for all existing tracks to a new peer.
func (s *Session) NotifyExistingTracks(peer *Peer) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for peerID, router := range s.routers {
		if peerID == peer.ID() {
			continue
		}

		for trackID, track := range router.GetTracks() {
			if err := peer.SendNotification("trackAdded", map[string]any{
				"peerId":   peerID,
				"trackId":  trackID,
				"streamId": track.StreamID(),
				"kind":     track.Kind().String(),
			}); err != nil {
				slog.Warn("failed to notify existing track", slog.String("peerID", peerID), slog.String("trackID", trackID), slog.String("error", err.Error()))
			}
		}
	}
}

// Broadcast sends a message to all peers except the excluded one.
func (s *Session) Broadcast(excludePeerID string, method string, params map[string]any) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for peerID, peer := range s.peers {
		if peerID != excludePeerID {
			if err := peer.SendNotification(method, params); err != nil {
				slog.Warn("broadcast notification error", slog.String("peerID", peerID), slog.String("method", method), slog.String("error", err.Error()))
			}
		}
	}
}

// BroadcastData sends data to all peers except the sender via data channel.
func (s *Session) BroadcastData(senderPeerID string, data []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for peerID, peer := range s.peers {
		if peerID != senderPeerID {
			if err := peer.SendData(data); err != nil {
				slog.Warn("broadcast data error",
					slog.String("peerID", peerID),
					slog.String("error", err.Error()),
				)
			}
		}
	}
}

// Close closes the session and all its peers and routers.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, peer := range s.peers {
		if err := peer.Close(); err != nil {
			slog.Warn("peer close error", slog.String("peerID", peer.ID()), slog.String("error", err.Error()))
		}
	}

	for _, router := range s.routers {
		if err := router.Close(); err != nil {
			slog.Warn("router close error", slog.String("routerID", router.ID()), slog.String("error", err.Error()))
		}
	}

	s.peers = make(map[string]*Peer)
	s.routers = make(map[string]*Router)
}
