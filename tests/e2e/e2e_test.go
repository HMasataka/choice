// Package e2e provides end-to-end tests for the SFU server.
//
// NOTE: These tests use MockConnectionHandler for signaling protocol integration testing.
// They verify WebSocket JSON-RPC message exchange and protocol compliance, NOT full
// WebRTC media stack functionality (STUN/TURN/ICE/RTP). Full WebRTC integration tests
// should be run separately in nightly/manual test suites.
//
// These tests verify complete user flows including:
// - 2-party calls (signaling flow)
// - Multi-party conferences (signaling flow)
// - Join/leave scenarios
// - Network disruption recovery (session store integration)
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HMasataka/choice/internal/room"
	"github.com/HMasataka/choice/internal/signaling"
	"github.com/HMasataka/choice/internal/signaling/protocol"
	"github.com/HMasataka/choice/internal/store"
	"github.com/HMasataka/choice/pkg/logger"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient represents a simulated client for E2E testing.
type TestClient struct {
	t             *testing.T
	id            string
	conn          *websocket.Conn
	pc            *webrtc.PeerConnection
	mu            sync.Mutex
	messages      []protocol.Message
	notifications []protocol.Notification
	tracks        []*webrtc.TrackLocalStaticRTP
	remoteTracks  []*webrtc.TrackRemote
	closed        bool
}

// NewTestClient creates a new test client.
func NewTestClient(t *testing.T, id string, wsURL string) *TestClient {
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err, "failed to connect to WebSocket")
	defer resp.Body.Close()

	// Create PeerConnection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	require.NoError(t, err, "failed to create PeerConnection")

	client := &TestClient{
		t:             t,
		id:            id,
		conn:          conn,
		pc:            pc,
		messages:      make([]protocol.Message, 0),
		notifications: make([]protocol.Notification, 0),
		tracks:        make([]*webrtc.TrackLocalStaticRTP, 0),
		remoteTracks:  make([]*webrtc.TrackRemote, 0),
	}

	// Handle incoming tracks
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		client.mu.Lock()
		client.remoteTracks = append(client.remoteTracks, track)
		client.mu.Unlock()
	})

	return client
}

// Close closes the client connection.
func (c *TestClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}
	c.closed = true

	if c.conn != nil {
		c.conn.Close()
	}
	if c.pc != nil {
		c.pc.Close()
	}
}

// SendJoin sends a join request.
func (c *TestClient) SendJoin(roomID, token string) (*protocol.Response, error) {
	req, err := protocol.NewRequest("550e8400-e29b-41d4-a716-446655440001", "join", map[string]interface{}{
		"roomId": roomID,
		"token":  token,
	})
	if err != nil {
		return nil, err
	}
	return c.sendRequest(req)
}

// SendLeave sends a leave request.
func (c *TestClient) SendLeave() (*protocol.Response, error) {
	req, err := protocol.NewRequest("550e8400-e29b-41d4-a716-446655440002", "leave", nil)
	if err != nil {
		return nil, err
	}
	return c.sendRequest(req)
}

// SendPublish sends a publish request.
func (c *TestClient) SendPublish(kind string, simulcast bool) (*protocol.Response, error) {
	req, err := protocol.NewRequest("550e8400-e29b-41d4-a716-446655440003", "publish", map[string]interface{}{
		"kind":      kind,
		"simulcast": simulcast,
	})
	if err != nil {
		return nil, err
	}
	return c.sendRequest(req)
}

// SendSubscribe sends a subscribe request.
func (c *TestClient) SendSubscribe(publisherID, trackID string) (*protocol.Response, error) {
	req, err := protocol.NewRequest("550e8400-e29b-41d4-a716-446655440004", "subscribe", map[string]interface{}{
		"publisherId": publisherID,
		"trackId":     trackID,
	})
	if err != nil {
		return nil, err
	}
	return c.sendRequest(req)
}

// SendOffer sends an SDP offer.
func (c *TestClient) SendOffer(sdp string) (*protocol.Response, error) {
	req, err := protocol.NewRequest("550e8400-e29b-41d4-a716-446655440005", "offer", map[string]interface{}{
		"sdp": sdp,
	})
	if err != nil {
		return nil, err
	}
	return c.sendRequest(req)
}

