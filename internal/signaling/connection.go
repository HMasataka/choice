package signaling

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// ConnectionState represents the state of a WebSocket connection.
type ConnectionState int32

const (
	// StateConnecting indicates the connection is being established.
	StateConnecting ConnectionState = iota
	// StateConnected indicates the connection is active.
	StateConnected
	// StateClosing indicates the connection is closing.
	StateClosing
	// StateClosed indicates the connection is closed.
	StateClosed
)

// String returns the string representation of the connection state.
func (s ConnectionState) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateClosing:
		return "closing"
	case StateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ConnectionMetadata contains metadata about the connection.
type ConnectionMetadata struct {
	// RemoteAddr is the remote address of the client.
	RemoteAddr string
	// UserAgent is the user agent string.
	UserAgent string
	// Headers contains selected headers from the original request.
	Headers map[string]string
	// ConnectedAt is when the connection was established.
	ConnectedAt time.Time
}

// Connection wraps a WebSocket connection with additional functionality.
type Connection struct {
	id       string
	ws       *websocket.Conn
	config   HandlerConfig
	metadata ConnectionMetadata

	state atomic.Int32
	send  chan []byte
	done  chan struct{}

	mu          sync.RWMutex
	closeCode   int
	closeReason string
	data        map[string]interface{} // Custom data storage
}

// NewConnection creates a new Connection wrapper.
func NewConnection(ws *websocket.Conn, cfg HandlerConfig, r *http.Request) *Connection {
	c := &Connection{
		id:     uuid.New().String(),
		ws:     ws,
		config: cfg,
		metadata: ConnectionMetadata{
			RemoteAddr:  r.RemoteAddr,
			UserAgent:   r.UserAgent(),
			ConnectedAt: time.Now(),
			Headers: map[string]string{
				"Origin": r.Header.Get("Origin"),
			},
		},
		send: make(chan []byte, 256), // Buffered channel for outgoing messages
		done: make(chan struct{}),
		data: make(map[string]interface{}),
	}

	c.state.Store(int32(StateConnected))

	return c
}

// ID returns the unique connection ID.
func (c *Connection) ID() string {
	return c.id
}

// State returns the current connection state.
func (c *Connection) State() ConnectionState {
	return ConnectionState(c.state.Load())
}

// Metadata returns the connection metadata.
func (c *Connection) Metadata() ConnectionMetadata {
	return c.metadata
}

// RemoteAddr returns the remote address.
func (c *Connection) RemoteAddr() string {
	return c.metadata.RemoteAddr
}

// Send queues a message to be sent to the client.
// Returns false if the connection is closed or the send buffer is full.
func (c *Connection) Send(message []byte) bool {
	if c.State() != StateConnected {
		return false
	}

	select {
	case c.send <- message:
		return true
	default:
		// Buffer full, message dropped
		return false
	}
}

// SendSync sends a message synchronously (blocks until sent or timeout).
func (c *Connection) SendSync(message []byte, timeout time.Duration) error {
	if c.State() != StateConnected {
		return ErrConnectionClosed
	}

	c.ws.SetWriteDeadline(time.Now().Add(timeout))
	return c.ws.WriteMessage(websocket.TextMessage, message)
}

// Close closes the connection.
func (c *Connection) Close() {
	c.CloseWithReason(websocket.CloseNormalClosure, "")
}

// CloseWithReason closes the connection with a specific close code and reason.
func (c *Connection) CloseWithReason(code int, reason string) {
	if !c.state.CompareAndSwap(int32(StateConnected), int32(StateClosing)) {
		return // Already closing or closed
	}

	c.mu.Lock()
	c.closeCode = code
	c.closeReason = reason
	c.mu.Unlock()

	// Send close message
	message := websocket.FormatCloseMessage(code, reason)
	c.ws.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second))

	// Close channels and connection
	close(c.done)
	close(c.send)
	c.ws.Close()

	c.state.Store(int32(StateClosed))
}

// setCloseReason sets the close reason (called from readPump on client-initiated close).
func (c *Connection) setCloseReason(code int, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeCode = code
	c.closeReason = reason
}

// CloseCode returns the close code.
func (c *Connection) CloseCode() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closeCode
}

// CloseReason returns the close reason.
func (c *Connection) CloseReason() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closeReason
}

// SetData stores custom data associated with this connection.
func (c *Connection) SetData(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// GetData retrieves custom data associated with this connection.
func (c *Connection) GetData(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.data[key]
	return value, ok
}

// DeleteData removes custom data associated with this connection.
func (c *Connection) DeleteData(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}

// Duration returns how long the connection has been active.
func (c *Connection) Duration() time.Duration {
	return time.Since(c.metadata.ConnectedAt)
}

// IsConnected returns true if the connection is in the connected state.
func (c *Connection) IsConnected() bool {
	return c.State() == StateConnected
}

// IsClosed returns true if the connection is closed.
func (c *Connection) IsClosed() bool {
	state := c.State()
	return state == StateClosing || state == StateClosed
}
