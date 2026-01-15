package store

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrSessionNotFound is returned when a session is not found.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionExpired is returned when a session has expired.
	ErrSessionExpired = errors.New("session expired")
)

// MemoryStore is an in-memory implementation of SessionStore.
type MemoryStore struct {
	mu                sync.RWMutex
	sessions          map[string]*Session
	participantIndex  map[string][]string // participantID -> []sessionID
	roomIndex         map[string][]string // roomID -> []sessionID
	cleanupInterval   time.Duration
	stopCleanup       chan struct{}
	cleanupWaitGroup  sync.WaitGroup
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{
		sessions:         make(map[string]*Session),
		participantIndex: make(map[string][]string),
		roomIndex:        make(map[string][]string),
		cleanupInterval:  10 * time.Minute,
		stopCleanup:      make(chan struct{}),
	}

	// Start cleanup goroutine
	s.cleanupWaitGroup.Add(1)
	go s.cleanupExpiredSessions()

	return s
}

// SaveSession saves a session to the store.
func (s *MemoryStore) SaveSession(ctx context.Context, session *Session) error {
	if session == nil || session.SessionID == "" {
		return errors.New("invalid session")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.SessionID] = session

	// Update participant index
	if session.ParticipantID != "" {
		s.participantIndex[session.ParticipantID] = append(
			s.participantIndex[session.ParticipantID],
			session.SessionID,
		)
	}

	// Update room index
	if session.RoomID != "" {
		s.roomIndex[session.RoomID] = append(
			s.roomIndex[session.RoomID],
			session.SessionID,
		)
	}

	return nil
}

// GetSession retrieves a session from the store by session ID.
func (s *MemoryStore) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	// Check if session has expired
	if time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	// Return a copy to prevent external modification
	return copySession(session), nil
}

// DeleteSession deletes a session from the store.
func (s *MemoryStore) DeleteSession(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	// Remove from participant index
	if session.ParticipantID != "" {
		s.removeFromIndex(s.participantIndex, session.ParticipantID, sessionID)
	}

	// Remove from room index
	if session.RoomID != "" {
		s.removeFromIndex(s.roomIndex, session.RoomID, sessionID)
	}

	delete(s.sessions, sessionID)

	return nil
}

// UpdateSession updates an existing session in the store.
func (s *MemoryStore) UpdateSession(ctx context.Context, session *Session) error {
	if session == nil || session.SessionID == "" {
		return errors.New("invalid session")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[session.SessionID]; !exists {
		return ErrSessionNotFound
	}

	s.sessions[session.SessionID] = session

	return nil
}

// GetSessionsByParticipant retrieves all sessions for a given participant.
func (s *MemoryStore) GetSessionsByParticipant(ctx context.Context, participantID string) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionIDs, exists := s.participantIndex[participantID]
	if !exists {
		return []*Session{}, nil
	}

	sessions := make([]*Session, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if session, exists := s.sessions[sessionID]; exists {
			// Check if session has not expired
			if time.Now().Before(session.ExpiresAt) {
				sessions = append(sessions, copySession(session))
			}
		}
	}

	return sessions, nil
}

// GetSessionsByRoom retrieves all sessions for a given room.
func (s *MemoryStore) GetSessionsByRoom(ctx context.Context, roomID string) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionIDs, exists := s.roomIndex[roomID]
	if !exists {
		return []*Session{}, nil
	}

	sessions := make([]*Session, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if session, exists := s.sessions[sessionID]; exists {
			// Check if session has not expired
			if time.Now().Before(session.ExpiresAt) {
				sessions = append(sessions, copySession(session))
			}
		}
	}

	return sessions, nil
}

// Close closes the store and cleans up resources.
func (s *MemoryStore) Close() error {
	close(s.stopCleanup)
	s.cleanupWaitGroup.Wait()
	return nil
}

// cleanupExpiredSessions periodically removes expired sessions from the store.
func (s *MemoryStore) cleanupExpiredSessions() {
	defer s.cleanupWaitGroup.Done()

	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCleanup:
			return
		}
	}
}

// cleanup removes expired sessions from the store.
func (s *MemoryStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	expiredSessions := make([]string, 0)

	// Find expired sessions
	for sessionID, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}

	// Remove expired sessions
	for _, sessionID := range expiredSessions {
		session := s.sessions[sessionID]

		// Remove from participant index
		if session.ParticipantID != "" {
			s.removeFromIndex(s.participantIndex, session.ParticipantID, sessionID)
		}

		// Remove from room index
		if session.RoomID != "" {
			s.removeFromIndex(s.roomIndex, session.RoomID, sessionID)
		}

		delete(s.sessions, sessionID)
	}
}

// removeFromIndex removes a session ID from an index.
func (s *MemoryStore) removeFromIndex(index map[string][]string, key, sessionID string) {
	sessionIDs := index[key]
	for i, sid := range sessionIDs {
		if sid == sessionID {
			index[key] = append(sessionIDs[:i], sessionIDs[i+1:]...)
			break
		}
	}

	// Remove the key if there are no more sessions
	if len(index[key]) == 0 {
		delete(index, key)
	}
}

// copySession creates a deep copy of a session.
func copySession(s *Session) *Session {
	publishedTracks := make([]string, len(s.PublishedTracks))
	copy(publishedTracks, s.PublishedTracks)

	subscriptions := make([]string, len(s.Subscriptions))
	copy(subscriptions, s.Subscriptions)

	metadata := make(map[string]interface{}, len(s.Metadata))
	for k, v := range s.Metadata {
		metadata[k] = v
	}

	return &Session{
		SessionID:       s.SessionID,
		ParticipantID:   s.ParticipantID,
		RoomID:          s.RoomID,
		PublishedTracks: publishedTracks,
		Subscriptions:   subscriptions,
		Metadata:        metadata,
		UserAgent:       s.UserAgent,
		IPAddress:       s.IPAddress,
		CreatedAt:       s.CreatedAt,
		ExpiresAt:       s.ExpiresAt,
	}
}
