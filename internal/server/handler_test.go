package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HMasataka/choice/internal/auth"
	"github.com/HMasataka/choice/pkg/config"
	"github.com/HMasataka/choice/pkg/logger"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()

	cfg := config.DefaultConfig()
	log, err := logger.New(logger.DefaultConfig())
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	return New(cfg, log)
}

func TestHandleHealth(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
}

func TestHandleReady(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ready" {
		t.Errorf("expected status 'ready', got %q", resp.Status)
	}
}

func TestHandleGetRoom(t *testing.T) {
	s := newTestServer(t)

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// Now get the room
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+createResp.ID, nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp RoomResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != createResp.ID {
		t.Errorf("expected room ID %q, got %q", createResp.ID, resp.ID)
	}
}

func TestHandleGetRoomNotFound(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/non-existent-room", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleCreateRoom(t *testing.T) {
	s := newTestServer(t)

	body := CreateRoomRequest{
		MaxParticipants: 50,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp CreateRoomResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.MaxParticipants != 50 {
		t.Errorf("expected max participants 50, got %d", resp.MaxParticipants)
	}
}

func TestHandleCreateRoomDefaultMaxParticipants(t *testing.T) {
	s := newTestServer(t)

	body := CreateRoomRequest{}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp CreateRoomResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.MaxParticipants != 100 {
		t.Errorf("expected default max participants 100, got %d", resp.MaxParticipants)
	}
}

func TestHandleDeleteRoom(t *testing.T) {
	s := newTestServer(t)

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// Now delete the room
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/"+createResp.ID, nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}
}

func TestHandleDeleteRoomNotFound(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/non-existent-room", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleGetParticipants(t *testing.T) {
	s := newTestServer(t)

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// Now get participants
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+createResp.ID+"/participants", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp ParticipantsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Participants == nil {
		t.Error("expected participants array, got nil")
	}
}

func TestHandleGetParticipantsNotFound(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms/non-existent-room/participants", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleCreateTokenMissingParticipantID(t *testing.T) {
	s := newTestServer(t)

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	createBodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(createBodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	body := CreateTokenRequest{
		Role: "publisher",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+createResp.ID+"/token", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestContentTypeJSON(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", contentType)
	}
}

func TestHandleCreateRoomInvalidJSON(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected error code %d, got %d", http.StatusBadRequest, resp.Code)
	}
}

func TestHandleCreateTokenInvalidJSON(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/test-room/token", bytes.NewReader([]byte("{invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestErrorResponseSchema(t *testing.T) {
	s := newTestServer(t)

	body := CreateTokenRequest{} // Missing required fields
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/test-room/token", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	// Verify error response schema
	if resp.Error == "" {
		t.Error("expected non-empty error field")
	}
	if resp.Code != http.StatusBadRequest {
		t.Errorf("expected code %d, got %d", http.StatusBadRequest, resp.Code)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message field")
	}
}

func TestHandleLockRoom(t *testing.T) {
	s := newTestServer(t)

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// Now lock the room
	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+createResp.ID+"/lock", nil)
	lockW := httptest.NewRecorder()
	s.router.ServeHTTP(lockW, lockReq)

	if lockW.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, lockW.Code)
	}

	var lockResp RoomResponse
	if err := json.NewDecoder(lockW.Body).Decode(&lockResp); err != nil {
		t.Fatalf("failed to decode lock response: %v", err)
	}

	if lockResp.Status != "locked" {
		t.Errorf("expected status 'locked', got %q", lockResp.Status)
	}
}

func TestHandleLockRoomNotFound(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/non-existent-room/lock", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleUnlockRoom(t *testing.T) {
	s := newTestServer(t)

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// Lock the room first
	lockReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+createResp.ID+"/lock", nil)
	lockW := httptest.NewRecorder()
	s.router.ServeHTTP(lockW, lockReq)

	if lockW.Code != http.StatusOK {
		t.Fatalf("failed to lock room: status %d", lockW.Code)
	}

	// Now unlock the room
	unlockReq := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/"+createResp.ID+"/lock", nil)
	unlockW := httptest.NewRecorder()
	s.router.ServeHTTP(unlockW, unlockReq)

	if unlockW.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, unlockW.Code)
	}

	var unlockResp RoomResponse
	if err := json.NewDecoder(unlockW.Body).Decode(&unlockResp); err != nil {
		t.Fatalf("failed to decode unlock response: %v", err)
	}

	// Room should be back to 'created' state since no participants
	if unlockResp.Status != "created" {
		t.Errorf("expected status 'created', got %q", unlockResp.Status)
	}
}

