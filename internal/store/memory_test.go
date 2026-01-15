package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_SaveSession(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	session := &Session{
		SessionID:       "session-1",
		ParticipantID:   "participant-1",
		RoomID:          "room-1",
		PublishedTracks: []string{"track-1"},
		Subscriptions:   []string{"sub-1"},
		Metadata:        map[string]interface{}{"key": "value"},
		UserAgent:       "test-agent",
		IPAddress:       "127.0.0.1",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session)
	require.NoError(t, err)

	// Retrieve the session
	retrieved, err := store.GetSession(ctx, "session-1")
	require.NoError(t, err)
	assert.Equal(t, session.SessionID, retrieved.SessionID)
	assert.Equal(t, session.ParticipantID, retrieved.ParticipantID)
	assert.Equal(t, session.RoomID, retrieved.RoomID)
}

func TestMemoryStore_GetSession(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	t.Run("returns error for non-existent session", func(t *testing.T) {
		_, err := store.GetSession(ctx, "nonexistent")
		assert.Equal(t, ErrSessionNotFound, err)
	})

	t.Run("returns error for expired session", func(t *testing.T) {
		session := &Session{
			SessionID:     "session-expired",
			ParticipantID: "participant-1",
			RoomID:        "room-1",
			CreatedAt:     time.Now().Add(-2 * time.Hour),
			ExpiresAt:     time.Now().Add(-1 * time.Hour),
		}

		err := store.SaveSession(ctx, session)
		require.NoError(t, err)

		_, err = store.GetSession(ctx, "session-expired")
		assert.Equal(t, ErrSessionExpired, err)
	})
}

func TestMemoryStore_DeleteSession(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	session := &Session{
		SessionID:     "session-1",
		ParticipantID: "participant-1",
		RoomID:        "room-1",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session)
	require.NoError(t, err)

	err = store.DeleteSession(ctx, "session-1")
	require.NoError(t, err)

	_, err = store.GetSession(ctx, "session-1")
	assert.Equal(t, ErrSessionNotFound, err)
}

func TestMemoryStore_UpdateSession(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	session := &Session{
		SessionID:       "session-1",
		ParticipantID:   "participant-1",
		RoomID:          "room-1",
		PublishedTracks: []string{"track-1"},
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session)
	require.NoError(t, err)

	// Update session
	session.PublishedTracks = append(session.PublishedTracks, "track-2")
	err = store.UpdateSession(ctx, session)
	require.NoError(t, err)

	// Retrieve updated session
	retrieved, err := store.GetSession(ctx, "session-1")
	require.NoError(t, err)
	assert.Len(t, retrieved.PublishedTracks, 2)
}

func TestMemoryStore_GetSessionsByParticipant(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	session1 := &Session{
		SessionID:     "session-1",
		ParticipantID: "participant-1",
		RoomID:        "room-1",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}
	session2 := &Session{
		SessionID:     "session-2",
		ParticipantID: "participant-1",
		RoomID:        "room-2",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session1)
	require.NoError(t, err)
	err = store.SaveSession(ctx, session2)
	require.NoError(t, err)

	sessions, err := store.GetSessionsByParticipant(ctx, "participant-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestMemoryStore_GetSessionsByRoom(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	session1 := &Session{
		SessionID:     "session-1",
		ParticipantID: "participant-1",
		RoomID:        "room-1",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}
	session2 := &Session{
		SessionID:     "session-2",
		ParticipantID: "participant-2",
		RoomID:        "room-1",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}

	err := store.SaveSession(ctx, session1)
	require.NoError(t, err)
	err = store.SaveSession(ctx, session2)
	require.NoError(t, err)

	sessions, err := store.GetSessionsByRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}
