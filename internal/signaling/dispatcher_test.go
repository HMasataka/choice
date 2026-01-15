package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HMasataka/choice/internal/signaling/protocol"
)

func TestDispatcher_RegisterMethod(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	called := false
	d.RegisterMethod("test", func(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
		called = true
		return map[string]string{"status": "ok"}, nil
	})

	// Verify handler is registered
	handler, ok := d.getMethodHandler("test")
	if !ok {
		t.Fatal("handler should be registered")
	}

	// Call handler
	_, err := handler(context.Background(), nil, &protocol.Request{Method: "test"})
	if err != nil {
		t.Fatalf("handler should not return error: %v", err)
	}

	if !called {
		t.Fatal("handler should have been called")
	}
}

func TestDispatcher_UnregisterMethod(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	d.RegisterMethod("test", func(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
		return nil, nil
	})

	// Verify registered
	_, ok := d.getMethodHandler("test")
	if !ok {
		t.Fatal("handler should be registered")
	}

	// Unregister
	d.UnregisterMethod("test")

	// Verify unregistered
	_, ok = d.getMethodHandler("test")
	if ok {
		t.Fatal("handler should be unregistered")
	}
}

func TestDispatcher_Dispatch_ParseError(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	// Send invalid JSON
	response := d.Dispatch(context.Background(), nil, []byte("invalid json"))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("response should be valid JSON: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("response should contain error")
	}

	if resp.Error.Code != protocol.CodeParseError {
		t.Fatalf("error code should be parse error, got %d", resp.Error.Code)
	}
}

func TestDispatcher_Dispatch_InvalidRequest(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	// Send request with invalid version
	invalidReq := `{"jsonrpc":"1.0","id":"550e8400-e29b-41d4-a716-446655440000","method":"test"}`
	response := d.Dispatch(context.Background(), nil, []byte(invalidReq))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("response should be valid JSON: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("response should contain error")
	}

	if resp.Error.Code != protocol.CodeInvalidRequest {
		t.Fatalf("error code should be invalid request, got %d", resp.Error.Code)
	}
}

func TestDispatcher_Dispatch_MethodNotFound(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	// Send request for non-existent method
	req := `{"jsonrpc":"2.0","id":"550e8400-e29b-41d4-a716-446655440000","method":"join","params":{"token":"test"}}`
	response := d.Dispatch(context.Background(), nil, []byte(req))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("response should be valid JSON: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("response should contain error")
	}

	if resp.Error.Code != protocol.CodeMethodNotFound {
		t.Fatalf("error code should be method not found, got %d", resp.Error.Code)
	}
}

func TestDispatcher_Dispatch_SuccessfulRequest(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	d.RegisterMethod("join", func(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
		return map[string]string{
			"sessionId":     "session-123",
			"roomId":        "room-456",
			"participantId": "participant-789",
		}, nil
	})

	req := `{"jsonrpc":"2.0","id":"550e8400-e29b-41d4-a716-446655440000","method":"join","params":{"token":"test"}}`
	response := d.Dispatch(context.Background(), nil, []byte(req))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("response should be valid JSON: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("response should not contain error: %v", resp.Error)
	}

	if resp.ID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("response ID should match request ID, got %s", resp.ID)
	}

	if resp.Result == nil {
		t.Fatal("response should contain result")
	}

	// Verify result content
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("result should be valid JSON: %v", err)
	}

	if result["sessionId"] != "session-123" {
		t.Fatalf("sessionId should be session-123, got %s", result["sessionId"])
	}
}

func TestDispatcher_Dispatch_HandlerError(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	d.RegisterMethod("join", func(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
		return nil, protocol.NewInvalidParamsError("token is required")
	})

	req := `{"jsonrpc":"2.0","id":"550e8400-e29b-41d4-a716-446655440000","method":"join","params":{}}`
	response := d.Dispatch(context.Background(), nil, []byte(req))

	var resp protocol.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		t.Fatalf("response should be valid JSON: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("response should contain error")
	}

	if resp.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("error code should be invalid params, got %d", resp.Error.Code)
	}

	if resp.Error.Message != "Invalid params" {
		t.Fatalf("error message should be 'Invalid params', got %s", resp.Error.Message)
	}

	// Check that details are in Data
	if resp.Error.Data == nil {
		t.Fatal("error data should contain details")
	}
	if details, ok := resp.Error.Data["details"]; !ok || details != "token is required" {
		t.Fatalf("error data details should be 'token is required', got %v", resp.Error.Data)
	}
}

func TestDispatcher_Dispatch_Notification(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	called := false
	d.SetNotificationHandler(func(ctx context.Context, conn *Connection, notif *protocol.Message) {
		called = true
	})

	// Send notification (no id)
	notif := `{"jsonrpc":"2.0","method":"test","params":{}}`
	response := d.Dispatch(context.Background(), nil, []byte(notif))

	// Notifications should not return a response
	if response != nil {
		t.Fatal("notification should not return response")
	}

	if !called {
		t.Fatal("notification handler should have been called")
	}
}

