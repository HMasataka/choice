package signaling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockHandler is a mock ConnectionHandler for testing.
type mockHandler struct {
	mu           sync.Mutex
	connects     []*Connection
	disconnects  []disconnectEvent
	messages     []messageEvent
	connectCh    chan *Connection
	disconnectCh chan disconnectEvent
	messageCh    chan messageEvent
}

type disconnectEvent struct {
	conn *Connection
	err  error
}

type messageEvent struct {
	conn    *Connection
	message []byte
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		connectCh:    make(chan *Connection, 10),
		disconnectCh: make(chan disconnectEvent, 10),
		messageCh:    make(chan messageEvent, 10),
	}
}

func (m *mockHandler) OnConnect(conn *Connection) {
	m.mu.Lock()
	m.connects = append(m.connects, conn)
	m.mu.Unlock()

	select {
	case m.connectCh <- conn:
	default:
	}
}

func (m *mockHandler) OnMessage(conn *Connection, message []byte) {
	evt := messageEvent{conn: conn, message: message}
	m.mu.Lock()
	m.messages = append(m.messages, evt)
	m.mu.Unlock()

	select {
	case m.messageCh <- evt:
	default:
	}
}

func (m *mockHandler) OnDisconnect(conn *Connection, err error) {
	evt := disconnectEvent{conn: conn, err: err}
	m.mu.Lock()
	m.disconnects = append(m.disconnects, evt)
	m.mu.Unlock()

	select {
	case m.disconnectCh <- evt:
	default:
	}
}

func (m *mockHandler) ConnectCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.connects)
}

func (m *mockHandler) DisconnectCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.disconnects)
}