// sendRequest sends a JSON-RPC request and waits for response.
func (c *TestClient) sendRequest(req *protocol.Request) (*protocol.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response with timeout
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := c.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var resp protocol.Response
	if err := json.Unmarshal(msg, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// ReadNotification reads a notification from the WebSocket.
func (c *TestClient) ReadNotification(timeout time.Duration) (*protocol.Notification, error) {
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var notification protocol.Notification
	if err := json.Unmarshal(msg, &notification); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.notifications = append(c.notifications, notification)
	c.mu.Unlock()

	return &notification, nil
}

// GetRemoteTracks returns the remote tracks.
func (c *TestClient) GetRemoteTracks() []*webrtc.TrackRemote {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.remoteTracks
}

// MockConnectionHandler implements ConnectionHandler for testing.
type MockConnectionHandler struct {
	mu          sync.Mutex
	connections map[string]*signaling.Connection
}

// NewMockConnectionHandler creates a new mock connection handler.
func NewMockConnectionHandler() *MockConnectionHandler {
	return &MockConnectionHandler{
		connections: make(map[string]*signaling.Connection),
	}
}

// OnConnect handles new connections.
func (h *MockConnectionHandler) OnConnect(conn *signaling.Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[conn.ID()] = conn
}

// OnMessage handles incoming messages.
func (h *MockConnectionHandler) OnMessage(conn *signaling.Connection, message []byte) {
	// Parse and handle messages
	var req protocol.Request
	if err := json.Unmarshal(message, &req); err != nil {
		return
	}

	// Send a simple response
	resp, err := protocol.NewSuccessResponse(req.ID, map[string]interface{}{
		"status": "ok",
	})
	if err != nil {
		return
	}
	data, _ := json.Marshal(resp)
	conn.Send(data)
}

// OnDisconnect handles disconnections.
func (h *MockConnectionHandler) OnDisconnect(conn *signaling.Connection, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, conn.ID())
}

// setupTestServer creates a test server for E2E testing.
func setupTestServer(t *testing.T) (*httptest.Server, *room.Manager, store.SessionStore) {
	log, err := logger.New(logger.Config{Level: "error"})
	require.NoError(t, err)
	roomManager := room.NewManager(log)
	sessionStore := store.NewMemoryStore()

	mockHandler := NewMockConnectionHandler()

	cfg := signaling.DefaultHandlerConfig()
	cfg.MaxMessageSize = 65536
	cfg.RateLimit.ConnectionsPerSecondPerIP = 100
	cfg.RateLimit.MessagesPerSecondPerConnection = 100

	handler := signaling.NewHandler(cfg, mockHandler)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))

	return server, roomManager, sessionStore
}

// TestTwoPartyCall tests a basic 2-party video call signaling flow.
// This test verifies WebSocket JSON-RPC protocol compliance using MockConnectionHandler.
func TestTwoPartyCall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	server, roomManager, _ := setupTestServer(t)
	defer server.Close()

	// Create a room
	testRoom, err := roomManager.CreateRoom("test-room-1")
	require.NoError(t, err)
	require.NotNil(t, testRoom)

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Create two clients
	client1 := NewTestClient(t, "client1", wsURL)
	defer client1.Close()

	client2 := NewTestClient(t, "client2", wsURL)
	defer client2.Close()

	// Client 1 joins the room
	resp1, err := client1.SendJoin("test-room-1", "test-token-1")
	require.NoError(t, err)
	require.NotNil(t, resp1, "response should not be nil")
	// Verify successful response structure (no error, has result)
	assert.Nil(t, resp1.Error, "join response should not contain error")
	assert.NotNil(t, resp1.Result, "join response should contain result")

	// Client 2 joins the room
	resp2, err := client2.SendJoin("test-room-1", "test-token-2")
	require.NoError(t, err)
	require.NotNil(t, resp2, "response should not be nil")
	assert.Nil(t, resp2.Error, "join response should not contain error")
	assert.NotNil(t, resp2.Result, "join response should contain result")

	// Both clients should have joined
	t.Log("Two-party call signaling flow completed successfully")
}

// TestMultiPartyConference tests a 5-party conference signaling flow.
// This test verifies concurrent join operations using MockConnectionHandler.
func TestMultiPartyConference(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	server, roomManager, _ := setupTestServer(t)
	defer server.Close()

	// Create a room
	testRoom, err := roomManager.CreateRoom("test-room-multi")
	require.NoError(t, err)
	require.NotNil(t, testRoom)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Create 5 clients
	clients := make([]*TestClient, 5)
	for i := 0; i < 5; i++ {
		clients[i] = NewTestClient(t, fmt.Sprintf("client%d", i), wsURL)
		defer clients[i].Close()
	}

	// All clients join
	successCount := 0
	for i, client := range clients {
		resp, err := client.SendJoin("test-room-multi", fmt.Sprintf("token-%d", i))
		require.NoError(t, err)
		require.NotNil(t, resp, "response should not be nil for client %d", i)
		// Verify successful response
		if resp.Error == nil && resp.Result != nil {
			successCount++
		}
	}

	assert.Equal(t, 5, successCount, "all 5 clients should join successfully")
	t.Log("Multi-party conference signaling flow completed with 5 participants")
}