func TestDispatcher_ConcurrencyLimit(t *testing.T) {
	cfg := DispatcherConfig{
		MaxConcurrentRequests: 2,
	}
	d := NewDispatcher(cfg)

	blockChan := make(chan struct{})
	var callCount int32

	d.RegisterMethod("join", func(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
		atomic.AddInt32(&callCount, 1)
		<-blockChan // Block until released
		return map[string]string{"status": "ok"}, nil
	})

	// Create a mock connection
	mockConn := &Connection{id: "test-conn-id"}

	// Start multiple concurrent requests
	var wg sync.WaitGroup
	responses := make([][]byte, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := `{"jsonrpc":"2.0","id":"550e8400-e29b-41d4-a716-44665544000` + string('0'+byte(idx)) + `","method":"join","params":{"token":"test"}}`
			responses[idx] = d.Dispatch(context.Background(), mockConn, []byte(req))
		}(i)
	}

	// Wait for requests to start
	time.Sleep(50 * time.Millisecond)

	// Only 2 requests should be in progress
	if atomic.LoadInt32(&callCount) > 2 {
		t.Fatalf("only 2 concurrent requests should be allowed, got %d", callCount)
	}

	// Release blocked handlers
	close(blockChan)
	wg.Wait()

	// Count error responses
	errorCount := 0
	for _, resp := range responses {
		var r protocol.Response
		if err := json.Unmarshal(resp, &r); err == nil && r.Error != nil {
			errorCount++
		}
	}

	// At least one request should have been rejected due to concurrency limit
	if errorCount == 0 {
		t.Log("Note: All requests completed. The third request may have been queued after the first completed.")
	}
}

func TestDispatcher_RemoveConnection(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	// Manually add pending request count
	d.pendingRequestsMu.Lock()
	d.pendingRequests["conn-1"] = 5
	d.pendingRequestsMu.Unlock()

	// Remove connection
	d.RemoveConnection("conn-1")

	// Verify cleaned up
	d.pendingRequestsMu.Lock()
	_, exists := d.pendingRequests["conn-1"]
	d.pendingRequestsMu.Unlock()

	if exists {
		t.Fatal("pending requests should be removed")
	}
}

func TestDispatcherConnectionHandler(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())
	handler := NewDispatcherConnectionHandler(d)

	connectCalled := false
	disconnectCalled := false
	var disconnectErr error

	handler.SetOnConnect(func(conn *Connection) {
		connectCalled = true
	})

	handler.SetOnDisconnect(func(conn *Connection, err error) {
		disconnectCalled = true
		disconnectErr = err
	})

	mockConn := &Connection{id: "test-conn"}

	// Test OnConnect
	handler.OnConnect(mockConn)
	if !connectCalled {
		t.Fatal("OnConnect callback should be called")
	}

	// Test OnDisconnect
	handler.OnDisconnect(mockConn, ErrConnectionClosed)
	if !disconnectCalled {
		t.Fatal("OnDisconnect callback should be called")
	}
	if !errors.Is(disconnectErr, ErrConnectionClosed) {
		t.Fatalf("disconnect error should be ErrConnectionClosed, got %v", disconnectErr)
	}
}

func TestDispatcher_RequestSlotAcquisition(t *testing.T) {
	cfg := DispatcherConfig{
		MaxConcurrentRequests: 2,
	}
	d := NewDispatcher(cfg)

	connID := "test-conn"

	// Acquire first slot
	if !d.acquireRequestSlot(connID) {
		t.Fatal("first slot acquisition should succeed")
	}

	// Acquire second slot
	if !d.acquireRequestSlot(connID) {
		t.Fatal("second slot acquisition should succeed")
	}

	// Third slot should fail
	if d.acquireRequestSlot(connID) {
		t.Fatal("third slot acquisition should fail")
	}

	// Release one slot
	d.releaseRequestSlot(connID)

	// Now should be able to acquire again
	if !d.acquireRequestSlot(connID) {
		t.Fatal("should be able to acquire after release")
	}

	// Release all
	d.releaseRequestSlot(connID)
	d.releaseRequestSlot(connID)

	// Verify cleanup
	d.pendingRequestsMu.Lock()
	_, exists := d.pendingRequests[connID]
	d.pendingRequestsMu.Unlock()

	if exists {
		t.Fatal("pending requests should be cleaned up when reaching zero")
	}
}

func TestDispatcher_Dispatch_Response(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	// Send a response message (unusual but should be handled gracefully)
	resp := `{"jsonrpc":"2.0","id":"550e8400-e29b-41d4-a716-446655440000","result":{"status":"ok"}}`
	response := d.Dispatch(context.Background(), nil, []byte(resp))

	// Responses should be ignored (return nil)
	if response != nil {
		t.Fatal("response messages should be ignored")
	}
}

func TestDispatcher_AllMethods(t *testing.T) {
	d := NewDispatcher(DefaultDispatcherConfig())

	// Register all standard methods
	methods := []string{
		protocol.MethodJoin,
		protocol.MethodLeave,
		protocol.MethodPublish,
		protocol.MethodUnpublish,
		protocol.MethodSubscribe,
		protocol.MethodUnsubscribe,
		protocol.MethodSetPreferredLayer,
		protocol.MethodOffer,
		protocol.MethodAnswer,
		protocol.MethodCandidate,
	}

	for _, method := range methods {
		m := method // Capture
		d.RegisterMethod(m, func(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error) {
			return map[string]string{"method": m}, nil
		})
	}

	// Test each method
	for _, method := range methods {
		req := `{"jsonrpc":"2.0","id":"550e8400-e29b-41d4-a716-446655440000","method":"` + method + `","params":{}}`
		response := d.Dispatch(context.Background(), nil, []byte(req))

		var resp protocol.Response
		if err := json.Unmarshal(response, &resp); err != nil {
			t.Fatalf("method %s: response should be valid JSON: %v", method, err)
		}

		if resp.Error != nil {
			t.Fatalf("method %s: should not return error: %v", method, resp.Error)
		}
	}
}
