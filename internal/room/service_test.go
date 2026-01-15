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
