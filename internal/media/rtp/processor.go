package rtp

import (
	"fmt"
	"sync"

	"github.com/pion/rtp"
)

// Processor handles RTP packet processing for SFU operations.
// Per design.md 3.7.1: SSRC management, sequence number rewriting, timestamp normalization.
type Processor interface {
	// ProcessIncoming processes incoming RTP packets from publisher.
	// Updates internal state for SSRC tracking.
	ProcessIncoming(packet *rtp.Packet) error

	// ForwardToSubscriber forwards RTP packet to subscriber with necessary transformations:
	// - SSRC rewriting (per subscriber)
	// - Sequence number normalization (continuity per subscriber)
	// - Timestamp normalization (continuity per subscriber)
	// Returns the transformed packet.
	ForwardToSubscriber(subscriberID string, packet *rtp.Packet) (*rtp.Packet, error)

	// GetSSRCMapping returns SSRC mapping for a subscriber.
	// Maps publisher SSRC to subscriber SSRC.
	GetSSRCMapping(subscriberID string) map[uint32]uint32
}

// processor is the concrete implementation of Processor.
type processor struct {
	mu sync.RWMutex

	// ssrcMap maps subscriberID -> publisherSSRC -> subscriberSSRC
	// Each subscriber gets unique SSRCs for each publisher stream
	ssrcMap map[string]map[uint32]uint32

	// seqState manages sequence number state per subscriber per SSRC
	// subscriberID -> ssrc -> seqState
	seqState map[string]map[uint32]*seqState

	// tsState manages timestamp state per subscriber per SSRC
	// subscriberID -> ssrc -> tsState
	tsState map[string]map[uint32]*tsState

	// nextSSRC is the next available SSRC to allocate
	nextSSRC uint32
}

// seqState tracks sequence number transformation state.
type seqState struct {
	baseIn  uint16 // First input sequence number seen
	baseOut uint16 // Output sequence number base (typically 0)
	lastIn  uint16 // Last input sequence number seen (for detecting out-of-order)
	lastOut uint16 // Last output sequence number sent
	inited  bool   // Whether we've seen the first packet
}

// tsState tracks timestamp transformation state.
type tsState struct {
	baseIn  uint32 // First input timestamp seen
	baseOut uint32 // Output timestamp base (typically 0)
	lastIn  uint32 // Last input timestamp seen (for detecting out-of-order)
	inited  bool   // Whether we've seen the first packet
}

// isSeqNewer returns true if a is newer than b in RTP sequence number space.
// Per RFC 3550: uses 16-bit wraparound logic where a difference less than
// half the range (0x8000) indicates "newer".
func isSeqNewer(a, b uint16) bool {
	diff := uint16(a - b)
	return diff > 0 && diff < 0x8000
}

// isTSNewer returns true if a is newer than b in RTP timestamp space.
// Uses 32-bit wraparound logic where a difference less than
// half the range (0x80000000) indicates "newer".
func isTSNewer(a, b uint32) bool {
	diff := uint32(a - b)
	return diff > 0 && diff < 0x80000000
}

// NewProcessor creates a new RTP processor.
func NewProcessor() Processor {
	return &processor{
		ssrcMap:  make(map[string]map[uint32]uint32),
		seqState: make(map[string]map[uint32]*seqState),
		tsState:  make(map[string]map[uint32]*tsState),
		nextSSRC: 1000, // Start from a reasonable SSRC value
	}
}

// ProcessIncoming processes incoming RTP packets from publisher.
// Updates internal state but does not modify the packet.
func (p *processor) ProcessIncoming(packet *rtp.Packet) error {
	if packet == nil {
		return fmt.Errorf("packet cannot be nil")
	}

	// For now, just validate. State is managed per-subscriber in ForwardToSubscriber.
	// Future: Could track publisher-side statistics here.

	return nil
}

