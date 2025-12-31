package sfu

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Session struct {
	id      string
	sfu     *SFU
	peers   map[string]*PeerConnection
	routers map[string]*Router
	mu      sync.RWMutex
}

func NewSession(id string, sfu *SFU) *Session {
	return &Session{
		id:      id,
		sfu:     sfu,
		peers:   make(map[string]*PeerConnection),
		routers: make(map[string]*Router),
	}
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) AddPeer(peerID string, conn *websocket.Conn) (*PeerConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	peer, err := NewPeerConnection(peerID, s, conn)
	if err != nil {
		return nil, err
	}

	s.peers[peerID] = peer

	return peer, nil
}

func (s *Session) GetPeer(peerID string) (*PeerConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peer, ok := s.peers[peerID]
	if !ok {
		return nil, ErrPeerNotFound
	}

	return peer, nil
}

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

func (s *Session) GetPeers() []*PeerConnection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peers := make([]*PeerConnection, 0, len(s.peers))
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}

	return peers
}

func (s *Session) AddRouter(peerID string, router *Router) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.routers[peerID] = router
}

func (s *Session) GetRouter(peerID string) (*Router, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	router, ok := s.routers[peerID]
	return router, ok
}

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

func (s *Session) Broadcast(excludePeerID string, message interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for peerID, peer := range s.peers {
		if peerID != excludePeerID {
			peer.SendMessage(message)
		}
	}
}

func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, peer := range s.peers {
		peer.Close()
	}

	for _, router := range s.routers {
		router.Close()
	}

	s.peers = make(map[string]*PeerConnection)
	s.routers = make(map[string]*Router)
}
