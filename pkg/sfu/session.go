package sfu

import (
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

// Peer Management

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
		peer.Close()
		delete(s.peers, peerID)
	}

	if router, ok := s.routers[peerID]; ok {
		router.Close()
		delete(s.routers, peerID)
	}
}

// Router Management

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

// Subscription

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

		for trackID, simulcastRecv := range router.GetSimulcastReceivers() {
			peer.SendNotification("trackAdded", map[string]interface{}{
				"peerId":   peerID,
				"trackId":  trackID,
				"streamId": simulcastRecv.StreamID(),
				"kind":     simulcastRecv.Kind().String(),
			})
		}
	}
}

// Broadcast sends a message to all peers except the excluded one.
func (s *Session) Broadcast(excludePeerID string, method string, params map[string]interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for peerID, peer := range s.peers {
		if peerID != excludePeerID {
			peer.SendNotification(method, params)
		}
	}
}

// Close closes the session and all its peers and routers.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, peer := range s.peers {
		peer.Close()
	}

	for _, router := range s.routers {
		router.Close()
	}

	s.peers = make(map[string]*Peer)
	s.routers = make(map[string]*Router)
}
