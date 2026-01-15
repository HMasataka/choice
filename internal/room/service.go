package room

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/HMasataka/choice/internal/auth"
	"github.com/HMasataka/choice/internal/signaling"
	"github.com/HMasataka/choice/internal/signaling/protocol"
	"github.com/HMasataka/choice/internal/store"
	"github.com/HMasataka/choice/pkg/logger"
)

// TURNCredentialProvider defines the interface for generating TURN credentials.
type TURNCredentialProvider interface {
	GenerateCredentials(participantID string) ([]TURNCredential, error)
}

// TURNCredential represents a TURN server credential.
type TURNCredential struct {
	URLs     []string
	Username string
	Password string
}

// Service implements the RoomService interface.
type Service struct {
	manager      *Manager
	sessionStore store.SessionStore
	jwtValidator JWTValidator
	turnProvider TURNCredentialProvider
	logger       *logger.Logger
	sessionTTL   time.Duration
	eventEmitter *EventEmitter
}

// JWTValidator defines the interface for validating JWT tokens.
type JWTValidator interface {
	Validate(ctx context.Context, token string) (*auth.Claims, error)
}

// ServiceConfig contains configuration for the room service.
type ServiceConfig struct {
	// SessionTTL is the time-to-live for sessions (used for reconnection).
	SessionTTL time.Duration
}

// DefaultServiceConfig returns the default service configuration.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		SessionTTL: 30 * time.Second,
	}
}

// NewService creates a new room service.
func NewService(
	manager *Manager,
	sessionStore store.SessionStore,
	jwtValidator JWTValidator,
	turnProvider TURNCredentialProvider,
	logger *logger.Logger,
	cfg ServiceConfig,
) *Service {
	return &Service{
		manager:      manager,
		sessionStore: sessionStore,
		jwtValidator: jwtValidator,
		turnProvider: turnProvider,
		logger:       logger,
		sessionTTL:   cfg.SessionTTL,
		eventEmitter: NewEventEmitter(),
	}
}

// EventEmitter returns the event emitter for the service.
func (s *Service) EventEmitter() *EventEmitter {
	return s.eventEmitter
}

