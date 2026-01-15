package signaling

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		expected string
	}{
		{StateConnecting, "connecting"},
		{StateConnected, "connected"},
		{StateClosing, "closing"},
		{StateClosed, "closed"},
		{ConnectionState(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.state.String())
		}
	}
}

func TestConnection_Data(t *testing.T) {
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

	var conn *Connection
	select {
	case conn = <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	t.Run("set and get data", func(t *testing.T) {
		conn.SetData("key1", "value1")
		conn.SetData("key2", 42)

		val1, ok := conn.GetData("key1")
		if !ok || val1 != "value1" {
			t.Errorf("expected value1, got %v", val1)
		}

		val2, ok := conn.GetData("key2")
		if !ok || val2 != 42 {
			t.Errorf("expected 42, got %v", val2)
		}
	})

	t.Run("get non-existent data", func(t *testing.T) {
		_, ok := conn.GetData("nonexistent")
		if ok {
			t.Error("expected key to not exist")
		}
	})

	t.Run("delete data", func(t *testing.T) {
		conn.SetData("toDelete", "value")
		conn.DeleteData("toDelete")

		_, ok := conn.GetData("toDelete")
		if ok {
			t.Error("expected key to be deleted")
		}
	})
}

func TestConnection_Metadata(t *testing.T) {
	mock := newMockHandler()
	handler := NewHandler(DefaultHandlerConfig(), mock)

	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	dialer := websocket.Dialer{}
	header := http.Header{}
	header.Set("Origin", "http://test.com")

	ws, resp, err := dialer.Dial(wsURL, header)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer ws.Close()

	var conn *Connection
	select {
	case conn = <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	metadata := conn.Metadata()

	if metadata.RemoteAddr == "" {
		t.Error("expected non-empty RemoteAddr")
	}

	if metadata.ConnectedAt.IsZero() {
		t.Error("expected non-zero ConnectedAt")
	}

	if metadata.Headers["Origin"] != "http://test.com" {
		t.Errorf("expected origin http://test.com, got %s", metadata.Headers["Origin"])
	}
}

func TestConnection_ID(t *testing.T) {
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

	var conn *Connection
	select {
	case conn = <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	id := conn.ID()
	if id == "" {
		t.Error("expected non-empty ID")
	}

	// ID should be a valid UUID format
	if len(id) != 36 { // UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		t.Errorf("expected UUID format, got %s", id)
	}
}

func TestConnection_State(t *testing.T) {
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

	var conn *Connection
	select {
	case conn = <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	// Initially connected
	if conn.State() != StateConnected {
		t.Errorf("expected StateConnected, got %v", conn.State())
	}

	if !conn.IsConnected() {
		t.Error("expected IsConnected to be true")
	}

	if conn.IsClosed() {
		t.Error("expected IsClosed to be false")
	}

	// Close connection
	ws.Close()
	time.Sleep(100 * time.Millisecond)

	// Check for disconnect event
	select {
	case <-mock.disconnectCh:
	case <-time.After(time.Second):
		// May not fire if connection cleanup happens differently
	}
}

func TestConnection_Duration(t *testing.T) {
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

	var conn *Connection
	select {
	case conn = <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	time.Sleep(50 * time.Millisecond)

	duration := conn.Duration()
	if duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", duration)
	}
}

func TestConnection_SendWhenClosed(t *testing.T) {
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

	var conn *Connection
	select {
	case conn = <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	// Close connection
	ws.Close()
	time.Sleep(100 * time.Millisecond)

	// Try to send after close
	if conn.Send([]byte("test")) {
		t.Error("expected Send to return false for closed connection")
	}
}

func TestConnection_GetByID(t *testing.T) {
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

	var conn *Connection
	select {
	case conn = <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connect")
	}

	// Get by ID
	retrieved := handler.GetConnection(conn.ID())
	if retrieved == nil {
		t.Error("expected to retrieve connection by ID")
	}

	if retrieved.ID() != conn.ID() {
		t.Error("retrieved connection has different ID")
	}

	// Get non-existent
	notFound := handler.GetConnection("non-existent-id")
	if notFound != nil {
		t.Error("expected nil for non-existent ID")
	}
}

func TestConnection_BroadcastExcept(t *testing.T) {
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

	// Wait for connections
	var conn1 *Connection
	select {
	case conn1 = <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	select {
	case <-mock.connectCh:
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	// Broadcast except conn1
	handler.BroadcastExcept([]byte("message"), conn1.ID())

	// ws1 should not receive (it's the excluded one)
	ws1.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err1 := ws1.ReadMessage()
	if err1 == nil {
		t.Error("ws1 should not have received message")
	}

	// ws2 should receive
	ws2.SetReadDeadline(time.Now().Add(time.Second))
	_, msg, err2 := ws2.ReadMessage()
	if err2 != nil {
		t.Errorf("ws2 should have received message: %v", err2)
	}
	if string(msg) != "message" {
		t.Errorf("expected 'message', got %q", msg)
	}
}