// TestJoinLeaveScenario tests participants joining and leaving signaling flow.
// This test verifies join/leave protocol sequences using MockConnectionHandler.
func TestJoinLeaveScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	server, roomManager, _ := setupTestServer(t)
	defer server.Close()

	testRoom, err := roomManager.CreateRoom("test-room-join-leave")
	require.NoError(t, err)
	require.NotNil(t, testRoom)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Client 1 joins
	client1 := NewTestClient(t, "client1", wsURL)
	defer client1.Close()

	resp1, err := client1.SendJoin("test-room-join-leave", "token-1")
	require.NoError(t, err)
	require.NotNil(t, resp1, "join response should not be nil")
	assert.Nil(t, resp1.Error, "join should succeed for client1")

	// Client 2 joins
	client2 := NewTestClient(t, "client2", wsURL)

	resp2, err := client2.SendJoin("test-room-join-leave", "token-2")
	require.NoError(t, err)
	require.NotNil(t, resp2, "join response should not be nil")
	assert.Nil(t, resp2.Error, "join should succeed for client2")

	// Client 2 leaves
	leaveResp, err := client2.SendLeave()
	require.NoError(t, err)
	assert.Nil(t, leaveResp.Error, "leave should succeed for client2")
	client2.Close()

	// Client 3 joins
	client3 := NewTestClient(t, "client3", wsURL)
	defer client3.Close()

	resp3, err := client3.SendJoin("test-room-join-leave", "token-3")
	require.NoError(t, err)
	require.NotNil(t, resp3, "join response should not be nil")
	assert.Nil(t, resp3.Error, "join should succeed for client3")

	t.Log("Join/leave signaling flow completed successfully")
}

// TestNetworkDisruptionRecovery tests recovery from network disruption signaling flow.
// This test verifies session persistence and reconnection protocol using MockConnectionHandler.
// NOTE: The sessionStore is created but not wired to the signaling handler in this mock setup.
// This test primarily validates the reconnection protocol structure, not actual session recovery.
func TestNetworkDisruptionRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	server, roomManager, sessionStore := setupTestServer(t)
	defer server.Close()

	testRoom, err := roomManager.CreateRoom("test-room-recovery")
	require.NoError(t, err)
	require.NotNil(t, testRoom)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Client joins
	client := NewTestClient(t, "client-recovery", wsURL)

	resp, err := client.SendJoin("test-room-recovery", "token-recovery")
	require.NoError(t, err)
	require.NotNil(t, resp, "initial join response should not be nil")
	assert.Nil(t, resp.Error, "initial join should succeed")

	// Simulate network disruption by closing the connection
	client.Close()

	// Create a session for reconnection
	ctx := context.Background()
	session := &store.Session{
		SessionID:     "session-recovery",
		ParticipantID: "client-recovery",
		RoomID:        "test-room-recovery",
	}
	err = sessionStore.SaveSession(ctx, session)
	require.NoError(t, err)

	// Verify session was saved
	savedSession, err := sessionStore.GetSession(ctx, "session-recovery")
	require.NoError(t, err)
	assert.Equal(t, "client-recovery", savedSession.ParticipantID)
	assert.Equal(t, "test-room-recovery", savedSession.RoomID)

	// Wait for reconnection window (simulate short delay)
	time.Sleep(100 * time.Millisecond)

	// Reconnect
	newClient := NewTestClient(t, "client-recovery", wsURL)
	defer newClient.Close()

	// Join with session ID for reconnection
	req, err := protocol.NewRequest("550e8400-e29b-41d4-a716-446655440006", "join", map[string]interface{}{
		"roomId":    "test-room-recovery",
		"token":     "token-recovery",
		"sessionId": "session-recovery",
	})
	require.NoError(t, err)

	data, err := json.Marshal(req)
	require.NoError(t, err)

	newClient.mu.Lock()
	err = newClient.conn.WriteMessage(websocket.TextMessage, data)
	newClient.mu.Unlock()
	require.NoError(t, err)

	// Read response and verify
	newClient.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := newClient.conn.ReadMessage()
	require.NoError(t, err)

	var reconnResp protocol.Response
	err = json.Unmarshal(msg, &reconnResp)
	require.NoError(t, err)
	assert.Nil(t, reconnResp.Error, "reconnection join should succeed")

	t.Log("Network disruption recovery signaling flow completed successfully")
}

