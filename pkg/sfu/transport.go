package sfu

import (
	"sync"

	"github.com/gorilla/websocket"
)

// wsConn wraps a WebSocket connection with thread-safe write operations.
type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func newWSConn(conn *websocket.Conn) *wsConn {
	return &wsConn{conn: conn}
}

// WriteMessage writes a message to the WebSocket connection in a thread-safe manner.
func (w *wsConn) WriteMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(messageType, data)
}

// ReadMessage reads a message from the WebSocket connection.
func (w *wsConn) ReadMessage() (int, []byte, error) {
	return w.conn.ReadMessage()
}

// Close closes the WebSocket connection.
func (w *wsConn) Close() error {
	return w.conn.Close()
}
