package rtp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// JitterBuffer buffers RTP packets to smooth out network jitter.
// Per design.md: Jitter buffer provides 50ms adaptive buffering.
// Phase 1 implementation: Fixed 50ms delay (non-adaptive).
//
// Note: Phase 1 uses FIFO (insertion order) rather than sequence number ordering.
// This is acceptable for Phase 1 basic functionality. Sequence-based reordering
// will be implemented in Phase 3 as part of quality optimization.
type JitterBuffer interface {
	// Push adds a packet to the buffer with its receive time.
	Push(packet *rtp.Packet, receivedAt time.Time) error

	// Pop retrieves the next packet that has been buffered long enough.
	// Phase 1: Returns packet after fixed 50ms delay.
	// Returns (nil, nil) if no packets are ready.
	Pop(ctx context.Context) (*rtp.Packet, error)

	// Flush clears all buffered packets.
	Flush() error
}

// bufferedPacket represents a packet with its metadata.
type bufferedPacket struct {
	packet     *rtp.Packet
	receivedAt time.Time
	readyAt    time.Time // receivedAt + buffer delay
}

// fixedJitterBuffer implements a fixed-delay jitter buffer for Phase 1.
type fixedJitterBuffer struct {
	mu    sync.Mutex
	queue []*bufferedPacket
	delay time.Duration // Fixed delay (50ms for Phase 1)
}

// NewJitterBuffer creates a new JitterBuffer.
// Phase 1: Returns a fixed 50ms delay implementation.
func NewJitterBuffer() JitterBuffer {
	return &fixedJitterBuffer{
		queue: make([]*bufferedPacket, 0),
		delay: 50 * time.Millisecond, // Fixed 50ms delay
	}
}

// Push adds a packet to the buffer.
//
// Ownership: The caller must not modify the packet after calling Push.
// Ownership of the packet is transferred to the JitterBuffer.
func (jb *fixedJitterBuffer) Push(packet *rtp.Packet, receivedAt time.Time) error {
	if packet == nil {
		return fmt.Errorf("packet cannot be nil")
	}

	jb.mu.Lock()
	defer jb.mu.Unlock()

	bp := &bufferedPacket{
		packet:     packet,
		receivedAt: receivedAt,
		readyAt:    receivedAt.Add(jb.delay),
	}

	jb.queue = append(jb.queue, bp)
	return nil
}

// Pop retrieves the next packet that is ready to be delivered.
// Returns (nil, nil) if no packets are ready yet.
// Returns error if context is cancelled.
func (jb *fixedJitterBuffer) Pop(ctx context.Context) (*rtp.Packet, error) {
	// Check context first
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	jb.mu.Lock()
	defer jb.mu.Unlock()

	if len(jb.queue) == 0 {
		return nil, nil
	}

	now := time.Now()

	// Find the first packet that is ready
	for i, bp := range jb.queue {
		if !now.Before(bp.readyAt) {
			// Packet is ready, remove from queue
			packet := bp.packet
			jb.queue = append(jb.queue[:i], jb.queue[i+1:]...)
			return packet, nil
		}
	}

	// No packets ready yet
	return nil, nil
}

// Flush clears all buffered packets.
func (jb *fixedJitterBuffer) Flush() error {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	jb.queue = make([]*bufferedPacket, 0)
	return nil
}
