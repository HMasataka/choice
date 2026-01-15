package rtp

import (
	"fmt"
	"sync"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProcessor(t *testing.T) {
	proc := NewProcessor()
	assert.NotNil(t, proc)
}

func TestProcessor_ProcessIncoming(t *testing.T) {
	proc := NewProcessor()

	// Test with valid packet
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

	err := proc.ProcessIncoming(packet)
	assert.NoError(t, err)

	// Test with nil packet
	err = proc.ProcessIncoming(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "packet cannot be nil")
}

func TestProcessor_ForwardToSubscriber_SSRCRewriting(t *testing.T) {
	proc := NewProcessor()

	publisherSSRC := uint32(12345)
	subscriberID := "sub1"

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 100,
			Timestamp:      1000,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	// Forward to subscriber
	outPacket, err := proc.ForwardToSubscriber(subscriberID, packet)
	require.NoError(t, err)
	assert.NotNil(t, outPacket)

	// SSRC should be rewritten
	assert.NotEqual(t, publisherSSRC, outPacket.SSRC)
	assert.Equal(t, uint32(1000), outPacket.SSRC) // First allocated SSRC

	// Forward same publisher SSRC again - should get same subscriber SSRC
	packet2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 101,
			Timestamp:      1100,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{5, 6, 7, 8},
	}

	outPacket2, err := proc.ForwardToSubscriber(subscriberID, packet2)
	require.NoError(t, err)
	assert.Equal(t, outPacket.SSRC, outPacket2.SSRC)
}

func TestProcessor_ForwardToSubscriber_SequenceNumberNormalization(t *testing.T) {
	proc := NewProcessor()

	subscriberID := "sub1"
	publisherSSRC := uint32(12345)

	// First packet: sequence 1000
	packet1 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1000,
			Timestamp:      10000,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	outPacket1, err := proc.ForwardToSubscriber(subscriberID, packet1)
	require.NoError(t, err)
	assert.Equal(t, uint16(0), outPacket1.SequenceNumber) // Starts from 0

	// Second packet: sequence 1001
	packet2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1001,
			Timestamp:      10100,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{5, 6, 7, 8},
	}

	outPacket2, err := proc.ForwardToSubscriber(subscriberID, packet2)
	require.NoError(t, err)
	assert.Equal(t, uint16(1), outPacket2.SequenceNumber) // Incremented

	// Third packet: sequence 1005 (gap)
	packet3 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1005,
			Timestamp:      10500,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{9, 10, 11, 12},
	}

	outPacket3, err := proc.ForwardToSubscriber(subscriberID, packet3)
	require.NoError(t, err)
	assert.Equal(t, uint16(5), outPacket3.SequenceNumber) // Gap maintained
}

func TestProcessor_ForwardToSubscriber_SequenceNumberWraparound(t *testing.T) {
	proc := NewProcessor()

	subscriberID := "sub1"
	publisherSSRC := uint32(12345)

	// First packet: sequence 65530 (near wraparound)
	packet1 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 65530,
			Timestamp:      10000,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	outPacket1, err := proc.ForwardToSubscriber(subscriberID, packet1)
	require.NoError(t, err)
	assert.Equal(t, uint16(0), outPacket1.SequenceNumber)

	// Second packet: sequence 65535 (max uint16)
	packet2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 65535,
			Timestamp:      10100,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{5, 6, 7, 8},
	}

	outPacket2, err := proc.ForwardToSubscriber(subscriberID, packet2)
	require.NoError(t, err)
	assert.Equal(t, uint16(5), outPacket2.SequenceNumber)

	// Third packet: sequence 0 (wrapped around)
	packet3 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 0,
			Timestamp:      10200,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{9, 10, 11, 12},
	}

	outPacket3, err := proc.ForwardToSubscriber(subscriberID, packet3)
	require.NoError(t, err)
	assert.Equal(t, uint16(6), outPacket3.SequenceNumber) // Continues sequence
}

func TestProcessor_ForwardToSubscriber_TimestampNormalization(t *testing.T) {
	proc := NewProcessor()

	subscriberID := "sub1"
	publisherSSRC := uint32(12345)

	// First packet: timestamp 50000
	packet1 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1000,
			Timestamp:      50000,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	outPacket1, err := proc.ForwardToSubscriber(subscriberID, packet1)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), outPacket1.Timestamp) // Starts from 0

	// Second packet: timestamp 50100
	packet2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1001,
			Timestamp:      50100,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{5, 6, 7, 8},
	}

	outPacket2, err := proc.ForwardToSubscriber(subscriberID, packet2)
	require.NoError(t, err)
	assert.Equal(t, uint32(100), outPacket2.Timestamp) // Offset maintained

	// Third packet: timestamp 50300 (gap)
	packet3 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1002,
			Timestamp:      50300,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{9, 10, 11, 12},
	}

	outPacket3, err := proc.ForwardToSubscriber(subscriberID, packet3)
	require.NoError(t, err)
	assert.Equal(t, uint32(300), outPacket3.Timestamp) // Gap maintained
}