func (m *mockHandler) MessageCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func TestHandler_BasicConnection(t *testing.T) {
	mock := newMockHandler()
	handler := NewHandler(DefaultHandlerConfig(), mock)

	server := httptest.NewServer(handler)
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	// Wait for connection
	select {
	case <-mock.connectCh:
		// OK
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	if mock.ConnectCount() != 1 {
		t.Errorf("expected 1 connect, got %d", mock.ConnectCount())
	}

	if handler.ConnectionCount() != 1 {
		t.Errorf("expected 1 active connection, got %d", handler.ConnectionCount())
	}
}

func TestHandler_MessageExchange(t *testing.T) {
	mock := newMockHandler()
	handler := NewHandler(DefaultHandlerConfig(), mock)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	// Wait for connection
	select {
	case <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	// Send message (must be valid JSON for rate limiting)
	testMessage := []byte(`{"message":"hello server"}`)
	if err := ws.WriteMessage(websocket.TextMessage, testMessage); err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// Wait for message
	select {
	case msg := <-mock.messageCh:
		if string(msg.message) != string(testMessage) {
			t.Errorf("expected message %q, got %q", testMessage, msg.message)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestHandler_BinaryMessageRejected(t *testing.T) {
	mock := newMockHandler()
	handler := NewHandler(DefaultHandlerConfig(), mock)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	// Wait for connection
	select {
	case <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	if err := ws.WriteMessage(websocket.BinaryMessage, []byte(`{"message":"binary"}`)); err != nil {
		t.Fatalf("failed to send binary message: %v", err)
	}

	select {
	case evt := <-mock.disconnectCh:
		if evt.err != ErrInvalidMessage {
			t.Errorf("expected ErrInvalidMessage, got %v", evt.err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for disconnect")
	}
}

func TestHandler_ServerToClientMessage(t *testing.T) {
	mock := newMockHandler()
	handler := NewHandler(DefaultHandlerConfig(), mock)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	// Wait for connection
	var conn *Connection
	select {
	case conn = <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	// Send message from server
	testMessage := []byte("hello client")
	if !conn.Send(testMessage) {
		t.Fatal("failed to queue message")
	}

	// Read message on client
	ws.SetReadDeadline(time.Now().Add(time.Second))
	_, message, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	if string(message) != string(testMessage) {
		t.Errorf("expected message %q, got %q", testMessage, message)
	}
}

func TestHandler_Broadcast(t *testing.T) {
	mock := newMockHandler()
	handler := NewHandler(DefaultHandlerConfig(), mock)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect two clients
	ws1, resp1, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp1 != nil {
		defer resp1.Body.Close()
	}
	if err != nil {
		t.Fatalf("failed to connect client 1: %v", err)
	}
	defer ws1.Close()

	ws2, resp2, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp2 != nil {
		defer resp2.Body.Close()
	}
	if err != nil {
		t.Fatalf("failed to connect client 2: %v", err)
	}
	defer ws2.Close()

	// Wait for both connections
	for i := 0; i < 2; i++ {
		select {
		case <-mock.connectCh:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for connect")
		}
	}

	if handler.ConnectionCount() != 2 {
		t.Errorf("expected 2 connections, got %d", handler.ConnectionCount())
	}

	// Broadcast message
	broadcastMsg := []byte("broadcast to all")
	handler.Broadcast(broadcastMsg)

	// Both clients should receive
	for i, ws := range []*websocket.Conn{ws1, ws2} {
		ws.SetReadDeadline(time.Now().Add(time.Second))
		_, msg, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("client %d failed to read: %v", i+1, err)
		}
		if string(msg) != string(broadcastMsg) {
			t.Errorf("client %d expected %q, got %q", i+1, broadcastMsg, msg)
		}
	}
}

func TestHandler_Disconnect(t *testing.T) {
	mock := newMockHandler()
	handler := NewHandler(DefaultHandlerConfig(), mock)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Wait for connection
	select {
	case <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	// Close client connection
	ws.Close()

	// Wait for disconnect
	select {
	case <-mock.disconnectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for disconnect")
	}

	if mock.DisconnectCount() != 1 {
		t.Errorf("expected 1 disconnect, got %d", mock.DisconnectCount())
	}

	// Connection count should be 0
	time.Sleep(50 * time.Millisecond) // Allow cleanup
	if handler.ConnectionCount() != 0 {
		t.Errorf("expected 0 connections after disconnect, got %d", handler.ConnectionCount())
	}
}

func TestHandler_CloseAll(t *testing.T) {
	mock := newMockHandler()
	handler := NewHandler(DefaultHandlerConfig(), mock)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect multiple clients
	var clients []*websocket.Conn
	for i := 0; i < 3; i++ {
		ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if resp != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			t.Fatalf("failed to connect client %d: %v", i, err)
		}
		clients = append(clients, ws)
	}

	// Wait for all connections
	for i := 0; i < 3; i++ {
		select {
		case <-mock.connectCh:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for connect")
		}
	}

	// Close all
	handler.CloseAll()

	// All clients should be disconnected
	time.Sleep(100 * time.Millisecond)

	for i, ws := range clients {
		ws.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, _, err := ws.ReadMessage()
		if err == nil {
			t.Errorf("client %d should have received close", i)
		}
	}
}

func TestHandler_CloseAllWithContext(t *testing.T) {
	mock := newMockHandler()
	handler := NewHandler(DefaultHandlerConfig(), mock)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	// Wait for connection
	select {
	case <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	// Close with context
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = handler.CloseAllWithContext(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHandler_OriginCheck(t *testing.T) {
	t.Run("allows all origins when not configured", func(t *testing.T) {
		mock := newMockHandler()
		cfg := DefaultHandlerConfig()
		cfg.AllowedOrigins = nil // Allow all
		handler := NewHandler(cfg, mock)

		server := httptest.NewServer(handler)
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

		dialer := websocket.Dialer{}
		header := http.Header{}
		header.Set("Origin", "http://example.com")

		ws, resp, err := dialer.Dial(wsURL, header)
		if resp != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			t.Fatalf("expected connection to be allowed: %v", err)
		}
		ws.Close()
	})

	t.Run("rejects disallowed origins", func(t *testing.T) {
		mock := newMockHandler()
		cfg := DefaultHandlerConfig()
		cfg.AllowedOrigins = []string{"http://allowed.com"}
		handler := NewHandler(cfg, mock)

		server := httptest.NewServer(handler)
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

		dialer := websocket.Dialer{}
		header := http.Header{}
		header.Set("Origin", "http://disallowed.com")

		_, resp, err := dialer.Dial(wsURL, header)
		if resp != nil {
			defer resp.Body.Close()
		}
		if err == nil {
			t.Fatal("expected connection to be rejected")
		}
	})

	t.Run("allows matching origins", func(t *testing.T) {
		mock := newMockHandler()
		cfg := DefaultHandlerConfig()
		cfg.AllowedOrigins = []string{"http://allowed.com"}
		handler := NewHandler(cfg, mock)

		server := httptest.NewServer(handler)
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

		dialer := websocket.Dialer{}
		header := http.Header{}
		header.Set("Origin", "http://allowed.com")

		ws, resp, err := dialer.Dial(wsURL, header)
		if resp != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			t.Fatalf("expected connection to be allowed: %v", err)
		}
		ws.Close()
	})
}

func TestDefaultHandlerConfig(t *testing.T) {
	cfg := DefaultHandlerConfig()

	if cfg.ReadBufferSize != 4096 {
		t.Errorf("expected ReadBufferSize 4096, got %d", cfg.ReadBufferSize)
	}
	if cfg.WriteBufferSize != 4096 {
		t.Errorf("expected WriteBufferSize 4096, got %d", cfg.WriteBufferSize)
	}
	if cfg.WriteWait != 10*time.Second {
		t.Errorf("expected WriteWait 10s, got %v", cfg.WriteWait)
	}
	if cfg.PongWait != 60*time.Second {
		t.Errorf("expected PongWait 60s, got %v", cfg.PongWait)
	}
	if cfg.PingPeriod != 54*time.Second {
		t.Errorf("expected PingPeriod 54s, got %v", cfg.PingPeriod)
	}
	if cfg.MaxMessageSize != 64*1024 {
		t.Errorf("expected MaxMessageSize 64KB, got %d", cfg.MaxMessageSize)
	}
}