// Join handles a participant joining a room.
func (s *Service) Join(ctx context.Context, token string, sessionID string, metadata map[string]interface{}) (*signaling.JoinResponse, error) {
	// Validate JWT token
	claims, err := s.jwtValidator.Validate(ctx, token)
	if err != nil {
		s.logger.Error("failed to validate JWT", "error", err)
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Extract room ID from claims
	roomID := claims.RoomID
	if roomID == "" {
		return nil, fmt.Errorf("missing room_id in token claims")
	}

	// Extract participant ID from claims (use sub claim)
	participantID := claims.Subject
	if participantID == "" {
		return nil, fmt.Errorf("missing sub claim in token")
	}

	// Check if this is a reconnection attempt
	var existingSession *store.Session
	isReconnection := false
	if sessionID != "" {
		existingSession, err = s.sessionStore.GetSession(ctx, sessionID)
		if err != nil {
			if err != store.ErrSessionNotFound && err != store.ErrSessionExpired {
				s.logger.Error("failed to get existing session", "session_id", sessionID, "error", err)
			}
			// Session not found or expired, treat as new session
			sessionID = ""
		} else {
			// Validate session ownership and room match
			if existingSession.ParticipantID != participantID {
				s.logger.Error("session participant ID mismatch", "session_id", sessionID, "expected", existingSession.ParticipantID, "actual", participantID)
				return nil, fmt.Errorf("invalid session: participant mismatch")
			}
			if existingSession.RoomID != roomID {
				s.logger.Error("session room ID mismatch", "session_id", sessionID, "expected", existingSession.RoomID, "actual", roomID)
				return nil, fmt.Errorf("invalid session: room mismatch")
			}
			isReconnection = true
		}
	}

	// Get or create room
	room, err := s.manager.GetRoom(roomID)
	if err != nil {
		if err == ErrRoomNotFound {
			// Create room if it doesn't exist
			room, err = s.manager.CreateRoom(roomID)
			if err != nil {
				s.logger.Error("failed to create room", "room_id", roomID, "error", err)
				return nil, fmt.Errorf("failed to create room: %w", err)
			}
		} else {
			s.logger.Error("failed to get room", "room_id", roomID, "error", err)
			return nil, fmt.Errorf("failed to get room: %w", err)
		}
	}

	// Check if this is a reconnection and restore state
	if isReconnection {
		s.logger.Info("reconnecting participant", "participant_id", participantID, "room_id", roomID, "session_id", sessionID)

		// Check if participant is still in the room
		existingParticipant, err := room.GetParticipant(participantID)
		if err != nil {
			if err == ErrParticipantNotFound {
				// Participant was removed during disconnect, need to re-add
				s.logger.Info("participant not in room, re-adding", "participant_id", participantID, "room_id", roomID)
				participant := NewParticipant(participantID, roomID, WithParticipantMetadata(metadata))
				if addErr := room.AddParticipant(participant); addErr != nil {
					s.logger.Error("failed to re-add participant to room", "participant_id", participantID, "room_id", roomID, "error", addErr)
					return nil, fmt.Errorf("failed to re-add participant: %w", addErr)
				}
				// Restore participant state from session
				participant.SetPublishedTracks(existingSession.PublishedTracks)
				participant.SetSubscriptions(existingSession.Subscriptions)
			} else {
				s.logger.Error("failed to get participant during reconnection", "participant_id", participantID, "error", err)
				return nil, fmt.Errorf("failed to get participant: %w", err)
			}
		} else {
			existingParticipant.SetMetadata(metadata)
		}
		// If participant is still in room, metadata is refreshed

		// Restore session state
		session := &store.Session{
			SessionID:       sessionID,
			ParticipantID:   participantID,
			RoomID:          roomID,
			PublishedTracks: existingSession.PublishedTracks,
			Subscriptions:   existingSession.Subscriptions,
			Metadata:        metadata,
			UserAgent:       existingSession.UserAgent,
			IPAddress:       existingSession.IPAddress,
			CreatedAt:       existingSession.CreatedAt,
			ExpiresAt:       time.Now().Add(s.sessionTTL),
		}

		if err := s.sessionStore.UpdateSession(ctx, session); err != nil {
			s.logger.Error("failed to update session", "session_id", sessionID, "error", err)
			return nil, fmt.Errorf("failed to update session: %w", err)
		}

		// Generate ICE servers with credentials
		iceServers, err := s.generateIceServers(participantID)
		if err != nil {
			s.logger.Warn("failed to generate ICE servers", "error", err)
			// Continue with empty ICE servers
			iceServers = []protocol.IceServer{}
		}

		// Get list of other participants
		participants := s.buildParticipantList(room, participantID)

		// Build reconnect info with media state to restore
		reconnectInfo := &protocol.ReconnectInfo{
			PublishedTracks: slices.Clone(existingSession.PublishedTracks),
			Subscriptions:   slices.Clone(existingSession.Subscriptions),
		}

		// Emit participant reconnected event
		s.eventEmitter.Emit(CreateParticipantReconnectedEvent(roomID, participantID, metadata))

		return &signaling.JoinResponse{
			SessionID:     sessionID,
			RoomID:        roomID,
			ParticipantID: participantID,
			Participants:  participants,
			IceServers:    iceServers,
			Reconnected:   true,
			ReconnectInfo: reconnectInfo,
		}, nil
	}

	// Create new participant
	participant := NewParticipant(participantID, roomID, WithParticipantMetadata(metadata))

	// Add participant to room
	if err := room.AddParticipant(participant); err != nil {
		// If participant already exists (e.g., from an expired session), remove and re-add
		if err == ErrParticipantAlreadyJoined {
			s.logger.Info("participant already in room, removing and re-adding", "participant_id", participantID, "room_id", roomID)
			if removeErr := room.RemoveParticipant(participantID); removeErr != nil {
				s.logger.Error("failed to remove existing participant", "participant_id", participantID, "error", removeErr)
				return nil, fmt.Errorf("failed to remove existing participant: %w", removeErr)
			}
			// Try adding again
			if err := room.AddParticipant(participant); err != nil {
				s.logger.Error("failed to add participant to room after removal", "participant_id", participantID, "room_id", roomID, "error", err)
				return nil, fmt.Errorf("failed to add participant: %w", err)
			}
		} else {
			s.logger.Error("failed to add participant to room", "participant_id", participantID, "room_id", roomID, "error", err)
			return nil, fmt.Errorf("failed to add participant: %w", err)
		}
	}

	// Generate new session ID if not provided
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// Create session
	session := &store.Session{
		SessionID:       sessionID,
		ParticipantID:   participantID,
		RoomID:          roomID,
		PublishedTracks: []string{},
		Subscriptions:   []string{},
		Metadata:        metadata,
		UserAgent:       "", // TODO: Extract from context
		IPAddress:       "", // TODO: Extract from context
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(s.sessionTTL),
	}

	if err := s.sessionStore.SaveSession(ctx, session); err != nil {
		s.logger.Error("failed to save session", "session_id", sessionID, "error", err)
		// Don't fail the join if session save fails
	}

	// Generate ICE servers with credentials
	iceServers, err := s.generateIceServers(participantID)
	if err != nil {
		s.logger.Warn("failed to generate ICE servers", "error", err)
		// Continue with empty ICE servers
		iceServers = []protocol.IceServer{}
	}

	// Get list of other participants
	participants := s.buildParticipantList(room, participantID)

	s.logger.Info("participant joined room", "participant_id", participantID, "room_id", roomID, "session_id", sessionID)

	return &signaling.JoinResponse{
		SessionID:     sessionID,
		RoomID:        roomID,
		ParticipantID: participantID,
		Participants:  participants,
		IceServers:    iceServers,
	}, nil
}

// Leave handles a participant leaving a room explicitly.
// This deletes the session and removes the participant from the room.
func (s *Service) Leave(ctx context.Context, participantID string) error {
	// Find the session for this participant
	sessions, err := s.sessionStore.GetSessionsByParticipant(ctx, participantID)
	if err != nil {
		s.logger.Error("failed to get sessions for participant", "participant_id", participantID, "error", err)
	}

	// Delete all sessions for this participant
	for _, session := range sessions {
		if err := s.sessionStore.DeleteSession(ctx, session.SessionID); err != nil {
			s.logger.Error("failed to delete session", "session_id", session.SessionID, "error", err)
		}

		// Get room and remove participant
		room, err := s.manager.GetRoom(session.RoomID)
		if err != nil {
			if err == ErrRoomNotFound {
				// Room already deleted, skip
				continue
			}
			s.logger.Error("failed to get room", "room_id", session.RoomID, "error", err)
			continue
		}

		if err := room.RemoveParticipant(participantID); err != nil {
			if err == ErrParticipantNotFound {
				// Participant already removed, skip
				continue
			}
			s.logger.Error("failed to remove participant from room", "participant_id", participantID, "room_id", session.RoomID, "error", err)
		}
	}

	s.logger.Info("participant left", "participant_id", participantID)

	return nil
}

// Disconnect handles a participant disconnecting (connection closed).
// Unlike Leave, this keeps the session alive for potential reconnection within TTL.
// The participant is removed from the room but can rejoin with the same session.
func (s *Service) Disconnect(ctx context.Context, participantID string) error {
	// Find the session for this participant
	sessions, err := s.sessionStore.GetSessionsByParticipant(ctx, participantID)
	if err != nil {
		s.logger.Error("failed to get sessions for participant", "participant_id", participantID, "error", err)
	}

	// Remove participant from room but keep session for reconnection
	for _, session := range sessions {
		// Extend session TTL from disconnect time
		session.ExpiresAt = time.Now().Add(s.sessionTTL)
		if err := s.sessionStore.UpdateSession(ctx, session); err != nil {
			s.logger.Error("failed to update session expiry", "session_id", session.SessionID, "error", err)
		}

		// Get room and remove participant
		room, err := s.manager.GetRoom(session.RoomID)
		if err != nil {
			if err == ErrRoomNotFound {
				// Room already deleted, skip
				continue
			}
			s.logger.Error("failed to get room", "room_id", session.RoomID, "error", err)
			continue
		}

		if err := room.RemoveParticipant(participantID); err != nil {
			if err == ErrParticipantNotFound {
				// Participant already removed, skip
				continue
			}
			s.logger.Error("failed to remove participant from room", "participant_id", participantID, "room_id", session.RoomID, "error", err)
		}

		s.logger.Info("participant disconnected (session retained for reconnection)",
			"participant_id", participantID,
			"room_id", session.RoomID,
			"session_id", session.SessionID,
			"session_expires_at", session.ExpiresAt)
	}

	return nil
}

// generateIceServers generates ICE servers with TURN credentials for the participant.
func (s *Service) generateIceServers(participantID string) ([]protocol.IceServer, error) {
	if s.turnProvider == nil {
		return []protocol.IceServer{}, nil
	}

	// Get TURN credentials
	credentials, err := s.turnProvider.GenerateCredentials(participantID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate TURN credentials: %w", err)
	}

	iceServers := make([]protocol.IceServer, 0, len(credentials))
	for _, cred := range credentials {
		iceServers = append(iceServers, protocol.IceServer{
			URLs:       cred.URLs,
			Username:   cred.Username,
			Credential: cred.Password,
		})
	}

	return iceServers, nil
}

// buildParticipantList builds a list of participant info for the join response.
func (s *Service) buildParticipantList(room *Room, excludeParticipantID string) []protocol.ParticipantInfo {
	participants := room.GetParticipants()
	participantInfos := make([]protocol.ParticipantInfo, 0, len(participants))

	for _, p := range participants {
		if p.ID == excludeParticipantID {
			// Don't include the joining participant in the list
			continue
		}

		participantInfos = append(participantInfos, protocol.ParticipantInfo{
			ID:       p.ID,
			Metadata: p.GetMetadata(),
		})
	}

	return participantInfos
}