func TestProcessor_ForwardToSubscriber_MultipleSubscribers(t *testing.T) {
	proc := NewProcessor()

	publisherSSRC := uint32(12345)
	sub1ID := "sub1"
	sub2ID := "sub2"

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1000,
			Timestamp:      10000,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	// Forward to subscriber 1
	outPacket1, err := proc.ForwardToSubscriber(sub1ID, packet)
	require.NoError(t, err)

	// Forward to subscriber 2
	outPacket2, err := proc.ForwardToSubscriber(sub2ID, packet)
	require.NoError(t, err)

	// Each subscriber should get different SSRC
	assert.NotEqual(t, outPacket1.SSRC, outPacket2.SSRC)

	// But both should start from sequence 0
	assert.Equal(t, uint16(0), outPacket1.SequenceNumber)
	assert.Equal(t, uint16(0), outPacket2.SequenceNumber)

	// And both should start from timestamp 0
	assert.Equal(t, uint32(0), outPacket1.Timestamp)
	assert.Equal(t, uint32(0), outPacket2.Timestamp)
}

func TestProcessor_ForwardToSubscriber_PayloadCopy(t *testing.T) {
	proc := NewProcessor()

	subscriberID := "sub1"
	originalPayload := []byte{1, 2, 3, 4, 5}

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1000,
			Timestamp:      10000,
			SSRC:           12345,
		},
		Payload: originalPayload,
	}

	outPacket, err := proc.ForwardToSubscriber(subscriberID, packet)
	require.NoError(t, err)

	// Payload should be copied
	assert.Equal(t, originalPayload, outPacket.Payload)

	// Modifying original should not affect output
	originalPayload[0] = 99
	assert.Equal(t, byte(1), outPacket.Payload[0])
}

func TestProcessor_ForwardToSubscriber_ExtensionPreserved(t *testing.T) {
	proc := NewProcessor()

	subscriberID := "sub1"

	// Create packet with extension using SetExtension
	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1000,
			Timestamp:      10000,
			SSRC:           12345,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	// Set an extension (e.g., audio level)
	err := packet.SetExtension(1, []byte{10, 20, 30, 40})
	require.NoError(t, err)

	outPacket, err := proc.ForwardToSubscriber(subscriberID, packet)
	require.NoError(t, err)

	// Extension should be preserved
	assert.True(t, outPacket.Extension)
	ext := outPacket.GetExtension(1)
	assert.NotNil(t, ext)
	assert.Equal(t, []byte{10, 20, 30, 40}, ext)
}

func TestProcessor_ForwardToSubscriber_EmptySubscriberID(t *testing.T) {
	proc := NewProcessor()

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1000,
			Timestamp:      10000,
			SSRC:           12345,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	outPacket, err := proc.ForwardToSubscriber("", packet)
	assert.Error(t, err)
	assert.Nil(t, outPacket)
	assert.Contains(t, err.Error(), "subscriber ID cannot be empty")
}

func TestProcessor_ForwardToSubscriber_NilPacket(t *testing.T) {
	proc := NewProcessor()

	outPacket, err := proc.ForwardToSubscriber("sub1", nil)
	assert.Error(t, err)
	assert.Nil(t, outPacket)
	assert.Contains(t, err.Error(), "packet cannot be nil")
}

func TestProcessor_GetSSRCMapping(t *testing.T) {
	proc := NewProcessor()

	publisherSSRC1 := uint32(12345)
	publisherSSRC2 := uint32(67890)
	subscriberID := "sub1"

	// Forward packets from two different publisher SSRCs
	packet1 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1000,
			Timestamp:      10000,
			SSRC:           publisherSSRC1,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	packet2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 2000,
			Timestamp:      20000,
			SSRC:           publisherSSRC2,
		},
		Payload: []byte{5, 6, 7, 8},
	}

	outPacket1, err := proc.ForwardToSubscriber(subscriberID, packet1)
	require.NoError(t, err)

	outPacket2, err := proc.ForwardToSubscriber(subscriberID, packet2)
	require.NoError(t, err)

	// Get SSRC mapping
	mapping := proc.GetSSRCMapping(subscriberID)
	assert.NotNil(t, mapping)
	assert.Len(t, mapping, 2)
	assert.Equal(t, outPacket1.SSRC, mapping[publisherSSRC1])
	assert.Equal(t, outPacket2.SSRC, mapping[publisherSSRC2])
}

