package rtp

import (
	"fmt"
	"sync"

	"github.com/pion/rtp"
)

// ExtensionProcessor handles RTP header extensions for MID and RID.
// Per design.md: MID identifies media streams, RID identifies simulcast layers.
type ExtensionProcessor interface {
	// ExtractExtensions extracts MID/RID from packet (pure extraction).
	// Returns ExtensionInfo with HasMID/HasRID flags indicating presence.
	ExtractExtensions(packet *rtp.Packet) (*ExtensionInfo, error)

	// UpdateCache caches extension info for a stream identified by publisher+SSRC.
	UpdateCache(publisherID string, ssrc uint32, info *ExtensionInfo)

	// GetExtensionInfo retrieves cached extension info by publisher+SSRC.
	// Returns (info, true) if found, (nil, false) if not cached.
	GetExtensionInfo(publisherID string, ssrc uint32) (*ExtensionInfo, bool)
}

// ExtensionInfo contains extracted RTP header extension information.
type ExtensionInfo struct {
	MID    string // Media ID (identifies media stream)
	RID    string // RTP Stream ID (identifies simulcast layer)
	HasMID bool   // True if MID extension was present
	HasRID bool   // True if RID extension was present
}

// extensionProcessor is the concrete implementation of ExtensionProcessor.
type extensionProcessor struct {
	mu sync.RWMutex

	// cache stores extension info: publisherID -> ssrc -> ExtensionInfo
	cache map[string]map[uint32]*ExtensionInfo

	// extensionIDs stores the RTP header extension IDs for MID and RID.
	// These are negotiated during SDP exchange and may vary per session.
	// ID 0 means the extension is not configured.
	midExtensionID uint8
	ridExtensionID uint8
}

// NewExtensionProcessor creates a new ExtensionProcessor.
// midExtID and ridExtID are the RTP header extension IDs negotiated during SDP exchange.
// Use 0 to indicate the extension is not configured.
func NewExtensionProcessor(midExtID, ridExtID uint8) ExtensionProcessor {
	return &extensionProcessor{
		cache:          make(map[string]map[uint32]*ExtensionInfo),
		midExtensionID: midExtID,
		ridExtensionID: ridExtID,
	}
}

// ExtractExtensions extracts MID/RID from packet.
func (ep *extensionProcessor) ExtractExtensions(packet *rtp.Packet) (*ExtensionInfo, error) {
	if packet == nil {
		return nil, fmt.Errorf("packet cannot be nil")
	}

	info := &ExtensionInfo{}

	// Extract MID if configured
	if ep.midExtensionID != 0 {
		if midData := packet.GetExtension(ep.midExtensionID); midData != nil {
			info.MID = string(midData)
			info.HasMID = true
		}
	}

	// Extract RID if configured
	if ep.ridExtensionID != 0 {
		if ridData := packet.GetExtension(ep.ridExtensionID); ridData != nil {
			info.RID = string(ridData)
			info.HasRID = true
		}
	}

	return info, nil
}

// UpdateCache caches extension info for a stream.
func (ep *extensionProcessor) UpdateCache(publisherID string, ssrc uint32, info *ExtensionInfo) {
	if publisherID == "" || info == nil {
		return
	}

	ep.mu.Lock()
	defer ep.mu.Unlock()

	if ep.cache[publisherID] == nil {
		ep.cache[publisherID] = make(map[uint32]*ExtensionInfo)
	}

	// Store a copy to prevent external modifications
	ep.cache[publisherID][ssrc] = &ExtensionInfo{
		MID:    info.MID,
		RID:    info.RID,
		HasMID: info.HasMID,
		HasRID: info.HasRID,
	}
}

// GetExtensionInfo retrieves cached extension info.
func (ep *extensionProcessor) GetExtensionInfo(publisherID string, ssrc uint32) (*ExtensionInfo, bool) {
	if publisherID == "" {
		return nil, false
	}

	ep.mu.RLock()
	defer ep.mu.RUnlock()

	publisherCache, exists := ep.cache[publisherID]
	if !exists {
		return nil, false
	}

	info, exists := publisherCache[ssrc]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent external modifications
	return &ExtensionInfo{
		MID:    info.MID,
		RID:    info.RID,
		HasMID: info.HasMID,
		HasRID: info.HasRID,
	}, true
}
