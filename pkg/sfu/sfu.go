package sfu

import (
	"sync"

	"github.com/pion/webrtc/v4"
)

type SessionProvider interface {
	GetTransportConfig() WebRTCTransportConfig
	GetSession(id string) Session
}

var _ SessionProvider = (*SFU)(nil)

func NewSFU() *SFU {
	return &SFU{
		sessions: make(map[string]Session),
	}
}

type SFU struct {
	lock     sync.RWMutex
	sessions map[string]Session

	transportConfig WebRTCTransportConfig
}

func (s *SFU) GetSession(id string) Session {
	session, ok := s.getSession(id)
	if ok {
		return session
	}

	return s.newSession(id)
}

func (s *SFU) getSession(id string) (Session, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	session, ok := s.sessions[id]

	return session, ok
}

func (s *SFU) newSession(id string) Session {
	session := NewSession(id)

	s.lock.Lock()
	s.sessions[id] = session
	s.lock.Unlock()

	return session
}

func (s *SFU) GetTransportConfig() WebRTCTransportConfig {
	return s.transportConfig
}

type WebRTCTransportConfig struct {
	Configuration webrtc.Configuration
	Setting       webrtc.SettingEngine
	RouterConfig  RouterConfig
}

func NewWebRTCTransportConfig() *WebRTCTransportConfig {
	return &WebRTCTransportConfig{}
}