func TestProcessor_GetSSRCMapping_EmptySubscriberID(t *testing.T) {
	proc := NewProcessor()

	mapping := proc.GetSSRCMapping("")
	assert.Nil(t, mapping)
}

func TestProcessor_GetSSRCMapping_UnknownSubscriber(t *testing.T) {
	proc := NewProcessor()

	mapping := proc.GetSSRCMapping("unknown")
	assert.NotNil(t, mapping)
	assert.Empty(t, mapping)
}

func TestProcessor_GetSSRCMapping_Copy(t *testing.T) {
	proc := NewProcessor()

	subscriberID := "sub1"
	publisherSSRC := uint32(12345)

	packet := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1000,
			Timestamp:      10000,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	_, err := proc.ForwardToSubscriber(subscriberID, packet)
	require.NoError(t, err)

	// Get mapping
	mapping1 := proc.GetSSRCMapping(subscriberID)
	mapping2 := proc.GetSSRCMapping(subscriberID)

	// Should be separate copies
	assert.Equal(t, mapping1, mapping2)

	// Modifying one should not affect the other
	mapping1[999] = 888
	assert.NotContains(t, mapping2, uint32(999))
}

func TestProcessor_ForwardToSubscriber_OutOfOrderDrop(t *testing.T) {
	proc := NewProcessor()

	subscriberID := "sub1"
	publisherSSRC := uint32(12345)

	// First packet: sequence 1000
	packet1 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1000,
			Timestamp:      10000,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{1, 2, 3, 4},
	}

	outPacket1, err := proc.ForwardToSubscriber(subscriberID, packet1)
	require.NoError(t, err)
	assert.NotNil(t, outPacket1)

	// Second packet: sequence 1005 (gap, but valid)
	packet2 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1005,
			Timestamp:      10500,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{5, 6, 7, 8},
	}

	outPacket2, err := proc.ForwardToSubscriber(subscriberID, packet2)
	require.NoError(t, err)
	assert.NotNil(t, outPacket2)

	// Third packet: sequence 1003 (out-of-order, should be dropped)
	packet3 := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1003,
			Timestamp:      10300,
			SSRC:           publisherSSRC,
		},
		Payload: []byte{9, 10, 11, 12},
	}

	outPacket3, err := proc.ForwardToSubscriber(subscriberID, packet3)
	assert.Error(t, err)
	assert.Nil(t, outPacket3)
	assert.Contains(t, err.Error(), "dropping out-of-order")
}

func TestProcessor_ConcurrentAccess(t *testing.T) {
	proc := NewProcessor()

	var wg sync.WaitGroup
	numGoroutines := 10
	numPacketsPerGoroutine := 100

	publisherSSRC := uint32(12345)

	// Channel to collect errors from goroutines
	errChan := make(chan error, numGoroutines*numPacketsPerGoroutine)

	// Concurrent forwards to multiple subscribers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(subscriberIndex int) {
			defer wg.Done()

			subscriberID := fmt.Sprintf("sub%d", subscriberIndex)

			for j := 0; j < numPacketsPerGoroutine; j++ {
				packet := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    96,
						SequenceNumber: uint16(1000 + j),
						Timestamp:      uint32(10000 + j*100),
						SSRC:           publisherSSRC,
					},
					Payload: []byte{byte(j)},
				}

				_, err := proc.ForwardToSubscriber(subscriberID, packet)
				if err != nil {
					errChan <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		t.Errorf("ForwardToSubscriber error: %v", err)
	}

	// Verify all subscribers got unique SSRCs
	ssrcSet := make(map[uint32]bool)
	for i := 0; i < numGoroutines; i++ {
		subscriberID := fmt.Sprintf("sub%d", i)
		mapping := proc.GetSSRCMapping(subscriberID)
		assert.Len(t, mapping, 1) // Each subscriber should have mapping for one publisher SSRC

		subscriberSSRC := mapping[publisherSSRC]
		assert.False(t, ssrcSet[subscriberSSRC], "Duplicate SSRC found")
		ssrcSet[subscriberSSRC] = true
	}
}
