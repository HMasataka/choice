package rtp

import (
	"context"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPacer(t *testing.T) {
	pacer := NewPacer()
	assert.NotNil(t, pacer)
}

func TestPacer_EnqueueDequeue(t *testing.T) {
	pacer := NewPacer()
	ctx := context.Background()

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 100,
			Timestamp:      1000,
			SSRC:           12345,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	// Enqueue packet
	err := pacer.Enqueue(packet, time.Now())
	assert.NoError(t, err)

	// Dequeue packet (should be immediate in no-op implementation)
	retrieved, err := pacer.Dequeue(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, packet.SequenceNumber, retrieved.SequenceNumber)
	assert.Equal(t, packet.Timestamp, retrieved.Timestamp)
	assert.Equal(t, packet.SSRC, retrieved.SSRC)
}

func TestPacer_Enqueue_NilPacket(t *testing.T) {
	pacer := NewPacer()

	err := pacer.Enqueue(nil, time.Now())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "packet cannot be nil")
}

func TestPacer_Dequeue_Empty(t *testing.T) {
	pacer := NewPacer()
	ctx := context.Background()

	// Dequeue from empty pacer
	packet, err := pacer.Dequeue(ctx)
	assert.NoError(t, err)
	assert.Nil(t, packet)
}

func TestPacer_Dequeue_ContextCancelled(t *testing.T) {
	pacer := NewPacer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return context error
	packet, err := pacer.Dequeue(ctx)
	assert.Error(t, err)
	assert.Nil(t, packet)
	assert.Equal(t, context.Canceled, err)
}

func TestPacer_MultiplePackets(t *testing.T) {
	pacer := NewPacer()
	ctx := context.Background()

	numPackets := 10
	packets := make([]*rtp.Packet, numPackets)

	// Enqueue multiple packets
	for i := 0; i < numPackets; i++ {
		packets[i] = &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(100 + i),
				Timestamp:      uint32(1000 + i*100),
				SSRC:           12345,
			},
			Payload: []byte{byte(i)},
		}
		err := pacer.Enqueue(packets[i], time.Now())
		require.NoError(t, err)
	}

	// Dequeue all packets
	for i := 0; i < numPackets; i++ {
		retrieved, err := pacer.Dequeue(ctx)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, uint16(100+i), retrieved.SequenceNumber)
	}

	// Queue should be empty now
	retrieved, err := pacer.Dequeue(ctx)
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestPacer_QueueFull(t *testing.T) {
	pacer := NewPacer()

	// Fill the queue (capacity is 1000)
	for i := 0; i < 1000; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i * 100),
				SSRC:           12345,
			},
			Payload: []byte{byte(i)},
		}
		err := pacer.Enqueue(packet, time.Now())
		require.NoError(t, err)
	}

	// Try to enqueue one more (should fail)
	extraPacket := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 9999,
			Timestamp:      9999,
			SSRC:           12345,
		},
		Payload: []byte{99},
	}
	err := pacer.Enqueue(extraPacket, time.Now())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pacer queue full")
}

func TestPacer_IgnoresSendTime(t *testing.T) {
	pacer := NewPacer()
	ctx := context.Background()

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 100,
			Timestamp:      1000,
			SSRC:           12345,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	// Enqueue with future send time
	futureTime := time.Now().Add(10 * time.Second)
	err := pacer.Enqueue(packet, futureTime)
	require.NoError(t, err)

	// Should be immediately available (no-op implementation ignores sendAt)
	retrieved, err := pacer.Dequeue(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, packet.SequenceNumber, retrieved.SequenceNumber)
}
