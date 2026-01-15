package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HMasataka/choice/internal/auth"
	"github.com/HMasataka/choice/internal/room"
)

// HealthResponse represents a health check response.
type HealthResponse struct {
	Status string `json:"status"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// RoomResponse represents a room info response.
type RoomResponse struct {
	ID              string `json:"id"`
	ParticipantCount int    `json:"participant_count"`
	MaxParticipants int    `json:"max_participants"`
	Status          string `json:"status"`
}

// ParticipantsResponse represents a participants list response.
type ParticipantsResponse struct {
	Participants []ParticipantInfo `json:"participants"`
}

// ParticipantInfo represents participant information.
type ParticipantInfo struct {
	ID       string `json:"id"`
	Metadata any    `json:"metadata,omitempty"`
}

// TokenResponse represents a token creation response.
type TokenResponse struct {
	Token string `json:"token"`
}

// CreateRoomRequest represents a room creation request.
type CreateRoomRequest struct {
	MaxParticipants int    `json:"max_participants,omitempty"`
	Metadata        any    `json:"metadata,omitempty"`
}

// CreateRoomResponse represents a room creation response.
type CreateRoomResponse struct {
	ID              string `json:"id"`
	MaxParticipants int    `json:"max_participants"`
}

// CreateTokenRequest represents a token creation request.
type CreateTokenRequest struct {
	ParticipantID   string `json:"participant_id"`
	Role            string `json:"role,omitempty"`
	ExpiresIn       int    `json:"expires_in,omitempty"` // seconds
	Metadata        any    `json:"metadata,omitempty"`
}

// handleHealth handles the /health endpoint.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

// handleReady handles the /ready endpoint.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// TODO: Add actual readiness checks (database, redis, etc.)
	s.writeJSON(w, http.StatusOK, HealthResponse{Status: "ready"})
}

// handleGetRoom handles GET /api/v1/rooms/{id}.
func (s *Server) handleGetRoom(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	if roomID == "" {
		s.writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}

	room, err := s.roomManager.GetRoom(roomID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "room not found")
		return
	}

	s.writeJSON(w, http.StatusOK, RoomResponse{
		ID:               roomID,
		ParticipantCount: room.ParticipantCount(),
		MaxParticipants:  room.MaxParticipants,
		Status:           string(room.GetState()),
	})
}

// handleCreateRoom handles POST /api/v1/rooms.
func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	var req CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	maxParticipants := req.MaxParticipants
	if maxParticipants <= 0 {
		maxParticipants = 100
	}
	// Enforce upper bound to prevent memory exhaustion
	const maxParticipantsLimit = 10000
	if maxParticipants > maxParticipantsLimit {
		s.writeError(w, http.StatusBadRequest, "max_participants exceeds limit")
		return
	}

	// Generate a room ID
	roomID := generateRoomID()

	// Create room options
	var opts []room.RoomOption
	opts = append(opts, room.WithMaxParticipants(maxParticipants))
	if req.Metadata != nil {
		if metadata, ok := req.Metadata.(map[string]interface{}); ok {
			opts = append(opts, room.WithMetadata(metadata))
		}
	}

	// Create room
	createdRoom, err := s.roomManager.CreateRoom(roomID, opts...)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to create room")
		return
	}

	s.writeJSON(w, http.StatusCreated, CreateRoomResponse{
		ID:              createdRoom.ID,
		MaxParticipants: createdRoom.MaxParticipants,
	})
}

// handleDeleteRoom handles DELETE /api/v1/rooms/{id}.
func (s *Server) handleDeleteRoom(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	if roomID == "" {
		s.writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}

	err := s.roomManager.DeleteRoom(roomID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "room not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetParticipants handles GET /api/v1/rooms/{id}/participants.
func (s *Server) handleGetParticipants(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	if roomID == "" {
		s.writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}

	room, err := s.roomManager.GetRoom(roomID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "room not found")
		return
	}

	participants := room.GetParticipants()
	participantInfos := make([]ParticipantInfo, 0, len(participants))
	for _, p := range participants {
		participantInfos = append(participantInfos, ParticipantInfo{
			ID:       p.ID,
			Metadata: p.GetMetadata(),
		})
	}

	s.writeJSON(w, http.StatusOK, ParticipantsResponse{
		Participants: participantInfos,
	})
}

// handleCreateToken handles POST /api/v1/rooms/{id}/token.
func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	if roomID == "" {
		s.writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}

	var req CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ParticipantID == "" {
		s.writeError(w, http.StatusBadRequest, "participant_id is required")
		return
	}

	// Verify room exists
	_, err := s.roomManager.GetRoom(roomID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "room not found")
		return
	}

	// Generate token requires tokenGenerator to be configured
	if s.tokenGenerator == nil {
		s.writeError(w, http.StatusServiceUnavailable, "token generation not configured")
		return
	}

	// Default expiration: 1 hour
	expiresIn := req.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	// Default role: publisher
	role := req.Role
	if role == "" {
		role = "publisher"
	}

	token, err := s.tokenGenerator.GenerateToken(roomID, req.ParticipantID, role, expiresIn)
	if err != nil {
		// Check for client errors
		if errors.Is(err, auth.ErrInvalidRole) {
			s.writeError(w, http.StatusBadRequest, "invalid role")
			return
		}
		if errors.Is(err, auth.ErrInvalidExpiresIn) {
			s.writeError(w, http.StatusBadRequest, "invalid expires_in value")
			return
		}
		s.logger.Error("failed to generate token", "error", err)
		s.writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	s.writeJSON(w, http.StatusOK, TokenResponse{
		Token: token,
	})
}

// handleLockRoom handles POST /api/v1/rooms/{id}/lock.
func (s *Server) handleLockRoom(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	if roomID == "" {
		s.writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}

	rm, err := s.roomManager.GetRoom(roomID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "room not found")
		return
	}

	if err := rm.Lock(); err != nil {
		if err == room.ErrRoomClosed {
			s.writeError(w, http.StatusGone, "room is closed")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "failed to lock room")
		return
	}

	s.writeJSON(w, http.StatusOK, RoomResponse{
		ID:               roomID,
		ParticipantCount: rm.ParticipantCount(),
		MaxParticipants:  rm.MaxParticipants,
		Status:           string(rm.GetState()),
	})
}

// handleUnlockRoom handles DELETE /api/v1/rooms/{id}/lock.
func (s *Server) handleUnlockRoom(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	if roomID == "" {
		s.writeError(w, http.StatusBadRequest, "room_id is required")
		return
	}

	rm, err := s.roomManager.GetRoom(roomID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "room not found")
		return
	}

	if err := rm.Unlock(); err != nil {
		if err == room.ErrRoomClosed {
			s.writeError(w, http.StatusGone, "room is closed")
			return
		}
		if err == room.ErrRoomNotLocked {
			s.writeError(w, http.StatusConflict, "room is not locked")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "failed to unlock room")
		return
	}

	s.writeJSON(w, http.StatusOK, RoomResponse{
		ID:               roomID,
		ParticipantCount: rm.ParticipantCount(),
		MaxParticipants:  rm.MaxParticipants,
		Status:           string(rm.GetState()),
	})
}

// writeJSON writes a JSON response.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Error("failed to encode JSON response", "error", err)
	}
}

// writeError writes an error response.
func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Code:    status,
		Message: message,
	})
}

// generateRoomID generates a new room ID.
func generateRoomID() string {
	// TODO: Implement a better room ID generation strategy
	// For now, use a simple UUID
	return "room-" + randomString(8)
}

// randomString generates a random alphanumeric string of the given length.
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to a simple string in case of error
		return "fallback"
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b)
}
