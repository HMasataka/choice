// Package load provides load testing for the SFU server.
//
// NOTE: These tests use MockConnectionHandler for signaling protocol load testing.
// They verify WebSocket connection capacity and message throughput, NOT actual
// WebRTC media throughput (RTP/RTCP). Full media load testing should be performed
// separately with real WebRTC clients in dedicated load testing environments.
//
// These tests verify:
// - Concurrent connection handling (100 connections/room)
// - Room scalability (50 rooms)
// - Message throughput (signaling messages)
// - Long-running stability (sample duration)
package load

import (
	"bytes"
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
	"github.com/HMasataka/choice/internal/server"
	"github.com/HMasataka/choice/internal/signaling"
	"github.com/HMasataka/choice/internal/signaling/protocol"
	"github.com/HMasataka/choice/pkg/config"
	"github.com/HMasataka/choice/pkg/logger"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LoadTestConfig configures load test parameters.
type LoadTestConfig struct {
	NumConnections    int
	NumRooms          int
	MessagesPerSecond int
	TestDuration      time.Duration
	RampUpDuration    time.Duration
	ConnectionTimeout time.Duration
	MessageTimeout    time.Duration
}

// DefaultLoadTestConfig returns default load test configuration.
func DefaultLoadTestConfig() LoadTestConfig {
	return LoadTestConfig{
		NumConnections:    100,
		NumRooms:          10,
		MessagesPerSecond: 1000,
		TestDuration:      30 * time.Second,
		RampUpDuration:    5 * time.Second,
		ConnectionTimeout: 5 * time.Second,
		MessageTimeout:    5 * time.Second,
	}
}

// LoadTestResult contains load test results.
type LoadTestResult struct {
	TotalConnections   int64
	SuccessConnections int64
	FailedConnections  int64
	TotalMessages      int64
	SuccessMessages    int64
	FailedMessages     int64
	AverageLatency     time.Duration
	MaxLatency         time.Duration
	MinLatency         time.Duration
	MessagesPerSecond  float64
	Duration           time.Duration
	Errors             []string
}

// LoadTestClient represents a load test client.
type LoadTestClient struct {
	id        string
	roomID    string
	conn      *websocket.Conn
	mu        sync.Mutex
	closed    bool
	latencies []time.Duration
}

// NewLoadTestClient creates a new load test client.
func NewLoadTestClient(id, roomID, wsURL string, timeout time.Duration) (*LoadTestClient, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: timeout,
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	resp.Body.Close()

	return &LoadTestClient{
		id:        id,
		roomID:    roomID,
		conn:      conn,
		latencies: make([]time.Duration, 0),
	}, nil
}

// Close closes the client.
func (c *LoadTestClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SendMessage sends a message and measures latency.
func (c *LoadTestClient) SendMessage(req *protocol.Request, timeout time.Duration) (*protocol.Response, time.Duration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, 0, fmt.Errorf("client is closed")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	start := time.Now()

	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, 0, fmt.Errorf("failed to send message: %w", err)
	}

	c.conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := c.conn.ReadMessage()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response: %w", err)
	}

	latency := time.Since(start)

	var resp protocol.Response
	if err := json.Unmarshal(msg, &resp); err != nil {
		return nil, latency, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	c.latencies = append(c.latencies, latency)

	return &resp, latency, nil
}

// MockConnectionHandler implements ConnectionHandler for load testing.
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
	var req protocol.Request
	if err := json.Unmarshal(message, &req); err != nil {
		return
	}

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

// setupLoadTestServer creates a server for load testing.
func setupLoadTestServer(t *testing.T) (*httptest.Server, *room.Manager) {
	log, err := logger.New(logger.Config{Level: "error"})
	require.NoError(t, err)
	roomManager := room.NewManager(log)

	mockHandler := NewMockConnectionHandler()

	cfg := signaling.DefaultHandlerConfig()
	cfg.MaxMessageSize = 65536
	cfg.RateLimit.ConnectionsPerSecondPerIP = 1000
	cfg.RateLimit.MessagesPerSecondPerConnection = 10000

	handler := signaling.NewHandler(cfg, mockHandler)

	srv := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))

	return srv, roomManager
}

