package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/HMasataka/choice/internal/signaling/protocol"
)

// mockRoomService is a mock implementation of RoomService for testing.
type mockRoomService struct {
	joinFunc       func(ctx context.Context, token string, sessionID string, metadata map[string]interface{}) (*JoinResponse, error)
	leaveFunc      func(ctx context.Context, participantID string) error
	disconnectFunc func(ctx context.Context, participantID string) error
}

func (m *mockRoomService) Join(ctx context.Context, token string, sessionID string, metadata map[string]interface{}) (*JoinResponse, error) {
	if m.joinFunc != nil {
		return m.joinFunc(ctx, token, sessionID, metadata)
	}
	return &JoinResponse{
		SessionID:     "test-session-id",
		RoomID:        "test-room-id",
		ParticipantID: "test-participant-id",
		Participants:  []protocol.ParticipantInfo{},
		IceServers:    []protocol.IceServer{},
	}, nil
}

func (m *mockRoomService) Leave(ctx context.Context, participantID string) error {
	if m.leaveFunc != nil {
		return m.leaveFunc(ctx, participantID)
	}
	return nil
}

func (m *mockRoomService) Disconnect(ctx context.Context, participantID string) error {
	if m.disconnectFunc != nil {
		return m.disconnectFunc(ctx, participantID)
	}
	return nil
}

// mockWebRTCService is a mock implementation of WebRTCService for testing.
type mockWebRTCService struct {
	handleOfferFunc     func(ctx context.Context, participantID string, sdp string) (string, error)
	handleAnswerFunc    func(ctx context.Context, participantID string, sdp string) error
	handleCandidateFunc func(ctx context.Context, participantID string, candidate string, sdpMid string, sdpMLineIndex *int) error
}

func (m *mockWebRTCService) HandleOffer(ctx context.Context, participantID string, sdp string) (string, error) {
	if m.handleOfferFunc != nil {
		return m.handleOfferFunc(ctx, participantID, sdp)
	}
	return "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n", nil
}

func (m *mockWebRTCService) HandleAnswer(ctx context.Context, participantID string, sdp string) error {
	if m.handleAnswerFunc != nil {
		return m.handleAnswerFunc(ctx, participantID, sdp)
	}
	return nil
}

func (m *mockWebRTCService) HandleCandidate(ctx context.Context, participantID string, candidate string, sdpMid string, sdpMLineIndex *int) error {
	if m.handleCandidateFunc != nil {
		return m.handleCandidateFunc(ctx, participantID, candidate, sdpMid, sdpMLineIndex)
	}
	return nil
}

// mockMediaService is a mock implementation of MediaService for testing.
type mockMediaService struct {
	publishFunc           func(ctx context.Context, participantID string, kind protocol.TrackKind, simulcast bool, metadata map[string]interface{}, label string) (*PublishResponse, error)
	unpublishFunc         func(ctx context.Context, participantID string, trackID string) error
	subscribeFunc         func(ctx context.Context, participantID string, publisherID string, trackID string, preferredLayer protocol.SimulcastLayer) (*SubscribeResponse, error)
	unsubscribeFunc       func(ctx context.Context, participantID string, subscriptionID string) error
	setPreferredLayerFunc func(ctx context.Context, participantID string, trackID string, layer protocol.SimulcastLayer) error
}

func (m *mockMediaService) Publish(ctx context.Context, participantID string, kind protocol.TrackKind, simulcast bool, metadata map[string]interface{}, label string) (*PublishResponse, error) {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, participantID, kind, simulcast, metadata, label)
	}
	return &PublishResponse{
		TrackID: "test-track-id",
		Mid:     "0",
	}, nil
}

func (m *mockMediaService) Unpublish(ctx context.Context, participantID string, trackID string) error {
	if m.unpublishFunc != nil {
		return m.unpublishFunc(ctx, participantID, trackID)
	}
	return nil
}

