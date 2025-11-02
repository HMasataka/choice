package handshake

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	payload "github.com/HMasataka/choice/payload/handshake"
	ws "github.com/gorilla/websocket"
)

type ConnectionOptions struct {
	WriteTimeout    time.Duration
	ReadTimeout     time.Duration
	PingInterval    time.Duration
	MaxMessageSize  int64
	ReadBufferSize  int
	WriteBufferSize int
}

func DefaultConnectionOptions() ConnectionOptions {
	return ConnectionOptions{
		WriteTimeout:    10 * time.Second,
		ReadTimeout:     90 * time.Second,
		PingInterval:    15 * time.Second,
		MaxMessageSize:  512 * 1024, // 512KB
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
}

type Connection struct {
	ctx      context.Context
	conn     *ws.Conn
	cancel   context.CancelFunc
	router   *Router
	options  ConnectionOptions
	sendChan chan []byte
	mutex    sync.RWMutex
	closed   bool
}

func NewConnection(conn *ws.Conn, router *Router, options ConnectionOptions) *Connection {
	ctx, cancel := context.WithCancel(context.Background())

	return &Connection{
		ctx:      ctx,
		conn:     conn,
		router:   router,
		cancel:   cancel,
		options:  options,
		sendChan: make(chan []byte, 256),
	}
}

func (c *Connection) Send(ctx context.Context, message []byte) error {
	c.mutex.RLock()
	if c.closed {
		c.mutex.RUnlock()
		return errors.New("connection is closed")
	}
	c.mutex.RUnlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return errors.New("connection context done")
	case c.sendChan <- message:
		return nil
	default:
		return errors.New("send channel full or blocked")
	}
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
	close(c.sendChan)

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

	go c.writePump(ctx)

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
				continue
			}

			// Set timestamp if not present
			if msg.Timestamp.IsZero() {
				msg.Timestamp = time.Now()
			}

			response, err := c.router.Handle(ctx, &msg)
			if err != nil {
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

				if err := c.Send(ctx, respData); err != nil {
					continue
				}
			}
		}
	}
}

func (c *Connection) writePump(ctx context.Context) {
	var ticker *time.Ticker
	if c.options.PingInterval > 0 {
		ticker = time.NewTicker(c.options.PingInterval)
		defer ticker.Stop()
	}

	for {
		if ticker != nil {
			select {
			case <-c.ctx.Done():
				return
			case <-ctx.Done():
				return
			case message, ok := <-c.sendChan:
				c.conn.SetWriteDeadline(time.Now().Add(c.options.WriteTimeout))

				if !ok {
					c.conn.WriteMessage(ws.CloseMessage, []byte{})
					return
				}

				if err := c.conn.WriteMessage(ws.TextMessage, message); err != nil {
					return
				}

				n := len(c.sendChan)
				for range n {
					select {
					case msg := <-c.sendChan:
						if err := c.conn.WriteMessage(ws.TextMessage, msg); err != nil {
							return
						}
					default:
					}
				}

			case <-ticker.C:
				c.conn.SetWriteDeadline(time.Now().Add(c.options.WriteTimeout))
				if err := c.conn.WriteMessage(ws.PingMessage, nil); err != nil {
					return
				}
			}
		} else {
			select {
			case <-c.ctx.Done():
				return
			case <-ctx.Done():
				return
			case message, ok := <-c.sendChan:
				c.conn.SetWriteDeadline(time.Now().Add(c.options.WriteTimeout))

				if !ok {
					c.conn.WriteMessage(ws.CloseMessage, []byte{})
					return
				}

				if err := c.conn.WriteMessage(ws.TextMessage, message); err != nil {
					return
				}

				n := len(c.sendChan)
				for range n {
					select {
					case msg := <-c.sendChan:
						if err := c.conn.WriteMessage(ws.TextMessage, msg); err != nil {
							return
						}
					default:
					}
				}
			}
		}
	}
}