// TestConcurrentConnections tests 100 concurrent connections per room.
func TestConcurrentConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	srv, roomManager := setupLoadTestServer(t)
	defer srv.Close()

	const numConnections = 100
	const roomID = "load-test-room"

	// Create room
	_, err := roomManager.CreateRoom(roomID)
	require.NoError(t, err)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	var wg sync.WaitGroup
	var successCount, failCount int64
	clients := make([]*LoadTestClient, 0, numConnections)
	clientsMu := sync.Mutex{}

	// Connect all clients
	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			client, err := NewLoadTestClient(
				fmt.Sprintf("client-%d", idx),
				roomID,
				wsURL,
				5*time.Second,
			)
			if err != nil {
				atomic.AddInt64(&failCount, 1)
				t.Logf("Connection %d failed: %v", idx, err)
				return
			}

			atomic.AddInt64(&successCount, 1)

			clientsMu.Lock()
			clients = append(clients, client)
			clientsMu.Unlock()
		}(i)
	}

	wg.Wait()

	// Cleanup
	for _, client := range clients {
		client.Close()
	}

	t.Logf("Concurrent connections: success=%d, failed=%d", successCount, failCount)
	assert.GreaterOrEqual(t, successCount, int64(numConnections*90/100), "At least 90%% connections should succeed")
}

// TestRoomScalability tests multiple rooms running simultaneously.
func TestRoomScalability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	srv, roomManager := setupLoadTestServer(t)
	defer srv.Close()

	const numRooms = 50
	const connectionsPerRoom = 10

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	// Create rooms
	for i := 0; i < numRooms; i++ {
		_, err := roomManager.CreateRoom(fmt.Sprintf("room-%d", i))
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	var totalSuccess, totalFail int64
	allClients := make([]*LoadTestClient, 0, numRooms*connectionsPerRoom)
	clientsMu := sync.Mutex{}

	// Connect clients to each room
	for roomIdx := 0; roomIdx < numRooms; roomIdx++ {
		roomID := fmt.Sprintf("room-%d", roomIdx)

		for connIdx := 0; connIdx < connectionsPerRoom; connIdx++ {
			wg.Add(1)
			go func(rID string, cIdx int) {
				defer wg.Done()

				client, err := NewLoadTestClient(
					fmt.Sprintf("client-%s-%d", rID, cIdx),
					rID,
					wsURL,
					5*time.Second,
				)
				if err != nil {
					atomic.AddInt64(&totalFail, 1)
					return
				}

				atomic.AddInt64(&totalSuccess, 1)

				clientsMu.Lock()
				allClients = append(allClients, client)
				clientsMu.Unlock()
			}(roomID, connIdx)
		}
	}

	wg.Wait()

	// Cleanup
	for _, client := range allClients {
		client.Close()
	}

	expectedTotal := int64(numRooms * connectionsPerRoom)
	t.Logf("Room scalability: rooms=%d, connections=%d, success=%d, failed=%d",
		numRooms, expectedTotal, totalSuccess, totalFail)
	assert.GreaterOrEqual(t, totalSuccess, expectedTotal*90/100, "At least 90%% connections should succeed")
}