func (m *mockMediaService) Subscribe(ctx context.Context, participantID string, publisherID string, trackID string, preferredLayer protocol.SimulcastLayer) (*SubscribeResponse, error) {
	if m.subscribeFunc != nil {
		return m.subscribeFunc(ctx, participantID, publisherID, trackID, preferredLayer)
	}
	return &SubscribeResponse{
		SubscriptionID: "test-subscription-id",
		TrackID:        trackID,
		PublisherID:    publisherID,
	}, nil
}

func (m *mockMediaService) Unsubscribe(ctx context.Context, participantID string, subscriptionID string) error {
	if m.unsubscribeFunc != nil {
		return m.unsubscribeFunc(ctx, participantID, subscriptionID)
	}
	return nil
}

func (m *mockMediaService) SetPreferredLayer(ctx context.Context, participantID string, trackID string, layer protocol.SimulcastLayer) error {
	if m.setPreferredLayerFunc != nil {
		return m.setPreferredLayerFunc(ctx, participantID, trackID, layer)
	}
	return nil
}

func TestHandlers_Join_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	roomService := &mockRoomService{}
	handlers := NewHandlers(dispatcher, roomService, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers // handlers registers methods

	// Create a mock connection
	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}

	// Build join request
	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "join",
		"params": {
			"token": "test-token"
		}
	}`

	// Dispatch request
	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	// Parse response
	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	// Verify result
	var result protocol.JoinResult
	if err := resp.UnmarshalResult(&result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if result.SessionID != "test-session-id" {
		t.Errorf("sessionId = %s, want test-session-id", result.SessionID)
	}

	if result.RoomID != "test-room-id" {
		t.Errorf("roomId = %s, want test-room-id", result.RoomID)
	}

	if result.ParticipantID != "test-participant-id" {
		t.Errorf("participantId = %s, want test-participant-id", result.ParticipantID)
	}

	// Verify ICE servers are included
	if len(result.IceServers) == 0 {
		t.Error("iceServers should not be empty")
	}

	// Verify connection data is set
	participantID, ok := conn.GetData("participant_id")
	if !ok {
		t.Error("participant_id should be set on connection")
	}
	if participantID != "test-participant-id" {
		t.Errorf("participant_id = %v, want test-participant-id", participantID)
	}
}

func TestHandlers_Join_MissingToken(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	// Build join request without token
	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "join",
		"params": {}
	}`

	response := dispatcher.Dispatch(context.Background(), nil, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for missing token")
	}

	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestHandlers_Join_StubResponse(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	// No room service - should return stub response
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "join",
		"params": {
			"token": "test-token"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result protocol.JoinResult
	if err := resp.UnmarshalResult(&result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	// Stub response should have stub- prefix with unique UUID
	if len(result.ParticipantID) < 5 || result.ParticipantID[:5] != "stub-" {
		t.Errorf("participantId should start with 'stub-', got %s", result.ParticipantID)
	}
}

func TestHandlers_Leave_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	roomService := &mockRoomService{}
	handlers := NewHandlers(dispatcher, roomService, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	// Set participant ID on connection (simulating already joined)
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "leave",
		"params": {}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	// Verify connection data is cleared
	_, ok := conn.GetData("participant_id")
	if ok {
		t.Error("participant_id should be cleared on connection")
	}
}

func TestHandlers_Leave_NotInRoom(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	// Don't set participant_id - not in a room

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "leave",
		"params": {}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for not in room")
	}

	if resp.Error.Code != protocol.CodeNotInRoom {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeNotInRoom)
	}
}

