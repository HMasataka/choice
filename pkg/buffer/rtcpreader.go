package buffer

import (
	"io"
	"sync"
	"sync/atomic"
)

// RTCPReader は着信RTCPパケットの処理とコールバック通知を提供する
type RTCPReader struct {
	mu       sync.RWMutex
	ssrc     uint32
	closed   atomic.Bool
	onPacket func([]byte)
	onClose  func()
}

func NewRTCPReader(ssrc uint32) *RTCPReader {
	return &RTCPReader{
		ssrc: ssrc,
	}
}

func (r *RTCPReader) Write(p []byte) (n int, err error) {
	if r.closed.Load() {
		err = io.EOF
		return
	}

	r.mu.RLock()
	f := r.onPacket
	r.mu.RUnlock()

	if f != nil {
		f(p)
	}

	return
}

func (r *RTCPReader) OnClose(fn func()) {
	r.onClose = fn
}

func (r *RTCPReader) Close() error {
	r.closed.Store(true)
	r.onClose()
	return nil
}

func (r *RTCPReader) OnPacket(f func([]byte)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.onPacket = f
}

func (r *RTCPReader) Read(_ []byte) (n int, err error) {
	return
}
