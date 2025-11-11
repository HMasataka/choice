package sfu

import "sync"

type SessionProvider interface {
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
}

func (s *SFU) GetSession(id string) Session {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if session, ok := s.sessions[id]; ok {
		return session
	}

	return s.newSession(id)
}

func (s *SFU) newSession(id string) Session {
	session := NewSession(id)

	s.lock.Lock()
	s.sessions[id] = session
	s.lock.Unlock()

	return session
}
