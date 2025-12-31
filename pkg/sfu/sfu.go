package sfu

import (
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// Errors
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrPeerNotFound    = errors.New("peer not found")
)

// Config holds the SFU configuration.
type Config struct {
	ICEServers []webrtc.ICEServer
}

// SFU is the main Selective Forwarding Unit that manages sessions and WebRTC connections.
type SFU struct {
	config   Config
	api      *webrtc.API
	sessions map[string]*Session
	mu       sync.RWMutex
	upgrader websocket.Upgrader
}

// NewSFU creates a new SFU instance.
func NewSFU(config Config) *SFU {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}

	return &SFU{
		config:   config,
		api:      webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine)),
		sessions: make(map[string]*Session),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// NewPeerConnection creates a new WebRTC peer connection with the configured ICE servers.
func (s *SFU) NewPeerConnection() (*webrtc.PeerConnection, error) {
	return s.api.NewPeerConnection(webrtc.Configuration{
		ICEServers: s.config.ICEServers,
	})
}

// Session Management

// GetOrCreateSession returns an existing session or creates a new one.
func (s *SFU) GetOrCreateSession(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[id]; ok {
		return session
	}

	session := newSession(id, s)
	s.sessions[id] = session
	return session
}

// GetSession returns a session by ID.
func (s *SFU) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// DeleteSession removes and closes a session.
func (s *SFU) DeleteSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[id]; ok {
		session.Close()
		delete(s.sessions, id)
	}
}

// HandleWebSocket handles incoming WebSocket connections for signaling.
func (s *SFU) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	rawConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := newWSConn(rawConn)
	defer conn.Close()

	handler := newSignalingHandler(s, conn)
	handler.run()
}
