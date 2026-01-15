package room

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/HMasataka/choice/internal/auth"
	"github.com/HMasataka/choice/internal/store"
	"github.com/HMasataka/choice/pkg/logger"
)

// mockJWTValidator is a mock implementation of JWTValidator for testing.
type mockJWTValidator struct {
	validateFunc func(ctx context.Context, token string) (*auth.Claims, error)
}

// Ensure mockJWTValidator implements JWTValidator interface.
var _ JWTValidator = (*mockJWTValidator)(nil)

func (m *mockJWTValidator) Validate(ctx context.Context, token string) (*auth.Claims, error) {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, token)
	}
	return &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "participant-default",
		},
		RoomID: "room-1",
	}, nil
}

func TestService_Join(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "error"})
	manager := NewManager(log)
	sessionStore := store.NewMemoryStore()
	defer sessionStore.Close()

	validator := &mockJWTValidator{
		validateFunc: func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RoomID: "room-1",
			}, nil
		},
	}

	service := NewService(manager, sessionStore, validator, nil, log, DefaultServiceConfig())

	ctx := context.Background()

	t.Run("joins a new room successfully", func(t *testing.T) {
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-1",
				},
				RoomID: "test-room-1",
			}, nil
		}

		resp, err := service.Join(ctx, "valid-token", "", map[string]interface{}{"name": "Alice"})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.SessionID)
		assert.Equal(t, "test-room-1", resp.RoomID)
		assert.NotEmpty(t, resp.ParticipantID)
		assert.Empty(t, resp.Participants)
	})

	t.Run("joins an existing room successfully", func(t *testing.T) {
		tokenCount := 0
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			tokenCount++
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-" + token,
				},
				RoomID: "test-room-2",
			}, nil
		}

		// First participant joins
		resp1, err := service.Join(ctx, "token-1", "", map[string]interface{}{"name": "Alice"})
		require.NoError(t, err)

		// Second participant joins
		resp2, err := service.Join(ctx, "token-2", "", map[string]interface{}{"name": "Bob"})
		require.NoError(t, err)

		assert.Equal(t, resp1.RoomID, resp2.RoomID)
		assert.NotEqual(t, resp1.ParticipantID, resp2.ParticipantID)
		assert.Len(t, resp2.Participants, 1) // Should see the first participant
	})

	t.Run("reconnects with existing session ID", func(t *testing.T) {
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-reconnect",
				},
				RoomID: "test-room-3",
			}, nil
		}

		// First join
		resp1, err := service.Join(ctx, "token-1", "", map[string]interface{}{"name": "Alice"})
		require.NoError(t, err)
		sessionID := resp1.SessionID

		// Simulate reconnection
		resp2, err := service.Join(ctx, "token-1", sessionID, map[string]interface{}{"name": "Alice Reconnected"})
		require.NoError(t, err)

		assert.Equal(t, sessionID, resp2.SessionID)
		assert.Equal(t, resp1.RoomID, resp2.RoomID)
		assert.Equal(t, resp1.ParticipantID, resp2.ParticipantID)
	})

	t.Run("fails with invalid token", func(t *testing.T) {
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			return nil, auth.ErrInvalidToken
		}

		_, err := service.Join(ctx, "invalid-token", "", map[string]interface{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token")
	})

	t.Run("fails with missing room_id in claims", func(t *testing.T) {
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-1",
				},
				RoomID: "",
			}, nil
		}

		_, err := service.Join(ctx, "token", "", map[string]interface{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing room_id")
	})
}

func TestService_Leave(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "error"})
	manager := NewManager(log)
	sessionStore := store.NewMemoryStore()
	defer sessionStore.Close()

	validator := &mockJWTValidator{
		validateFunc: func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RoomID: "room-1",
			}, nil
		},
	}

	service := NewService(manager, sessionStore, validator, nil, log, DefaultServiceConfig())

	ctx := context.Background()

	t.Run("leaves a room successfully", func(t *testing.T) {
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-leave",
				},
				RoomID: "test-room-4",
			}, nil
		}

		// Join room
		resp, err := service.Join(ctx, "token", "", map[string]interface{}{"name": "Alice"})
		require.NoError(t, err)
		participantID := resp.ParticipantID

		// Leave room
		err = service.Leave(ctx, participantID)
		require.NoError(t, err)

		// Verify session is deleted
		sessions, err := sessionStore.GetSessionsByParticipant(ctx, participantID)
		require.NoError(t, err)
		assert.Empty(t, sessions)
	})

	t.Run("handles leave for non-existent participant gracefully", func(t *testing.T) {
		err := service.Leave(ctx, "non-existent-participant")
		// Should not error even if participant doesn't exist
		assert.NoError(t, err)
	})
}