// TestConcurrentOperations tests concurrent operations from multiple clients.
// This test verifies WebSocket handler thread safety under concurrent load using MockConnectionHandler.
func TestConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	server, roomManager, _ := setupTestServer(t)
	defer server.Close()

	testRoom, err := roomManager.CreateRoom("test-room-concurrent")
	require.NoError(t, err)
	require.NotNil(t, testRoom)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	const numClients = 10
	var wg sync.WaitGroup
	errors := make(chan error, numClients)
	successCount := int64(0)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			client := NewTestClient(t, fmt.Sprintf("concurrent-client-%d", idx), wsURL)
			defer client.Close()

			// Join
			resp, err := client.SendJoin("test-room-concurrent", fmt.Sprintf("token-%d", idx))
			if err != nil {
				errors <- fmt.Errorf("client %d join error: %w", idx, err)
				return
			}
			if resp == nil {
				errors <- fmt.Errorf("client %d received nil response", idx)
				return
			}
			if resp.Error != nil {
				errors <- fmt.Errorf("client %d join failed with error: %v", idx, resp.Error)
				return
			}

			// Small delay to simulate real usage
			time.Sleep(time.Duration(idx*10) * time.Millisecond)

			// Leave
			leaveResp, err := client.SendLeave()
			if err != nil {
				errors <- fmt.Errorf("client %d leave error: %w", idx, err)
				return
			}
			if leaveResp.Error != nil {
				errors <- fmt.Errorf("client %d leave failed with error: %v", idx, leaveResp.Error)
				return
			}

			// Mark as successful
			atomic.AddInt64(&successCount, 1)
		}(i)
	}

	wg.Wait()
	close(errors)

	// Collect errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	assert.Empty(t, errs, "concurrent operations had errors: %v", errs)
	assert.Equal(t, int64(numClients), successCount, "all clients should complete successfully")
	t.Log("Concurrent operations signaling flow completed successfully")
}

// TestMediaPublishSubscribe tests media publish and subscribe signaling flow.
// This test verifies publish/subscribe protocol messages using MockConnectionHandler.
// NOTE: Actual WebRTC media exchange (SDP/ICE) is not fully validated in this mock setup.
func TestMediaPublishSubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	server, roomManager, _ := setupTestServer(t)
	defer server.Close()

	testRoom, err := roomManager.CreateRoom("test-room-media")
	require.NoError(t, err)
	require.NotNil(t, testRoom)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Publisher
	publisher := NewTestClient(t, "publisher", wsURL)
	defer publisher.Close()

	// Subscriber
	subscriber := NewTestClient(t, "subscriber", wsURL)
	defer subscriber.Close()

	// Both join
	pubJoinResp, err := publisher.SendJoin("test-room-media", "token-pub")
	require.NoError(t, err)
	require.NotNil(t, pubJoinResp, "publisher join response should not be nil")
	assert.Nil(t, pubJoinResp.Error, "publisher join should succeed")

	subJoinResp, err := subscriber.SendJoin("test-room-media", "token-sub")
	require.NoError(t, err)
	require.NotNil(t, subJoinResp, "subscriber join response should not be nil")
	assert.Nil(t, subJoinResp.Error, "subscriber join should succeed")

	// Publisher publishes video
	pubResp, err := publisher.SendPublish("video", false)
	require.NoError(t, err)
	require.NotNil(t, pubResp, "publish response should not be nil")
	assert.Nil(t, pubResp.Error, "publish should succeed")

	// Create and send offer from publisher
	offer, err := publisher.pc.CreateOffer(nil)
	require.NoError(t, err)

	err = publisher.pc.SetLocalDescription(offer)
	require.NoError(t, err)

	offerResp, err := publisher.SendOffer(offer.SDP)
	require.NoError(t, err)
	require.NotNil(t, offerResp, "offer response should not be nil")
	assert.Nil(t, offerResp.Error, "offer should succeed")

	t.Log("Media publish/subscribe signaling flow completed successfully")
}
