package rtp

import (
	"sync"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExtensionProcessor(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)
	assert.NotNil(t, ep)
}

func TestExtensionProcessor_ExtractExtensions(t *testing.T) {
	midExtID := uint8(1)
	ridExtID := uint8(2)
	ep := NewExtensionProcessor(midExtID, ridExtID)

	// Test with packet containing both MID and RID
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

	// Set MID extension
	err := packet.SetExtension(midExtID, []byte("video-main"))
	require.NoError(t, err)

	// Set RID extension
	err = packet.SetExtension(ridExtID, []byte("h"))
	require.NoError(t, err)

	// Extract extensions
	info, err := ep.ExtractExtensions(packet)
	require.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, "video-main", info.MID)
	assert.Equal(t, "h", info.RID)
	assert.True(t, info.HasMID)
	assert.True(t, info.HasRID)
}

func TestExtensionProcessor_ExtractExtensions_NoExtensions(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

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

	info, err := ep.ExtractExtensions(packet)
	require.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, "", info.MID)
	assert.Equal(t, "", info.RID)
	assert.False(t, info.HasMID)
	assert.False(t, info.HasRID)
}

func TestExtensionProcessor_ExtractExtensions_OnlyMID(t *testing.T) {
	midExtID := uint8(1)
	ep := NewExtensionProcessor(midExtID, 0) // RID not configured

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

	err := packet.SetExtension(midExtID, []byte("audio-track"))
	require.NoError(t, err)

	info, err := ep.ExtractExtensions(packet)
	require.NoError(t, err)
	assert.Equal(t, "audio-track", info.MID)
	assert.True(t, info.HasMID)
	assert.False(t, info.HasRID)
}

func TestExtensionProcessor_ExtractExtensions_NilPacket(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	info, err := ep.ExtractExtensions(nil)
	assert.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "packet cannot be nil")
}

func TestExtensionProcessor_UpdateCache(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	info := &ExtensionInfo{
		MID:    "video-main",
		RID:    "h",
		HasMID: true,
		HasRID: true,
	}

	ep.UpdateCache("publisher1", 12345, info)

	// Retrieve from cache
	retrieved, exists := ep.GetExtensionInfo("publisher1", 12345)
	assert.True(t, exists)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "video-main", retrieved.MID)
	assert.Equal(t, "h", retrieved.RID)
	assert.True(t, retrieved.HasMID)
	assert.True(t, retrieved.HasRID)
}

func TestExtensionProcessor_UpdateCache_EmptyPublisherID(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	info := &ExtensionInfo{
		MID:    "test",
		HasMID: true,
	}

	// Should not panic with empty publisher ID
	ep.UpdateCache("", 12345, info)

	retrieved, exists := ep.GetExtensionInfo("", 12345)
	assert.False(t, exists)
	assert.Nil(t, retrieved)
}

func TestExtensionProcessor_UpdateCache_NilInfo(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	// Should not panic with nil info
	ep.UpdateCache("publisher1", 12345, nil)

	retrieved, exists := ep.GetExtensionInfo("publisher1", 12345)
	assert.False(t, exists)
	assert.Nil(t, retrieved)
}

func TestExtensionProcessor_GetExtensionInfo_NotFound(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	// No cache entries
	retrieved, exists := ep.GetExtensionInfo("unknown", 99999)
	assert.False(t, exists)
	assert.Nil(t, retrieved)
}

func TestExtensionProcessor_GetExtensionInfo_PublisherNotFound(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	info := &ExtensionInfo{
		MID:    "test",
		HasMID: true,
	}
	ep.UpdateCache("publisher1", 12345, info)

	// Different publisher
	retrieved, exists := ep.GetExtensionInfo("publisher2", 12345)
	assert.False(t, exists)
	assert.Nil(t, retrieved)
}

func TestExtensionProcessor_GetExtensionInfo_SSRCNotFound(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	info := &ExtensionInfo{
		MID:    "test",
		HasMID: true,
	}
	ep.UpdateCache("publisher1", 12345, info)

	// Different SSRC
	retrieved, exists := ep.GetExtensionInfo("publisher1", 99999)
	assert.False(t, exists)
	assert.Nil(t, retrieved)
}

func TestExtensionProcessor_GetExtensionInfo_ReturnsCopy(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	info := &ExtensionInfo{
		MID:    "original",
		HasMID: true,
	}
	ep.UpdateCache("publisher1", 12345, info)

	// Get cached info
	retrieved, exists := ep.GetExtensionInfo("publisher1", 12345)
	require.True(t, exists)
	require.NotNil(t, retrieved)

	// Modify retrieved copy
	retrieved.MID = "modified"

	// Original should remain unchanged
	retrieved2, exists := ep.GetExtensionInfo("publisher1", 12345)
	require.True(t, exists)
	assert.Equal(t, "original", retrieved2.MID)
}

func TestExtensionProcessor_ConcurrentAccess(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	// Concurrent updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			publisherID := "publisher1"
			for j := 0; j < numOperations; j++ {
				ssrc := uint32(index*1000 + j)
				info := &ExtensionInfo{
					MID:    "video-main",
					RID:    "h",
					HasMID: true,
					HasRID: true,
				}
				ep.UpdateCache(publisherID, ssrc, info)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			publisherID := "publisher1"
			for j := 0; j < numOperations; j++ {
				ssrc := uint32(index*1000 + j)
				ep.GetExtensionInfo(publisherID, ssrc)
			}
		}(i)
	}

	wg.Wait()

	// Verify some entries were cached
	retrieved, exists := ep.GetExtensionInfo("publisher1", 1050)
	assert.True(t, exists)
	assert.NotNil(t, retrieved)
}

func TestExtensionProcessor_MultiplePublishers(t *testing.T) {
	ep := NewExtensionProcessor(1, 2)

	info1 := &ExtensionInfo{
		MID:    "video-publisher1",
		RID:    "h",
		HasMID: true,
		HasRID: true,
	}
	ep.UpdateCache("publisher1", 12345, info1)

	info2 := &ExtensionInfo{
		MID:    "audio-publisher2",
		HasMID: true,
		HasRID: false,
	}
	ep.UpdateCache("publisher2", 67890, info2)

	// Retrieve publisher1's info
	retrieved1, exists := ep.GetExtensionInfo("publisher1", 12345)
	assert.True(t, exists)
	assert.Equal(t, "video-publisher1", retrieved1.MID)
	assert.Equal(t, "h", retrieved1.RID)

	// Retrieve publisher2's info
	retrieved2, exists := ep.GetExtensionInfo("publisher2", 67890)
	assert.True(t, exists)
	assert.Equal(t, "audio-publisher2", retrieved2.MID)
	assert.False(t, retrieved2.HasRID)
}