func TestHandlers_Offer_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	rtcService := &mockWebRTCService{
		handleOfferFunc: func(ctx context.Context, participantID string, sdp string) (string, error) {
			return "v=0\r\no=- 1 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n", nil
		},
	}
	handlers := NewHandlers(dispatcher, nil, rtcService, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "offer",
		"params": {
			"sdp": "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result protocol.OfferResult
	if err := resp.UnmarshalResult(&result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if result.SDP == "" {
		t.Error("sdp should not be empty")
	}
}

func TestHandlers_Offer_MissingSDP(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "offer",
		"params": {}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for missing sdp")
	}

	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestHandlers_Answer_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	rtcService := &mockWebRTCService{}
	handlers := NewHandlers(dispatcher, nil, rtcService, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "answer",
		"params": {
			"sdp": "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandlers_Candidate_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	rtcService := &mockWebRTCService{}
	handlers := NewHandlers(dispatcher, nil, rtcService, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "candidate",
		"params": {
			"candidate": "candidate:123 1 udp 2122260223 192.168.1.1 12345 typ host",
			"sdpMid": "0",
			"sdpMLineIndex": 0
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandlers_Candidate_MissingCandidate(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "candidate",
		"params": {}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for missing candidate")
	}

	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestHandlers_ServiceError(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	roomService := &mockRoomService{
		joinFunc: func(ctx context.Context, token string, sessionID string, metadata map[string]interface{}) (*JoinResponse, error) {
			return nil, errors.New("room service error")
		},
	}
	handlers := NewHandlers(dispatcher, roomService, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "join",
		"params": {
			"token": "test-token"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error from service")
	}

	if resp.Error.Code != protocol.CodeInternalError {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInternalError)
	}
}

func TestHandlers_OnConnectionClosed(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	disconnectCalled := false
	roomService := &mockRoomService{
		disconnectFunc: func(ctx context.Context, participantID string) error {
			disconnectCalled = true
			if participantID != "test-participant-id" {
				t.Errorf("participantID = %s, want test-participant-id", participantID)
			}
			return nil
		},
	}
	handlers := NewHandlers(dispatcher, roomService, nil, nil, nil, DefaultHandlersConfig())

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	handlers.OnConnectionClosed(conn)

	if !disconnectCalled {
		t.Error("Disconnect should have been called")
	}

	// Verify connection data is cleared
	_, ok := conn.GetData("participant_id")
	if ok {
		t.Error("participant_id should be cleared")
	}
}

func TestHandlers_OnConnectionClosed_NotInRoom(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	disconnectCalled := false
	roomService := &mockRoomService{
		disconnectFunc: func(ctx context.Context, participantID string) error {
			disconnectCalled = true
			return nil
		},
	}
	handlers := NewHandlers(dispatcher, roomService, nil, nil, nil, DefaultHandlersConfig())

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	// Don't set participant_id

	handlers.OnConnectionClosed(conn)

	if disconnectCalled {
		t.Error("Disconnect should not have been called for connection not in room")
	}
}

// Media handler tests

func TestHandlers_Publish_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	mediaService := &mockMediaService{}
	handlers := NewHandlers(dispatcher, nil, nil, mediaService, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "publish",
		"params": {
			"kind": "video",
			"simulcast": true
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result protocol.PublishResult
	if err := resp.UnmarshalResult(&result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if result.TrackID != "test-track-id" {
		t.Errorf("trackId = %s, want test-track-id", result.TrackID)
	}

	if result.Mid != "0" {
		t.Errorf("mid = %s, want 0", result.Mid)
	}
}

func TestHandlers_Publish_InvalidKind(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "publish",
		"params": {
			"kind": "invalid"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for invalid kind")
	}

	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestHandlers_Publish_StubResponse(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	// No media service - should return stub response
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "publish",
		"params": {
			"kind": "video"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result protocol.PublishResult
	if err := resp.UnmarshalResult(&result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	// Stub response should have stub-track- prefix
	if len(result.TrackID) < 11 || result.TrackID[:11] != "stub-track-" {
		t.Errorf("trackId should start with 'stub-track-', got %s", result.TrackID)
	}
}

func TestHandlers_Unpublish_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	mediaService := &mockMediaService{}
	handlers := NewHandlers(dispatcher, nil, nil, mediaService, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "unpublish",
		"params": {
			"trackId": "test-track-id"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandlers_Unpublish_MissingTrackId(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "unpublish",
		"params": {}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for missing trackId")
	}

	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestHandlers_Subscribe_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	mediaService := &mockMediaService{}
	handlers := NewHandlers(dispatcher, nil, nil, mediaService, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "subscribe",
		"params": {
			"publisherId": "test-publisher-id",
			"trackId": "test-track-id",
			"preferredLayer": "h"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result protocol.SubscribeResult
	if err := resp.UnmarshalResult(&result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if result.SubscriptionID != "test-subscription-id" {
		t.Errorf("subscriptionId = %s, want test-subscription-id", result.SubscriptionID)
	}

	if result.TrackID != "test-track-id" {
		t.Errorf("trackId = %s, want test-track-id", result.TrackID)
	}

	if result.PublisherID != "test-publisher-id" {
		t.Errorf("publisherId = %s, want test-publisher-id", result.PublisherID)
	}
}

func TestHandlers_Subscribe_InvalidLayer(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "subscribe",
		"params": {
			"publisherId": "test-publisher-id",
			"trackId": "test-track-id",
			"preferredLayer": "invalid"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for invalid preferredLayer")
	}

	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestHandlers_Unsubscribe_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	mediaService := &mockMediaService{}
	handlers := NewHandlers(dispatcher, nil, nil, mediaService, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "unsubscribe",
		"params": {
			"subscriptionId": "test-subscription-id"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandlers_Unsubscribe_MissingSubscriptionId(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "unsubscribe",
		"params": {}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for missing subscriptionId")
	}

	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestHandlers_SetPreferredLayer_Success(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	mediaService := &mockMediaService{}
	handlers := NewHandlers(dispatcher, nil, nil, mediaService, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "setPreferredLayer",
		"params": {
			"trackId": "test-track-id",
			"layer": "m"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestHandlers_SetPreferredLayer_InvalidLayer(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "setPreferredLayer",
		"params": {
			"trackId": "test-track-id",
			"layer": "invalid"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for invalid layer")
	}

	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInvalidParams)
	}
}

func TestHandlers_Media_NotInRoom(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	handlers := NewHandlers(dispatcher, nil, nil, nil, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	// Don't set participant_id - not in room

	methods := []struct {
		name   string
		params string
	}{
		{"publish", `{"kind": "video"}`},
		{"unpublish", `{"trackId": "test-track-id"}`},
		{"subscribe", `{"publisherId": "test", "trackId": "test"}`},
		{"unsubscribe", `{"subscriptionId": "test"}`},
		{"setPreferredLayer", `{"trackId": "test", "layer": "h"}`},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			reqJSON := `{
				"jsonrpc": "2.0",
				"id": "550e8400-e29b-41d4-a716-446655440000",
				"method": "` + m.name + `",
				"params": ` + m.params + `
			}`

			response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

			var resp protocol.Response
			if err := json.Unmarshal(response, &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if resp.Error == nil {
				t.Fatal("expected error for not in room")
			}

			if resp.Error.Code != protocol.CodeNotInRoom {
				t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeNotInRoom)
			}
		})
	}
}

func TestHandlers_Media_ServiceError(t *testing.T) {
	dispatcher := NewDispatcher(DefaultDispatcherConfig())
	mediaService := &mockMediaService{
		publishFunc: func(ctx context.Context, participantID string, kind protocol.TrackKind, simulcast bool, metadata map[string]interface{}, label string) (*PublishResponse, error) {
			return nil, errors.New("media service error")
		},
	}
	handlers := NewHandlers(dispatcher, nil, nil, mediaService, nil, DefaultHandlersConfig())
	_ = handlers

	conn := &Connection{
		id:   "test-conn-id",
		data: make(map[string]interface{}),
	}
	conn.SetData("participant_id", "test-participant-id")

	reqJSON := `{
		"jsonrpc": "2.0",
		"id": "550e8400-e29b-41d4-a716-446655440000",
		"method": "publish",
		"params": {
			"kind": "video"
		}
	}`

	response := dispatcher.Dispatch(context.Background(), conn, []byte(reqJSON))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error from media service")
	}

	if resp.Error.Code != protocol.CodeInternalError {
		t.Errorf("error code = %d, want %d", resp.Error.Code, protocol.CodeInternalError)
	}
}