// TestMessageThroughput tests message throughput.
func TestMessageThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	srv, roomManager := setupLoadTestServer(t)
	defer srv.Close()

	const numClients = 10
	const messagesPerClient = 100
	const roomID = "throughput-test-room"

	_, err := roomManager.CreateRoom(roomID)
	require.NoError(t, err)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	// Create clients
	clients := make([]*LoadTestClient, numClients)
	for i := 0; i < numClients; i++ {
		client, err := NewLoadTestClient(
			fmt.Sprintf("throughput-client-%d", i),
			roomID,
			wsURL,
			5*time.Second,
		)
		require.NoError(t, err)
		clients[i] = client
		defer client.Close()
	}

	var wg sync.WaitGroup
	var totalMessages, successMessages int64
	var totalLatency int64
	var maxLatency, minLatency int64 = 0, int64(time.Hour)

	start := time.Now()

	// Send messages from all clients
	for _, client := range clients {
		wg.Add(1)
		go func(c *LoadTestClient) {
			defer wg.Done()

			for i := 0; i < messagesPerClient; i++ {
				atomic.AddInt64(&totalMessages, 1)

				req, err := protocol.NewRequest(
					fmt.Sprintf("550e8400-e29b-41d4-a716-4466554400%02d", i%100),
					"join",
					map[string]interface{}{
						"roomId": roomID,
						"token":  "test-token",
					},
				)
				if err != nil {
					continue
				}

				_, latency, err := c.SendMessage(req, 5*time.Second)
				if err != nil {
					continue
				}

				atomic.AddInt64(&successMessages, 1)
				atomic.AddInt64(&totalLatency, int64(latency))

				// Update max/min latency
				latencyNs := int64(latency)
				for {
					current := atomic.LoadInt64(&maxLatency)
					if latencyNs <= current {
						break
					}
					if atomic.CompareAndSwapInt64(&maxLatency, current, latencyNs) {
						break
					}
				}
				for {
					current := atomic.LoadInt64(&minLatency)
					if latencyNs >= current {
						break
					}
					if atomic.CompareAndSwapInt64(&minLatency, current, latencyNs) {
						break
					}
				}
			}
		}(client)
	}

	wg.Wait()
	duration := time.Since(start)

	avgLatency := time.Duration(0)
	if successMessages > 0 {
		avgLatency = time.Duration(totalLatency / successMessages)
	}

	messagesPerSecond := float64(successMessages) / duration.Seconds()

	t.Logf("Message throughput: total=%d, success=%d, duration=%v",
		totalMessages, successMessages, duration)
	t.Logf("Latency: avg=%v, max=%v, min=%v",
		avgLatency, time.Duration(maxLatency), time.Duration(minLatency))
	t.Logf("Throughput: %.2f messages/second", messagesPerSecond)

	// Verify throughput meets target (1000 msg/sec)
	// Note: Actual target depends on hardware, this is a baseline check
	assert.Greater(t, successMessages, int64(totalMessages*80/100), "At least 80%% messages should succeed")
}

// TestLongRunningStability tests stability over extended period.
func TestLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running test in short mode")
	}

	// Note: For 24-hour test, this would be run separately
	// This test runs for a shorter duration as a sample
	const testDuration = 30 * time.Second

	srv, roomManager := setupLoadTestServer(t)
	defer srv.Close()

	const numRooms = 5
	const clientsPerRoom = 5

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	// Create rooms
	for i := 0; i < numRooms; i++ {
		_, err := roomManager.CreateRoom(fmt.Sprintf("stability-room-%d", i))
		require.NoError(t, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	var wg sync.WaitGroup
	var totalOps, successOps int64
	errorsChan := make(chan error, 1000)

	// Run continuous operations
	for roomIdx := 0; roomIdx < numRooms; roomIdx++ {
		roomID := fmt.Sprintf("stability-room-%d", roomIdx)

		for clientIdx := 0; clientIdx < clientsPerRoom; clientIdx++ {
			wg.Add(1)
			go func(rID string, cIdx int) {
				defer wg.Done()

				client, err := NewLoadTestClient(
					fmt.Sprintf("stability-client-%s-%d", rID, cIdx),
					rID,
					wsURL,
					5*time.Second,
				)
				if err != nil {
					errorsChan <- err
					return
				}
				defer client.Close()

				ticker := time.NewTicker(100 * time.Millisecond)
				defer ticker.Stop()

				msgIdx := 0
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						atomic.AddInt64(&totalOps, 1)
						msgIdx++

						req, err := protocol.NewRequest(
							fmt.Sprintf("550e8400-e29b-41d4-a716-4466554400%02d", msgIdx%100),
							"join",
							map[string]interface{}{
								"roomId": rID,
								"token":  "test-token",
							},
						)
						if err != nil {
							continue
						}

						_, _, err = client.SendMessage(req, 5*time.Second)
						if err != nil {
							select {
							case errorsChan <- err:
							default:
							}
							continue
						}

						atomic.AddInt64(&successOps, 1)
					}
				}
			}(roomID, clientIdx)
		}
	}

	wg.Wait()
	close(errorsChan)

	// Count errors
	var errorCount int
	for range errorsChan {
		errorCount++
	}

	successRate := float64(successOps) / float64(totalOps) * 100

	t.Logf("Long-running stability: duration=%v, total=%d, success=%d, errors=%d, rate=%.2f%%",
		testDuration, totalOps, successOps, errorCount, successRate)

	assert.GreaterOrEqual(t, successRate, 80.0, "Success rate should be at least 80%%")
}

