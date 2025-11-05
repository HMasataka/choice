package handshake

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	payload "github.com/HMasataka/choice/payload/handshake"
	ws "github.com/gorilla/websocket"
)

type ConnectionOptions struct {
	ReadTimeout    time.Duration
	MaxMessageSize int64
	ReadBufferSize int
}

func DefaultConnectionOptions() ConnectionOptions {
	return ConnectionOptions{
		ReadTimeout:    90 * time.Second,
		MaxMessageSize: 512 * 1024, // 512KB
		ReadBufferSize: 1024,
	}
}

type Connection struct {
	ctx     context.Context
	conn    *ws.Conn
	cancel  context.CancelFunc
	router  *Router
	options ConnectionOptions
	sender  Sender
	mutex   sync.RWMutex
	closed  bool
}

func NewConnection(ctx context.Context, conn *ws.Conn, sender Sender, router *Router, options ConnectionOptions) *Connection {
	ctx, cancel := context.WithCancel(ctx)

	return &Connection{
		ctx:     ctx,
		conn:    conn,
		router:  router,
		cancel:  cancel,
		options: options,
		sender:  sender,
	}
}

func (c *Connection) send(ctx context.Context, message []byte) error {
	c.mutex.RLock()
	if c.closed {
		c.mutex.RUnlock()
		return errors.New("connection is closed")
	}
	c.mutex.RUnlock()

	return c.sender.Send(ctx, message)
}

func (c *Connection) Close() error {
	c.mutex.Lock()
	if c.closed {
		c.mutex.Unlock()
		return nil
	}
	c.closed = true
	c.mutex.Unlock()

	c.cancel()

	// Close sender first
	if err := c.sender.Close(); err != nil {
		// Log error but continue closing the connection
	}

	if err := c.conn.Close(); err != nil {
		return err
	}

	return nil
}

func (c *Connection) Context() context.Context {
	return c.ctx
}

func (c *Connection) Start(ctx context.Context) {
	done := make(chan struct{})

	go func() {
		defer close(done)
		c.readPump(ctx)
	}()

	// Start the sender in parallel
	c.sender.Start(ctx)

	<-done
}

func (c *Connection) readPump(ctx context.Context) {
	defer func() {
		c.Close()
	}()

	c.conn.SetReadLimit(c.options.MaxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(c.options.ReadTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(c.options.ReadTimeout))
		return nil
	})

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ctx.Done():
			return
		default:
			messageType, message, err := c.conn.ReadMessage()
			if err != nil {
				if ws.IsUnexpectedCloseError(err, ws.CloseGoingAway, ws.CloseAbnormalClosure) {
					return
				}
				return
			}

			if messageType != ws.TextMessage && messageType != ws.BinaryMessage {
				continue
			}

			var msg payload.Message
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Failed to unmarshal message: %v", err)
				continue
			}

			log.Printf("Received message type: %s", msg.Type)

			// Set timestamp if not present
			if msg.Timestamp.IsZero() {
				msg.Timestamp = time.Now()
			}

			response, err := c.router.Handle(ctx, &msg)
			if err != nil {
				log.Printf("Failed to handle message: %v", err)
				continue
			}

			if response != nil {
				// Ensure response has timestamp
				if response.Timestamp.IsZero() {
					response.Timestamp = time.Now()
				}

				respData, err := json.Marshal(response)
				if err != nil {
					continue
				}

				if err := c.send(ctx, respData); err != nil {
					continue
				}
			}
		}
	}
}
