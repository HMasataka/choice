package rtp

import (
	"context"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJitterBuffer(t *testing.T) {
	jb := NewJitterBuffer()
	assert.NotNil(t, jb)
}

func TestJitterBuffer_PushPop(t *testing.T) {
	jb := NewJitterBuffer()
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

	receivedAt := time.Now()

	// Push packet
	err := jb.Push(packet, receivedAt)
	require.NoError(t, err)

	// Pop immediately (should return nil as packet not ready yet)
	retrieved, err := jb.Pop(ctx)
	assert.NoError(t, err)
	assert.Nil(t, retrieved)

	// Wait for buffer delay (50ms)
	time.Sleep(55 * time.Millisecond)

	// Pop again (should return packet now)
	retrieved, err = jb.Pop(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, packet.SequenceNumber, retrieved.SequenceNumber)
	assert.Equal(t, packet.Timestamp, retrieved.Timestamp)
	assert.Equal(t, packet.SSRC, retrieved.SSRC)
}

func TestJitterBuffer_Push_NilPacket(t *testing.T) {
	jb := NewJitterBuffer()

	err := jb.Push(nil, time.Now())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "packet cannot be nil")
}

func TestJitterBuffer_Pop_Empty(t *testing.T) {
	jb := NewJitterBuffer()
	ctx := context.Background()

	// Pop from empty buffer
	packet, err := jb.Pop(ctx)
	assert.NoError(t, err)
	assert.Nil(t, packet)
}

func TestJitterBuffer_Pop_ContextCancelled(t *testing.T) {
	jb := NewJitterBuffer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return context error
	packet, err := jb.Pop(ctx)
	assert.Error(t, err)
	assert.Nil(t, packet)
	assert.Equal(t, context.Canceled, err)
}

func TestJitterBuffer_MultiplePackets(t *testing.T) {
	jb := NewJitterBuffer()
	ctx := context.Background()

	numPackets := 5
	packets := make([]*rtp.Packet, numPackets)
	baseTime := time.Now()

	// Push multiple packets with staggered receive times
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
		receivedAt := baseTime.Add(time.Duration(i) * 10 * time.Millisecond)
		err := jb.Push(packets[i], receivedAt)
		require.NoError(t, err)
	}

	// Wait for first packet to be ready (50ms + margin)
	time.Sleep(60 * time.Millisecond)

	// Pop first packet
	retrieved, err := jb.Pop(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, uint16(100), retrieved.SequenceNumber)

	// Wait a bit more for other packets
	time.Sleep(50 * time.Millisecond)

	// Pop remaining packets
	for i := 1; i < numPackets; i++ {
		retrieved, err := jb.Pop(ctx)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, uint16(100+i), retrieved.SequenceNumber)
	}

	// Buffer should be empty now
	retrieved, err = jb.Pop(ctx)
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestJitterBuffer_Flush(t *testing.T) {
	jb := NewJitterBuffer()
	ctx := context.Background()

	// Push some packets
	for i := 0; i < 5; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(100 + i),
				Timestamp:      uint32(1000 + i*100),
				SSRC:           12345,
			},
			Payload: []byte{byte(i)},
		}
		err := jb.Push(packet, time.Now())
		require.NoError(t, err)
	}

	// Flush buffer
	err := jb.Flush()
	assert.NoError(t, err)

	// Wait for packets to be ready (if they weren't flushed)
	time.Sleep(60 * time.Millisecond)

	// Pop should return nil (buffer was flushed)
	retrieved, err := jb.Pop(ctx)
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestJitterBuffer_DelayAccuracy(t *testing.T) {
	jb := NewJitterBuffer()
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

	pushTime := time.Now()
	err := jb.Push(packet, pushTime)
	require.NoError(t, err)

	// Try to pop at various intervals
	// At 30ms: should not be ready (well before 50ms threshold)
	time.Sleep(30 * time.Millisecond)
	retrieved, err := jb.Pop(ctx)
	assert.NoError(t, err)
	assert.Nil(t, retrieved)

	// At 60ms: should be ready now (well after 50ms threshold)
	time.Sleep(30 * time.Millisecond)
	retrieved, err = jb.Pop(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
}

func TestJitterBuffer_OrderPreservation(t *testing.T) {
	jb := NewJitterBuffer()
	ctx := context.Background()

	baseTime := time.Now()

	// Push packets in order
	for i := 0; i < 3; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(100 + i),
				Timestamp:      uint32(1000 + i*100),
				SSRC:           12345,
			},
			Payload: []byte{byte(i)},
		}
		// All packets received at slightly different times
		receivedAt := baseTime.Add(time.Duration(i) * 5 * time.Millisecond)
		err := jb.Push(packet, receivedAt)
		require.NoError(t, err)
	}

	// Wait for all packets to be ready
	time.Sleep(70 * time.Millisecond)

	// Pop packets - should maintain insertion order (FIFO)
	for i := 0; i < 3; i++ {
		retrieved, err := jb.Pop(ctx)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, uint16(100+i), retrieved.SequenceNumber)
	}
}

func TestJitterBuffer_ConcurrentPushFlush(t *testing.T) {
	jb := NewJitterBuffer()

	done := make(chan bool)

	// Concurrent pushes
	go func() {
		for i := 0; i < 100; i++ {
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
			jb.Push(packet, time.Now())
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Concurrent flushes
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(10 * time.Millisecond)
			jb.Flush()
		}
		done <- true
	}()

	// Wait for completion
	<-done
	<-done

	// No panics is a success
}
