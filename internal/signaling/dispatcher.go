package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/HMasataka/choice/internal/signaling/protocol"
)

// MethodHandler is a function that handles a JSON-RPC method call.
// It receives the connection, request, and a context for cancellation.
// Returns the result to be sent as a response, or an error.
type MethodHandler func(ctx context.Context, conn *Connection, req *protocol.Request) (interface{}, *protocol.Error)

// NotificationHandler is a function that handles an incoming notification (no response expected).
type NotificationHandler func(ctx context.Context, conn *Connection, notif *protocol.Message)

// DispatcherConfig contains configuration for the dispatcher.
type DispatcherConfig struct {
	// MaxConcurrentRequests limits the number of concurrent requests per connection.
	// 0 means unlimited.
	MaxConcurrentRequests int
}

// DefaultDispatcherConfig returns the default dispatcher configuration.
func DefaultDispatcherConfig() DispatcherConfig {
	return DispatcherConfig{
		MaxConcurrentRequests: 10,
	}
}

// Dispatcher routes JSON-RPC messages to appropriate handlers.
type Dispatcher struct {
	config              DispatcherConfig
	methodHandlers      map[string]MethodHandler
	notificationHandler NotificationHandler

	mu sync.RWMutex

	// Track pending requests per connection for concurrency limiting
	pendingRequests   map[string]int
	pendingRequestsMu sync.Mutex
}

// NewDispatcher creates a new message dispatcher.
func NewDispatcher(cfg DispatcherConfig) *Dispatcher {
	return &Dispatcher{
		config:          cfg,
		methodHandlers:  make(map[string]MethodHandler),
		pendingRequests: make(map[string]int),
	}
}

// RegisterMethod registers a handler for a specific JSON-RPC method.
func (d *Dispatcher) RegisterMethod(method string, handler MethodHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.methodHandlers[method] = handler
}

// UnregisterMethod removes a handler for a specific JSON-RPC method.
func (d *Dispatcher) UnregisterMethod(method string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.methodHandlers, method)
}

// SetNotificationHandler sets the handler for incoming notifications.
func (d *Dispatcher) SetNotificationHandler(handler NotificationHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.notificationHandler = handler
}

// getMethodHandler returns the handler for a method (thread-safe).
func (d *Dispatcher) getMethodHandler(method string) (MethodHandler, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	handler, ok := d.methodHandlers[method]
	return handler, ok
}

// getNotificationHandler returns the notification handler (thread-safe).
func (d *Dispatcher) getNotificationHandler() NotificationHandler {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.notificationHandler
}

// Dispatch processes a raw JSON message and routes it to the appropriate handler.
// Returns the response bytes to send, or nil if no response is needed (notifications).
// Note: Per JSON-RPC 2.0 spec, if we cannot determine the request ID (e.g., parse error),
// the response ID should be null. However, since our schema requires UUID for ID,
// we use an empty string in such cases and rely on the client to handle it.
func (d *Dispatcher) Dispatch(ctx context.Context, conn *Connection, data []byte) []byte {
	// Parse the message
	msg, parseErr := protocol.ParseMessage(data)
	if parseErr != nil {
		// Return parse error response - parseErr is always *protocol.Error
		var protoErr *protocol.Error
		if !errors.As(parseErr, &protoErr) {
			protoErr = protocol.NewParseError(parseErr.Error())
		}
		// Note: We cannot include the request ID here because parsing failed.
		// Per JSON-RPC 2.0, the ID should be null when it cannot be determined.
		resp := protocol.NewErrorResponse("", protoErr)
		respData, _ := resp.Marshal() //nolint:errcheck // Marshal error is unlikely with valid struct
		return respData
	}

	// Extract ID for error responses (if available)
	requestID := ""
	if msg.ID != nil {
		requestID = *msg.ID
	}

	// Determine message type and route accordingly
	if msg.IsRequest() {
		return d.handleRequest(ctx, conn, msg)
	}

	if msg.IsNotification() {
		d.handleNotification(ctx, conn, msg)
		return nil // No response for notifications
	}

	if msg.IsResponse() {
		// Responses from clients are unusual but possible in bidirectional scenarios
		// For now, we ignore client responses
		return nil
	}

	// Invalid message type - use the ID if available
	resp := protocol.NewErrorResponse(requestID, protocol.NewInvalidRequestError("invalid message type"))
	respData, _ := resp.Marshal() //nolint:errcheck // Marshal error is unlikely with valid struct
	return respData
}

// handleRequest processes a JSON-RPC request.
// Note: The request has already been validated by ParseMessage (UUID format, etc.).
// We construct the Request struct directly from the Message.
func (d *Dispatcher) handleRequest(ctx context.Context, conn *Connection, msg *protocol.Message) []byte {
	requestID := ""
	if msg.ID != nil {
		requestID = *msg.ID
	}

	// Check concurrency limit (skip if conn is nil - used in tests)
	if d.config.MaxConcurrentRequests > 0 && conn != nil {
		if !d.acquireRequestSlot(conn.ID()) {
			resp := protocol.NewErrorResponse(requestID, protocol.NewInternalError("too many concurrent requests"))
			respData, _ := resp.Marshal() //nolint:errcheck // Marshal error is unlikely with valid struct
			return respData
		}
		defer d.releaseRequestSlot(conn.ID())
	}

	// Build request from the already-validated message
	// ParseMessage has already validated: JSON-RPC version, ID format (UUID), method presence
	req := &protocol.Request{
		JSONRPC: msg.JSONRPC,
		ID:      requestID,
		Method:  msg.Method,
		Params:  msg.Params,
	}

	// Find handler for the method
	handler, ok := d.getMethodHandler(req.Method)
	if !ok {
		resp := protocol.NewErrorResponse(requestID, protocol.NewMethodNotFoundError(req.Method))
		respData, _ := resp.Marshal() //nolint:errcheck // Marshal error is unlikely with valid struct
		return respData
	}

	// Execute handler
	// Note: Parameter validation is the responsibility of individual handlers
	result, handlerErr := handler(ctx, conn, req)
	if handlerErr != nil {
		resp := protocol.NewErrorResponse(requestID, handlerErr)
		respData, _ := resp.Marshal() //nolint:errcheck // Marshal error is unlikely with valid struct
		return respData
	}

	// Build success response
	resp, marshalErr := protocol.NewSuccessResponse(requestID, result)
	if marshalErr != nil {
		resp := protocol.NewErrorResponse(requestID, protocol.NewInternalError("failed to marshal result"))
		respData, _ := resp.Marshal() //nolint:errcheck // Marshal error is unlikely with valid struct
		return respData
	}

	respData, _ := resp.Marshal() //nolint:errcheck // Marshal error is unlikely with valid struct
	return respData
}