func TestService_ReconnectWithMediaState(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "error"})
	manager := NewManager(log)
	sessionStore := store.NewMemoryStore()
	defer sessionStore.Close()

	validator := &mockJWTValidator{}

	service := NewService(manager, sessionStore, validator, nil, log, DefaultServiceConfig())

	ctx := context.Background()

	t.Run("reconnection returns media state to restore", func(t *testing.T) {
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-media-1",
				},
				RoomID: "test-room-media-1",
			}, nil
		}

		// First join
		resp1, err := service.Join(ctx, "token", "", map[string]interface{}{"name": "Alice"})
		require.NoError(t, err)
		assert.False(t, resp1.Reconnected)
		assert.Nil(t, resp1.ReconnectInfo)

		sessionID := resp1.SessionID

		// Simulate publishing tracks and subscribing by updating the session
		session, err := sessionStore.GetSession(ctx, sessionID)
		require.NoError(t, err)
		session.PublishedTracks = []string{"track-1", "track-2"}
		session.Subscriptions = []string{"sub-1", "sub-2", "sub-3"}
		err = sessionStore.UpdateSession(ctx, session)
		require.NoError(t, err)

		// Simulate reconnection
		resp2, err := service.Join(ctx, "token", sessionID, map[string]interface{}{"name": "Alice Reconnected"})
		require.NoError(t, err)

		// Verify reconnection state
		assert.Equal(t, sessionID, resp2.SessionID)
		assert.True(t, resp2.Reconnected)
		require.NotNil(t, resp2.ReconnectInfo)
		assert.Equal(t, []string{"track-1", "track-2"}, resp2.ReconnectInfo.PublishedTracks)
		assert.Equal(t, []string{"sub-1", "sub-2", "sub-3"}, resp2.ReconnectInfo.Subscriptions)
	})

	t.Run("reconnection emits participantReconnected event", func(t *testing.T) {
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-event-1",
				},
				RoomID: "test-room-event-1",
			}, nil
		}

		// Track events
		eventReceived := make(chan *Event, 1)
		service.EventEmitter().On(EventParticipantReconnected, func(event *Event) {
			eventReceived <- event
		})

		// First join
		resp1, err := service.Join(ctx, "token", "", map[string]interface{}{"name": "Bob"})
		require.NoError(t, err)
		sessionID := resp1.SessionID

		// Reconnect
		metadata := map[string]interface{}{"name": "Bob Reconnected"}
		_, err = service.Join(ctx, "token", sessionID, metadata)
		require.NoError(t, err)

		// Verify event was emitted
		select {
		case event := <-eventReceived:
			assert.Equal(t, EventParticipantReconnected, event.Type)
			assert.Equal(t, "test-room-event-1", event.RoomID)
			assert.Equal(t, "participant-event-1", event.ParticipantID)
			assert.Equal(t, "Bob Reconnected", event.Metadata["name"])
		case <-time.After(500 * time.Millisecond):
			t.Fatal("expected participantReconnected event was not received")
		}
	})

	t.Run("session mismatch returns error", func(t *testing.T) {
		callCount := 0
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			callCount++
			if callCount == 1 {
				return &auth.Claims{
					RegisteredClaims: jwt.RegisteredClaims{
						Subject: "participant-mismatch-1",
					},
					RoomID: "test-room-mismatch-1",
				}, nil
			}
			// Second call has different participant
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-mismatch-2",
				},
				RoomID: "test-room-mismatch-1",
			}, nil
		}

		// First join
		resp1, err := service.Join(ctx, "token-1", "", map[string]interface{}{"name": "Alice"})
		require.NoError(t, err)
		sessionID := resp1.SessionID

		// Try to reconnect with different participant ID
		_, err = service.Join(ctx, "token-2", sessionID, map[string]interface{}{"name": "Eve"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "participant mismatch")
	})

	t.Run("room mismatch returns error", func(t *testing.T) {
		callCount := 0
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			callCount++
			if callCount == 1 {
				return &auth.Claims{
					RegisteredClaims: jwt.RegisteredClaims{
						Subject: "participant-room-mismatch",
					},
					RoomID: "test-room-a",
				}, nil
			}
			// Second call has different room
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-room-mismatch",
				},
				RoomID: "test-room-b",
			}, nil
		}

		// First join
		resp1, err := service.Join(ctx, "token-1", "", map[string]interface{}{"name": "Alice"})
		require.NoError(t, err)
		sessionID := resp1.SessionID

		// Try to reconnect with different room
		_, err = service.Join(ctx, "token-2", sessionID, map[string]interface{}{"name": "Alice"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "room mismatch")
	})
}

func TestService_SessionExpiration(t *testing.T) {
	log, _ := logger.New(logger.Config{Level: "error"})
	manager := NewManager(log)
	sessionStore := store.NewMemoryStore()
	defer sessionStore.Close()

	validator := &mockJWTValidator{
		validateFunc: func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RoomID: "room-1",
			}, nil
		},
	}

	// Use a very short session TTL for testing
	cfg := DefaultServiceConfig()
	cfg.SessionTTL = 100 * time.Millisecond

	service := NewService(manager, sessionStore, validator, nil, log, cfg)

	ctx := context.Background()

	t.Run("expired session cannot be used for reconnection", func(t *testing.T) {
		validator.validateFunc = func(ctx context.Context, token string) (*auth.Claims, error) {
			return &auth.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject: "participant-expiry",
				},
				RoomID: "test-room-5",
			}, nil
		}

		// First join
		resp1, err := service.Join(ctx, "token", "", map[string]interface{}{"name": "Alice"})
		require.NoError(t, err)
		sessionID := resp1.SessionID

		// Wait for session to expire
		time.Sleep(150 * time.Millisecond)

		// Try to reconnect with expired session
		resp2, err := service.Join(ctx, "token", sessionID, map[string]interface{}{"name": "Alice"})
		require.NoError(t, err)

		// Should get a different session ID (new session created)
		// Because the old session expired
		assert.NotEqual(t, sessionID, resp2.SessionID)
	})
}