func TestHandleUnlockRoomNotLocked(t *testing.T) {
	s := newTestServer(t)

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// Try to unlock room that is not locked
	unlockReq := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/"+createResp.ID+"/lock", nil)
	unlockW := httptest.NewRecorder()
	s.router.ServeHTTP(unlockW, unlockReq)

	if unlockW.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, unlockW.Code)
	}
}

func TestHandleUnlockRoomNotFound(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/non-existent-room/lock", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleCreateTokenRoomNotFound(t *testing.T) {
	s := newTestServer(t)

	body := CreateTokenRequest{
		ParticipantID: "user-123",
		Role:          "publisher",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/non-existent-room/token", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleCreateTokenWithoutGenerator(t *testing.T) {
	s := newTestServer(t)

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	// Try to create token without generator configured
	body := CreateTokenRequest{
		ParticipantID: "user-123",
		Role:          "publisher",
	}
	tokenBodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+createResp.ID+"/token", bytes.NewReader(tokenBodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// mockTokenGenerator is a mock token generator for testing.
type mockTokenGenerator struct {
	generateFunc func(roomID, participantID, role string, expiresInSeconds int) (string, error)
}

func (m *mockTokenGenerator) GenerateToken(roomID, participantID, role string, expiresInSeconds int) (string, error) {
	if m.generateFunc != nil {
		return m.generateFunc(roomID, participantID, role, expiresInSeconds)
	}
	return "mock-token", nil
}

func TestHandleCreateTokenWithGenerator(t *testing.T) {
	s := newTestServer(t)
	s.SetTokenGenerator(&mockTokenGenerator{})

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	body := CreateTokenRequest{
		ParticipantID: "user-123",
		Role:          "publisher",
		ExpiresIn:     3600,
	}
	tokenBodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+createResp.ID+"/token", bytes.NewReader(tokenBodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp TokenResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Token != "mock-token" {
		t.Errorf("expected token 'mock-token', got %q", resp.Token)
	}
}

func TestHandleCreateTokenInvalidRole(t *testing.T) {
	s := newTestServer(t)
	s.SetTokenGenerator(&mockTokenGenerator{
		generateFunc: func(roomID, participantID, role string, expiresInSeconds int) (string, error) {
			return "", auth.ErrInvalidRole
		},
	})

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	body := CreateTokenRequest{
		ParticipantID: "user-123",
		Role:          "invalid-role",
	}
	tokenBodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+createResp.ID+"/token", bytes.NewReader(tokenBodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleCreateTokenInvalidExpiresIn(t *testing.T) {
	s := newTestServer(t)
	s.SetTokenGenerator(&mockTokenGenerator{
		generateFunc: func(roomID, participantID, role string, expiresInSeconds int) (string, error) {
			return "", auth.ErrInvalidExpiresIn
		},
	})

	// First, create a room
	createBody := CreateRoomRequest{MaxParticipants: 10}
	bodyBytes, _ := json.Marshal(createBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", bytes.NewReader(bodyBytes))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	s.router.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("failed to create room: status %d", createW.Code)
	}

	var createResp CreateRoomResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	body := CreateTokenRequest{
		ParticipantID: "user-123",
		Role:          "publisher",
		ExpiresIn:     -1,
	}
	tokenBodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/"+createResp.ID+"/token", bytes.NewReader(tokenBodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