// handleNotification processes a JSON-RPC notification (no response).
func (d *Dispatcher) handleNotification(ctx context.Context, conn *Connection, msg *protocol.Message) {
	handler := d.getNotificationHandler()
	if handler != nil {
		handler(ctx, conn, msg)
	}
}

// acquireRequestSlot tries to acquire a request slot for concurrency limiting.
// Returns false if the limit is reached.
func (d *Dispatcher) acquireRequestSlot(connID string) bool {
	d.pendingRequestsMu.Lock()
	defer d.pendingRequestsMu.Unlock()

	current := d.pendingRequests[connID]
	if current >= d.config.MaxConcurrentRequests {
		return false
	}

	d.pendingRequests[connID] = current + 1
	return true
}

// releaseRequestSlot releases a request slot.
func (d *Dispatcher) releaseRequestSlot(connID string) {
	d.pendingRequestsMu.Lock()
	defer d.pendingRequestsMu.Unlock()

	current := d.pendingRequests[connID]
	if current > 0 {
		d.pendingRequests[connID] = current - 1
	}

	// Clean up entry if no pending requests
	if d.pendingRequests[connID] == 0 {
		delete(d.pendingRequests, connID)
	}
}

// RemoveConnection cleans up state for a disconnected connection.
func (d *Dispatcher) RemoveConnection(connID string) {
	d.pendingRequestsMu.Lock()
	defer d.pendingRequestsMu.Unlock()
	delete(d.pendingRequests, connID)
}

// SendNotification sends a notification to a connection.
// Returns true if the message was queued successfully, false if conn is nil or send fails.
func (d *Dispatcher) SendNotification(conn *Connection, method string, params interface{}) bool {
	if conn == nil {
		return false
	}

	notif, err := protocol.NewNotification(method, params)
	if err != nil {
		return false
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return false
	}

	return conn.Send(data)
}

// SendErrorNotification sends an error notification to a connection.
// Returns true if the message was queued successfully, false if conn is nil or send fails.
func (d *Dispatcher) SendErrorNotification(conn *Connection, code int, message string, fatal bool) bool {
	if conn == nil {
		return false
	}

	notif, _ := protocol.NewErrorNotification(code, message, fatal) //nolint:errcheck // NewErrorNotification error is unlikely
	data, err := json.Marshal(notif)
	if err != nil {
		return false
	}
	return conn.Send(data)
}

// SendResponse sends a response to a connection.
// Returns true if the message was queued successfully, false if conn is nil or send fails.
func (d *Dispatcher) SendResponse(conn *Connection, resp *protocol.Response) bool {
	if conn == nil {
		return false
	}

	data, err := resp.Marshal()
	if err != nil {
		return false
	}
	return conn.Send(data)
}

// DispatcherConnectionHandler implements ConnectionHandler and uses Dispatcher for message routing.
type DispatcherConnectionHandler struct {
	dispatcher *Dispatcher

	mu               sync.RWMutex
	onConnectFunc    func(conn *Connection)
	onDisconnectFunc func(conn *Connection, err error)
}

// NewDispatcherConnectionHandler creates a new connection handler that uses the dispatcher.
func NewDispatcherConnectionHandler(dispatcher *Dispatcher) *DispatcherConnectionHandler {
	return &DispatcherConnectionHandler{
		dispatcher: dispatcher,
	}
}

// SetOnConnect sets the callback for new connections.
// This method is thread-safe and can be called at any time.
func (h *DispatcherConnectionHandler) SetOnConnect(f func(conn *Connection)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onConnectFunc = f
}

// SetOnDisconnect sets the callback for disconnections.
// This method is thread-safe and can be called at any time.
func (h *DispatcherConnectionHandler) SetOnDisconnect(f func(conn *Connection, err error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onDisconnectFunc = f
}

// OnConnect is called when a new connection is established.
func (h *DispatcherConnectionHandler) OnConnect(conn *Connection) {
	h.mu.RLock()
	f := h.onConnectFunc
	h.mu.RUnlock()

	if f != nil {
		f(conn)
	}
}

// OnMessage is called when a message is received.
func (h *DispatcherConnectionHandler) OnMessage(conn *Connection, message []byte) {
	ctx := context.Background()
	response := h.dispatcher.Dispatch(ctx, conn, message)
	if response != nil {
		conn.Send(response)
	}
}

// OnDisconnect is called when a connection is closed.
func (h *DispatcherConnectionHandler) OnDisconnect(conn *Connection, err error) {
	h.dispatcher.RemoveConnection(conn.ID())

	h.mu.RLock()
	f := h.onDisconnectFunc
	h.mu.RUnlock()

	if f != nil {
		f(conn, err)
	}
}
