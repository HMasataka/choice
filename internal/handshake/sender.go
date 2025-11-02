package handshake

import (
	"context"
	"errors"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"
)

// Sender defines the interface for sending messages to WebSocket connections
type Sender interface {
	// Send queues a message for sending. Returns error if the sender is closed or channel is full
	Send(ctx context.Context, message []byte) error
	// Start begins the sender's event loop in a separate goroutine
	Start(ctx context.Context)
	// Close gracefully shuts down the sender
	Close() error
	// IsClosed returns true if the sender has been closed
	IsClosed() bool
}

// SenderOptions configures the behavior of a WebSocketSender
type SenderOptions struct {
	WriteTimeout time.Duration
	PingInterval time.Duration
	BufferSize   int
}

// DefaultSenderOptions returns sensible default options for a WebSocketSender
func DefaultSenderOptions() SenderOptions {
	return SenderOptions{
		WriteTimeout: 10 * time.Second,
		PingInterval: 15 * time.Second,
		BufferSize:   256,
	}
}

// WebSocketSender implements the Sender interface for WebSocket connections
type WebSocketSender struct {
	ctx      context.Context
	conn     *ws.Conn
	options  SenderOptions
	sendChan chan []byte
	mutex    sync.RWMutex
	closed   bool
	cancel   context.CancelFunc
}

// NewWebSocketSender creates a new WebSocketSender instance
func NewWebSocketSender(ctx context.Context, conn *ws.Conn, options SenderOptions) *WebSocketSender {
	ctx, cancel := context.WithCancel(ctx)

	return &WebSocketSender{
		conn:     conn,
		options:  options,
		sendChan: make(chan []byte, options.BufferSize),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Send queues a message for sending
func (s *WebSocketSender) Send(ctx context.Context, message []byte) error {
	s.mutex.RLock()
	if s.closed {
		s.mutex.RUnlock()
		return errors.New("sender is closed")
	}
	s.mutex.RUnlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return errors.New("sender context done")
	case s.sendChan <- message:
		return nil
	default:
		return errors.New("send channel full or blocked")
	}
}

// Start begins the sender's event loop
func (s *WebSocketSender) Start(ctx context.Context) {
	go s.writePump(ctx)
}

// Close gracefully shuts down the sender
func (s *WebSocketSender) Close() error {
	s.mutex.Lock()
	if s.closed {
		s.mutex.Unlock()
		return nil
	}
	s.closed = true
	s.mutex.Unlock()

	s.cancel()
	close(s.sendChan)

	return nil
}

// IsClosed returns true if the sender has been closed
func (s *WebSocketSender) IsClosed() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.closed
}

// writePump handles the actual writing of messages to the WebSocket connection
func (s *WebSocketSender) writePump(ctx context.Context) {
	var ticker *time.Ticker
	if s.options.PingInterval > 0 {
		ticker = time.NewTicker(s.options.PingInterval)
		defer ticker.Stop()
	}

	for {
		if ticker != nil {
			select {
			case <-s.ctx.Done():
				return
			case <-ctx.Done():
				return
			case message, ok := <-s.sendChan:
				if err := s.writeMessage(message, ok); err != nil {
					return
				}
			case <-ticker.C:
				if err := s.writePing(); err != nil {
					return
				}
			}
		} else {
			select {
			case <-s.ctx.Done():
				return
			case <-ctx.Done():
				return
			case message, ok := <-s.sendChan:
				if err := s.writeMessage(message, ok); err != nil {
					return
				}
			}
		}
	}
}

// writeMessage writes a single message to the WebSocket
func (s *WebSocketSender) writeMessage(message []byte, ok bool) error {
	now := time.Now()
	s.conn.SetWriteDeadline(now.Add(s.options.WriteTimeout))

	if !ok {
		// Channel closed, send close message
		return s.conn.WriteMessage(ws.CloseMessage, []byte{})
	}

	return s.conn.WriteMessage(ws.TextMessage, message)
}

// writePing sends a ping message to keep the connection alive
func (s *WebSocketSender) writePing() error {
	s.conn.SetWriteDeadline(time.Now().Add(s.options.WriteTimeout))
	return s.conn.WriteMessage(ws.PingMessage, nil)
}
