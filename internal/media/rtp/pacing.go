package rtp

import (
	"context"
	"fmt"
	"time"

	"github.com/pion/rtp"
)

// Pacer controls the rate at which RTP packets are sent to avoid bursts.
// Per design.md: Packet pacing smooths burst transmission.
// Phase 1 implementation: no-op (immediate forwarding).
type Pacer interface {
	// Enqueue queues a packet with target send time.
	// Phase 1: Accepts packet but doesn't enforce pacing.
	Enqueue(packet *rtp.Packet, sendAt time.Time) error

	// Dequeue returns the next packet ready to send.
	// Phase 1: Returns immediately (no actual pacing).
	// Returns (nil, nil) if no packets are ready.
	Dequeue(ctx context.Context) (*rtp.Packet, error)
}

// noPacer is a no-op implementation of Pacer for Phase 1.
// Packets are immediately available for dequeue after enqueue.
type noPacer struct {
	queue chan *rtp.Packet
}

// NewPacer creates a new Pacer.
// Phase 1: Returns a no-op implementation.
func NewPacer() Pacer {
	return &noPacer{
		queue: make(chan *rtp.Packet, 1000), // Buffered channel for basic queuing
	}
}

// Enqueue adds a packet to the pacer queue.
// Phase 1: Ignores sendAt and immediately makes packet available.
//
// Ownership: The caller must not modify the packet after calling Enqueue.
// Ownership of the packet is transferred to the Pacer.
func (p *noPacer) Enqueue(packet *rtp.Packet, sendAt time.Time) error {
	if packet == nil {
		return fmt.Errorf("packet cannot be nil")
	}

	select {
	case p.queue <- packet:
		return nil
	default:
		return fmt.Errorf("pacer queue full")
	}
}

// Dequeue retrieves the next packet to send.
// Phase 1: Returns immediately without enforcing pacing.
// Returns (nil, nil) if no packets are available.
func (p *noPacer) Dequeue(ctx context.Context) (*rtp.Packet, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case packet := <-p.queue:
		return packet, nil
	default:
		// No packets ready
		return nil, nil
	}
}
