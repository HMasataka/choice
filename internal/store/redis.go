package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// RedisClient is an interface for Redis operations.
// This allows for easier testing and mocking.
type RedisClient interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, keys ...string) error
	SAdd(ctx context.Context, key string, members ...interface{}) error
	SMembers(ctx context.Context, key string) ([]string, error)
	SRem(ctx context.Context, key string, members ...interface{}) error
	Close() error
}

// RedisStore is a Redis-based implementation of SessionStore.
type RedisStore struct {
	client RedisClient
	prefix string
}

// NewRedisStore creates a new Redis session store.
func NewRedisStore(client RedisClient, prefix string) *RedisStore {
	if prefix == "" {
		prefix = "session:"
	}

	return &RedisStore{
		client: client,
		prefix: prefix,
	}
}

// SaveSession saves a session to Redis.
func (s *RedisStore) SaveSession(ctx context.Context, session *Session) error {
	if session == nil || session.SessionID == "" {
		return fmt.Errorf("invalid session")
	}

	// Serialize session to JSON
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Save session data
	key := s.sessionKey(session.SessionID)
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return ErrSessionExpired
	}

	if err := s.client.Set(ctx, key, data, ttl); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	// Add to participant index
	if session.ParticipantID != "" {
		participantKey := s.participantKey(session.ParticipantID)
		if err := s.client.SAdd(ctx, participantKey, session.SessionID); err != nil {
			return fmt.Errorf("failed to add to participant index: %w", err)
		}
	}

	// Add to room index
	if session.RoomID != "" {
		roomKey := s.roomKey(session.RoomID)
		if err := s.client.SAdd(ctx, roomKey, session.SessionID); err != nil {
			return fmt.Errorf("failed to add to room index: %w", err)
		}
	}

	return nil
}

// GetSession retrieves a session from Redis by session ID.
func (s *RedisStore) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	key := s.sessionKey(sessionID)

	data, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if data == "" {
		return nil, ErrSessionNotFound
	}

	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	// Check if session has expired
	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		_ = s.DeleteSession(ctx, sessionID)
		return nil, ErrSessionExpired
	}

	return &session, nil
}

// DeleteSession deletes a session from Redis.
func (s *RedisStore) DeleteSession(ctx context.Context, sessionID string) error {
	// Get session data directly without using GetSession to avoid infinite recursion
	key := s.sessionKey(sessionID)
	data, err := s.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get session for deletion: %w", err)
	}

	// If session data exists, parse it to get index information
	var session *Session
	if data != "" {
		var sess Session
		if err := json.Unmarshal([]byte(data), &sess); err == nil {
			session = &sess
		}
	}

	// Delete session data
	if err := s.client.Del(ctx, key); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Remove from participant index
	if session != nil && session.ParticipantID != "" {
		participantKey := s.participantKey(session.ParticipantID)
		if err := s.client.SRem(ctx, participantKey, sessionID); err != nil {
			return fmt.Errorf("failed to remove from participant index: %w", err)
		}
	}

	// Remove from room index
	if session != nil && session.RoomID != "" {
		roomKey := s.roomKey(session.RoomID)
		if err := s.client.SRem(ctx, roomKey, sessionID); err != nil {
			return fmt.Errorf("failed to remove from room index: %w", err)
		}
	}

	return nil
}

// UpdateSession updates an existing session in Redis.
func (s *RedisStore) UpdateSession(ctx context.Context, session *Session) error {
	// Get existing session to check for participant/room changes
	existingSession, err := s.GetSession(ctx, session.SessionID)
	if err != nil {
		return err
	}

	// If participant changed, update indexes
	if existingSession.ParticipantID != session.ParticipantID {
		// Remove from old participant index
		if existingSession.ParticipantID != "" {
			oldParticipantKey := s.participantKey(existingSession.ParticipantID)
			if err := s.client.SRem(ctx, oldParticipantKey, session.SessionID); err != nil {
				return fmt.Errorf("failed to remove from old participant index: %w", err)
			}
		}
	}

	// If room changed, update indexes
	if existingSession.RoomID != session.RoomID {
		// Remove from old room index
		if existingSession.RoomID != "" {
			oldRoomKey := s.roomKey(existingSession.RoomID)
			if err := s.client.SRem(ctx, oldRoomKey, session.SessionID); err != nil {
				return fmt.Errorf("failed to remove from old room index: %w", err)
			}
		}
	}

	// Save updated session
	return s.SaveSession(ctx, session)
}

// GetSessionsByParticipant retrieves all sessions for a given participant.
func (s *RedisStore) GetSessionsByParticipant(ctx context.Context, participantID string) ([]*Session, error) {
	participantKey := s.participantKey(participantID)

	sessionIDs, err := s.client.SMembers(ctx, participantKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get participant sessions: %w", err)
	}

	sessions := make([]*Session, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		session, err := s.GetSession(ctx, sessionID)
		if err != nil {
			if err == ErrSessionNotFound || err == ErrSessionExpired {
				// Skip expired or not found sessions
				continue
			}
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// GetSessionsByRoom retrieves all sessions for a given room.
func (s *RedisStore) GetSessionsByRoom(ctx context.Context, roomID string) ([]*Session, error) {
	roomKey := s.roomKey(roomID)

	sessionIDs, err := s.client.SMembers(ctx, roomKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get room sessions: %w", err)
	}

	sessions := make([]*Session, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		session, err := s.GetSession(ctx, sessionID)
		if err != nil {
			if err == ErrSessionNotFound || err == ErrSessionExpired {
				// Skip expired or not found sessions
				continue
			}
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// Close closes the Redis client.
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// sessionKey returns the Redis key for a session.
func (s *RedisStore) sessionKey(sessionID string) string {
	return fmt.Sprintf("%s%s", s.prefix, sessionID)
}

// participantKey returns the Redis key for a participant index.
func (s *RedisStore) participantKey(participantID string) string {
	return fmt.Sprintf("%sparticipant:%s", s.prefix, participantID)
}

// roomKey returns the Redis key for a room index.
func (s *RedisStore) roomKey(roomID string) string {
	return fmt.Sprintf("%sroom:%s", s.prefix, roomID)
}
