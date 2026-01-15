package signaling

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Common errors for signaling operations.
var (
	ErrConnectionClosed = errors.New("connection closed")
	ErrSendFailed       = errors.New("failed to send message")
	ErrInvalidMessage   = errors.New("invalid message")
	ErrUpgradeFailed    = errors.New("failed to upgrade connection")
)

// HandlerConfig contains configuration for the WebSocket handler.
type HandlerConfig struct {
	// ReadBufferSize is the buffer size for reading messages (default: 4096).
	ReadBufferSize int
	// WriteBufferSize is the buffer size for writing messages (default: 4096).
	WriteBufferSize int
	// WriteWait is the time allowed to write a message (default: 10 seconds).
	WriteWait time.Duration
	// PongWait is the time allowed to read the next pong message (default: 60 seconds).
	PongWait time.Duration
	// PingPeriod is the frequency of ping messages (must be less than PongWait).
	PingPeriod time.Duration
	// MaxMessageSize is the maximum message size allowed (default: 64KB).
	MaxMessageSize int64
	// HandshakeTimeout is the timeout for the WebSocket handshake (default: 10 seconds).
	HandshakeTimeout time.Duration
	// EnableCompression enables per-message compression (default: false).
	EnableCompression bool
	// AllowedOrigins is a list of allowed origins for CORS (empty = all allowed).
	AllowedOrigins []string
}

// DefaultHandlerConfig returns the default handler configuration.
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		WriteWait:         10 * time.Second,
		PongWait:          60 * time.Second,
		PingPeriod:        54 * time.Second, // Must be less than PongWait
		MaxMessageSize:    64 * 1024,        // 64KB
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: false,
		AllowedOrigins:    nil, // Allow all by default
	}
}

// ConnectionHandler handles WebSocket connection events.
type ConnectionHandler interface {
	// OnConnect is called when a new connection is established.
	OnConnect(conn *Connection)
	// OnMessage is called when a message is received.
	OnMessage(conn *Connection, message []byte)
	// OnDisconnect is called when a connection is closed.
	OnDisconnect(conn *Connection, err error)
}

// Handler manages WebSocket connections and message handling.
type Handler struct {
	config   HandlerConfig
	upgrader websocket.Upgrader
	handler  ConnectionHandler

	mu          sync.RWMutex
	connections map[string]*Connection
}

// NewHandler creates a new WebSocket handler.
func NewHandler(cfg HandlerConfig, handler ConnectionHandler) *Handler {
	h := &Handler{
		config:      cfg,
		handler:     handler,
		connections: make(map[string]*Connection),
	}

	h.upgrader = websocket.Upgrader{
		ReadBufferSize:    cfg.ReadBufferSize,
		WriteBufferSize:   cfg.WriteBufferSize,
		HandshakeTimeout:  cfg.HandshakeTimeout,
		EnableCompression: cfg.EnableCompression,
		CheckOrigin:       h.checkOrigin,
	}

	return h
}

// checkOrigin validates the request origin.
func (h *Handler) checkOrigin(r *http.Request) bool {
	if len(h.config.AllowedOrigins) == 0 {
		return true // Allow all origins
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // No origin header, allow
	}

	for _, allowed := range h.config.AllowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}

	return false
}

// ServeHTTP handles HTTP requests and upgrades them to WebSocket connections.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Error is already written to response by upgrader
		return
	}

	// Create connection wrapper
	connection := NewConnection(conn, h.config, r)

	// Register connection
	h.mu.Lock()
	h.connections[connection.ID()] = connection
	h.mu.Unlock()

	// Notify handler of new connection
	if h.handler != nil {
		h.handler.OnConnect(connection)
	}

	// Start connection goroutines
	go h.readPump(connection)
	go h.writePump(connection)
}

// readPump reads messages from the WebSocket connection.
func (h *Handler) readPump(conn *Connection) {
	defer func() {
		h.removeConnection(conn)
		conn.Close()
	}()

	ws := conn.ws
	ws.SetReadLimit(h.config.MaxMessageSize)
	ws.SetReadDeadline(time.Now().Add(h.config.PongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(h.config.PongWait))
		return nil
	})

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				conn.setCloseReason(closeErr.Code, closeErr.Text)
			}

			if h.handler != nil {
				h.handler.OnDisconnect(conn, err)
			}
			return
		}

		if h.handler != nil {
			h.handler.OnMessage(conn, message)
		}
	}
}

// writePump writes messages to the WebSocket connection.
func (h *Handler) writePump(conn *Connection) {
	ticker := time.NewTicker(h.config.PingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case message, ok := <-conn.send:
			conn.ws.SetWriteDeadline(time.Now().Add(h.config.WriteWait))
			if !ok {
				// Channel closed
				conn.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.ws.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			conn.ws.SetWriteDeadline(time.Now().Add(h.config.WriteWait))
			if err := conn.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-conn.done:
			return
		}
	}
}

// removeConnection removes a connection from the registry.
func (h *Handler) removeConnection(conn *Connection) {
	h.mu.Lock()
	delete(h.connections, conn.ID())
	h.mu.Unlock()
}

// GetConnection returns a connection by ID.
func (h *Handler) GetConnection(id string) *Connection {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.connections[id]
}

// ConnectionCount returns the number of active connections.
func (h *Handler) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

// Broadcast sends a message to all connections.
func (h *Handler) Broadcast(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, conn := range h.connections {
		conn.Send(message)
	}
}

// BroadcastExcept sends a message to all connections except the specified one.
func (h *Handler) BroadcastExcept(message []byte, exceptID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for id, conn := range h.connections {
		if id != exceptID {
			conn.Send(message)
		}
	}
}

// CloseAll closes all connections.
func (h *Handler) CloseAll() {
	h.mu.Lock()
	connections := make([]*Connection, 0, len(h.connections))
	for _, conn := range h.connections {
		connections = append(connections, conn)
	}
	h.mu.Unlock()

	for _, conn := range connections {
		conn.Close()
	}
}

// CloseAllWithContext closes all connections with a context for graceful shutdown.
func (h *Handler) CloseAllWithContext(ctx context.Context) error {
	h.mu.Lock()
	connections := make([]*Connection, 0, len(h.connections))
	for _, conn := range h.connections {
		connections = append(connections, conn)
	}
	h.mu.Unlock()

	done := make(chan struct{})
	go func() {
		for _, conn := range connections {
			conn.CloseWithReason(websocket.CloseGoingAway, "server shutting down")
		}
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		// Force close remaining connections
		for _, conn := range connections {
			conn.Close()
		}
		return ctx.Err()
	}
}
