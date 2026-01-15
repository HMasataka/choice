package store

import (
	"context"
	"time"
)

// Session represents a session with its associated data.
type Session struct {
	// SessionID is the unique identifier for the session.
	SessionID string
	// ParticipantID is the ID of the participant associated with this session.
	ParticipantID string
	// RoomID is the ID of the room the participant is in.
	RoomID string
	// PublishedTracks is a list of track IDs published by this participant.
	PublishedTracks []string
	// Subscriptions is a list of subscription IDs for this participant.
	Subscriptions []string
	// Metadata is arbitrary metadata associated with the session.
	Metadata map[string]interface{}
	// UserAgent is the user agent string of the client.
	UserAgent string
	// IPAddress is the IP address of the client.
	IPAddress string
	// CreatedAt is the time when the session was created.
	CreatedAt time.Time
	// ExpiresAt is the time when the session expires.
	ExpiresAt time.Time
}

// SessionStore is an interface for storing and retrieving session data.
type SessionStore interface {
	// SaveSession saves a session to the store.
	SaveSession(ctx context.Context, session *Session) error

	// GetSession retrieves a session from the store by session ID.
	GetSession(ctx context.Context, sessionID string) (*Session, error)

	// DeleteSession deletes a session from the store.
	DeleteSession(ctx context.Context, sessionID string) error

	// UpdateSession updates an existing session in the store.
	UpdateSession(ctx context.Context, session *Session) error

	// GetSessionsByParticipant retrieves all sessions for a given participant.
	GetSessionsByParticipant(ctx context.Context, participantID string) ([]*Session, error)

	// GetSessionsByRoom retrieves all sessions for a given room.
	GetSessionsByRoom(ctx context.Context, roomID string) ([]*Session, error)

	// Close closes the store and cleans up resources.
	Close() error
}
