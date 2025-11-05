package webrtc

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"
)

// DataChannel wraps a WebRTC data channel
type DataChannel struct {
	dc *webrtc.DataChannel

	messagesSent int64
	messagesRecv int64
	bytesSent    int64
	bytesRecv    int64

	onOpen    func()
	onClose   func()
	onMessage func([]byte)
	onError   func(error)

	mu sync.RWMutex
}

// NewDataChannel creates a new data channel wrapper
func NewDataChannel(dc *webrtc.DataChannel) *DataChannel {
	d := &DataChannel{
		dc: dc,
	}

	d.setupEventHandlers()

	return d
}

// Label returns the data channel label
func (d *DataChannel) Label() string {
	return d.dc.Label()
}

// ID returns the data channel ID
func (d *DataChannel) ID() *uint16 {
	return d.dc.ID()
}

// ReadyState returns the data channel ready state
func (d *DataChannel) ReadyState() webrtc.DataChannelState {
	return d.dc.ReadyState()
}

// Send sends data over the data channel
func (d *DataChannel) Send(data []byte) error {
	if d.dc.ReadyState() != webrtc.DataChannelStateOpen {
		return errors.New("data channel is not open")
	}

	if err := d.dc.Send(data); err != nil {
		return err
	}

	atomic.AddInt64(&d.messagesSent, 1)
	atomic.AddInt64(&d.bytesSent, int64(len(data)))

	return nil
}

// SendText sends text data over the data channel
func (d *DataChannel) SendText(text string) error {
	return d.Send([]byte(text))
}

// Close closes the data channel
func (d *DataChannel) Close() error {
	return d.dc.Close()
}

// OnOpen sets the open event handler
func (d *DataChannel) OnOpen(handler func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onOpen = handler
}

// OnClose sets the close event handler
func (d *DataChannel) OnClose(handler func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onClose = handler
}

// OnMessage sets the message event handler
func (d *DataChannel) OnMessage(handler func([]byte)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onMessage = handler
}

// OnError sets the error event handler
func (d *DataChannel) OnError(handler func(error)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onError = handler
}

// GetStats returns data channel statistics
func (d *DataChannel) GetStats() DataChannelStats {
	return DataChannelStats{
		Label:        d.Label(),
		State:        d.ReadyState().String(),
		MessagesSent: atomic.LoadInt64(&d.messagesSent),
		MessagesRecv: atomic.LoadInt64(&d.messagesRecv),
		BytesSent:    atomic.LoadInt64(&d.bytesSent),
		BytesRecv:    atomic.LoadInt64(&d.bytesRecv),
	}
}

func (d *DataChannel) setupEventHandlers() {
	d.dc.OnOpen(func() {
		d.mu.RLock()
		handler := d.onOpen
		d.mu.RUnlock()

		if handler != nil {
			handler()
		}
	})

	d.dc.OnClose(func() {
		d.mu.RLock()
		handler := d.onClose
		d.mu.RUnlock()

		if handler != nil {
			handler()
		}
	})

	d.dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		atomic.AddInt64(&d.messagesRecv, 1)
		atomic.AddInt64(&d.bytesRecv, int64(len(msg.Data)))

		d.mu.RLock()
		handler := d.onMessage
		d.mu.RUnlock()

		if handler != nil {
			handler(msg.Data)
		}
	})

	d.dc.OnError(func(err error) {
		d.mu.RLock()
		handler := d.onError
		d.mu.RUnlock()

		if handler != nil {
			handler(err)
		}
	})
}

type DataChannelStats struct {
	Label        string `json:"label"`
	State        string `json:"state"`
	MessagesSent int64  `json:"messages_sent"`
	MessagesRecv int64  `json:"messages_recv"`
	BytesSent    int64  `json:"bytes_sent"`
	BytesRecv    int64  `json:"bytes_recv"`
}
