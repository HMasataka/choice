package sfu

import (
	"sync"

	"github.com/gorilla/websocket"
)

// websocketConn wraps a WebSocket connection with thread-safe write operations.
type websocketConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func newWensocketConn(conn *websocket.Conn) *websocketConn {
	return &websocketConn{conn: conn}
}

// WriteMessage writes a message to the WebSocket connection in a thread-safe manner.
func (w *websocketConn) WriteMessage(messageType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(messageType, data)
}

// ReadMessage reads a message from the WebSocket connection.
func (w *websocketConn) ReadMessage() (int, []byte, error) {
	return w.conn.ReadMessage()
}

// Close closes the WebSocket connection.
func (w *websocketConn) Close() error {
	return w.conn.Close()
}