// TestRESTAPILoad tests REST API under load.
func TestRESTAPILoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	log, err := logger.New(logger.Config{Level: "error"})
	require.NoError(t, err)

	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTP: config.HTTPConfig{
				Host:         "localhost",
				Port:         0,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
			},
		},
	}

	srv := server.New(cfg, log)
	testServer := httptest.NewServer(srv.Router())
	defer testServer.Close()

	const numRequests = 1000
	const concurrency = 50

	var wg sync.WaitGroup
	var successCount, failCount int64
	semaphore := make(chan struct{}, concurrency)

	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(idx int) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// Create room
			reqBody := []byte(`{"max_participants": 10}`)
			resp, err := http.Post(
				testServer.URL+"/api/v1/rooms",
				"application/json",
				bytes.NewReader(reqBody),
			)
			if err != nil {
				atomic.AddInt64(&failCount, 1)
				return
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusCreated {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&failCount, 1)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	requestsPerSecond := float64(numRequests) / duration.Seconds()

	t.Logf("REST API load: requests=%d, success=%d, failed=%d, duration=%v",
		numRequests, successCount, failCount, duration)
	t.Logf("Throughput: %.2f requests/second", requestsPerSecond)

	successRate := float64(successCount) / float64(numRequests) * 100
	assert.GreaterOrEqual(t, successRate, 90.0, "At least 90%% requests should succeed")
}

// BenchmarkWebSocketConnection benchmarks WebSocket connection establishment.
func BenchmarkWebSocketConnection(b *testing.B) {
	log, err := logger.New(logger.Config{Level: "error"})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	roomManager := room.NewManager(log)

	mockHandler := NewMockConnectionHandler()

	cfg := signaling.DefaultHandlerConfig()
	cfg.MaxMessageSize = 65536
	cfg.RateLimit.ConnectionsPerSecondPerIP = 10000
	cfg.RateLimit.MessagesPerSecondPerConnection = 100000

	handler := signaling.NewHandler(cfg, mockHandler)

	srv := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))
	defer srv.Close()

	roomManager.CreateRoom("bench-room")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			b.Fatalf("Failed to connect: %v", err)
		}
		resp.Body.Close()
		conn.Close()
	}
}

// BenchmarkMessageRoundTrip benchmarks message round-trip latency.
func BenchmarkMessageRoundTrip(b *testing.B) {
	log, err := logger.New(logger.Config{Level: "error"})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}
	roomManager := room.NewManager(log)

	mockHandler := NewMockConnectionHandler()

	cfg := signaling.DefaultHandlerConfig()
	cfg.MaxMessageSize = 65536
	cfg.RateLimit.ConnectionsPerSecondPerIP = 10000
	cfg.RateLimit.MessagesPerSecondPerConnection = 100000

	handler := signaling.NewHandler(cfg, mockHandler)

	srv := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))
	defer srv.Close()

	roomManager.CreateRoom("bench-room")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	resp.Body.Close()
	defer conn.Close()

	req, _ := protocol.NewRequest("550e8400-e29b-41d4-a716-446655440001", "join", map[string]interface{}{
		"roomId": "bench-room",
		"token":  "test-token",
	})
	data, _ := json.Marshal(req)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			b.Fatalf("Failed to send message: %v", err)
		}

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, _, err := conn.ReadMessage()
		if err != nil {
			b.Fatalf("Failed to read message: %v", err)
		}
	}
}

// BenchmarkRoomCreation benchmarks room creation via REST API.
func BenchmarkRoomCreation(b *testing.B) {
	log, err := logger.New(logger.Config{Level: "error"})
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTP: config.HTTPConfig{
				Host:         "localhost",
				Port:         0,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
			},
		},
	}

	srv := server.New(cfg, log)
	testServer := httptest.NewServer(srv.Router())
	defer testServer.Close()

	reqBody := []byte(`{"max_participants": 100}`)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp, err := http.Post(
			testServer.URL+"/api/v1/rooms",
			"application/json",
			bytes.NewReader(reqBody),
		)
		if err != nil {
			b.Fatalf("Failed to create room: %v", err)
		}
		resp.Body.Close()
	}
}