// ForwardToSubscriber forwards RTP packet to subscriber with transformations.
func (p *processor) ForwardToSubscriber(subscriberID string, packet *rtp.Packet) (*rtp.Packet, error) {
	if subscriberID == "" {
		return nil, fmt.Errorf("subscriber ID cannot be empty")
	}
	if packet == nil {
		return nil, fmt.Errorf("packet cannot be nil")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	publisherSSRC := packet.SSRC

	// Get or allocate subscriber SSRC
	if p.ssrcMap[subscriberID] == nil {
		p.ssrcMap[subscriberID] = make(map[uint32]uint32)
	}
	subscriberSSRC, exists := p.ssrcMap[subscriberID][publisherSSRC]
	if !exists {
		// Allocate new SSRC for this subscriber
		subscriberSSRC = p.nextSSRC
		p.nextSSRC++
		p.ssrcMap[subscriberID][publisherSSRC] = subscriberSSRC
	}

	// Initialize state maps if needed
	if p.seqState[subscriberID] == nil {
		p.seqState[subscriberID] = make(map[uint32]*seqState)
	}
	if p.tsState[subscriberID] == nil {
		p.tsState[subscriberID] = make(map[uint32]*tsState)
	}

	// Get or create sequence state
	seq := p.seqState[subscriberID][publisherSSRC]
	if seq == nil {
		seq = &seqState{baseOut: 0}
		p.seqState[subscriberID][publisherSSRC] = seq
	}

	// Get or create timestamp state
	ts := p.tsState[subscriberID][publisherSSRC]
	if ts == nil {
		ts = &tsState{baseOut: 0}
		p.tsState[subscriberID][publisherSSRC] = ts
	}

	// Transform sequence number
	inSeq := packet.SequenceNumber
	var outSeq uint16
	if !seq.inited {
		// First packet: initialize base
		seq.baseIn = inSeq
		seq.lastIn = inSeq
		seq.baseOut = 0
		seq.lastOut = 0
		outSeq = 0
		seq.inited = true
	} else {
		// Check for out-of-order or duplicate packets using RFC 3550 logic
		if !isSeqNewer(inSeq, seq.lastIn) {
			// Out-of-order or duplicate packet, drop it
			return nil, fmt.Errorf("dropping out-of-order or duplicate packet: seq=%d, lastIn=%d", inSeq, seq.lastIn)
		}

		// Compute relative offset and apply to output base
		// Use unsigned wraparound arithmetic
		offset := uint32(inSeq) - uint32(seq.baseIn)
		outSeq = uint16((uint32(seq.baseOut) + offset) & 0xFFFF)
		seq.lastIn = inSeq
		seq.lastOut = outSeq
	}

	// Transform timestamp
	inTS := packet.Timestamp
	var outTS uint32
	if !ts.inited {
		// First packet: initialize base
		ts.baseIn = inTS
		ts.lastIn = inTS
		ts.baseOut = 0
		outTS = 0
		ts.inited = true
	} else {
		// Check for out-of-order or duplicate timestamps using RFC 3550 logic
		if !isTSNewer(inTS, ts.lastIn) {
			// Out-of-order or duplicate packet, drop it
			return nil, fmt.Errorf("dropping out-of-order or duplicate packet: ts=%d, lastIn=%d", inTS, ts.lastIn)
		}

		// Compute offset and apply to output base
		// Use unsigned wraparound arithmetic
		offset := uint64(inTS) - uint64(ts.baseIn)
		outTS = ts.baseOut + uint32(offset)
		ts.lastIn = inTS
	}

	// Create transformed packet by cloning and modifying
	// Clone the packet to preserve extensions and other fields
	outPacket := packet.Clone()

	// Override the fields that need transformation
	outPacket.SequenceNumber = outSeq
	outPacket.Timestamp = outTS
	outPacket.SSRC = subscriberSSRC

	return outPacket, nil
}

// GetSSRCMapping returns SSRC mapping for a subscriber.
// Returns a copy to prevent external modifications.
func (p *processor) GetSSRCMapping(subscriberID string) map[uint32]uint32 {
	if subscriberID == "" {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	mapping, exists := p.ssrcMap[subscriberID]
	if !exists {
		return make(map[uint32]uint32)
	}

	// Return a deep copy
	result := make(map[uint32]uint32, len(mapping))
	for k, v := range mapping {
		result[k] = v
	}
	return result
}
