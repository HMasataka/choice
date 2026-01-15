package rtcp

import (
	"context"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// NACKConfig contains configuration for NACK handling.
type NACKConfig struct {
	// MaxBufferSize is the maximum number of packets to buffer for retransmission.
	MaxBufferSize int

	// MaxRetries is the maximum number of retransmission attempts per packet.
	MaxRetries int

	// RetransmitTimeout is the time to wait before considering a retransmission failed.
	// Per tasks.md: Retransmission request within 10ms of packet loss detection.
	RetransmitTimeout time.Duration

	// RTXEnabled enables RTX (retransmission) support.
	RTXEnabled bool

	// RTXPayloadType is the payload type for RTX packets.
	RTXPayloadType uint8

	// RTXSSRCOffset is the SSRC offset for RTX streams.
	RTXSSRCOffset uint32
}

// DefaultNACKConfig returns the default NACK configuration.
func DefaultNACKConfig() *NACKConfig {
	return &NACKConfig{
		MaxBufferSize:     500,
		MaxRetries:        3,
		RetransmitTimeout: 10 * time.Millisecond, // Per tasks.md: 10ms
		RTXEnabled:        true,
		RTXPayloadType:    96, // Common RTX payload type
		RTXSSRCOffset:     1,
	}
}

// BufferedPacket contains a buffered RTP packet for potential retransmission.
type BufferedPacket struct {
	// Packet is the RTP packet.
	Packet *rtp.Packet

	// Raw is the raw packet bytes.
	Raw []byte

	// Timestamp is when the packet was buffered.
	Timestamp time.Time

	// RetransmitCount is the number of times this packet has been retransmitted.
	RetransmitCount int

	// LastRetransmit is the time of the last retransmission.
	LastRetransmit time.Time
}

// NACKHandler handles Generic NACK processing.
// Per tasks.md 3.2.4: NACK processing with RTX support.
type NACKHandler struct {
	mu     sync.RWMutex
	config *NACKConfig

	// packetBuffer stores packets for potential retransmission.
	// Key is SSRC, value is map of sequence number to packet.
	packetBuffer map[uint32]map[uint16]*BufferedPacket

	// pendingNACKs stores pending retransmission requests.
	pendingNACKs map[uint32]map[uint16]*pendingNACK

	// stats contains NACK statistics.
	stats NACKStats

	// onRetransmit is called when a packet needs to be retransmitted.
	onRetransmit func(ssrc uint32, packet *rtp.Packet, raw []byte)

	// ctx is the context for cancellation.
	ctx    context.Context
	cancel context.CancelFunc

	// running indicates if the handler is running.
	running bool
}

// pendingNACK tracks a pending retransmission request.
type pendingNACK struct {
	ssrc        uint32
	sequenceNum uint16
	requestTime time.Time
	retryCount  int
}

// NACKStats contains NACK statistics.
type NACKStats struct {
	// TotalNACKsReceived is the total number of NACK packets received.
	TotalNACKsReceived uint64

	// TotalPacketsRequested is the total number of packets requested via NACK.
	TotalPacketsRequested uint64

	// TotalPacketsRetransmitted is the total number of packets retransmitted.
	TotalPacketsRetransmitted uint64

	// TotalPacketsMissing is the total number of packets that couldn't be retransmitted.
	TotalPacketsMissing uint64

	// CurrentBufferSize is the current number of packets in the buffer.
	CurrentBufferSize int
}

// NewNACKHandler creates a new NACK handler.
func NewNACKHandler(cfg *NACKConfig) *NACKHandler {
	if cfg == nil {
		cfg = DefaultNACKConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &NACKHandler{
		config:       cfg,
		packetBuffer: make(map[uint32]map[uint16]*BufferedPacket),
		pendingNACKs: make(map[uint32]map[uint16]*pendingNACK),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the NACK handler's background goroutine.
func (n *NACKHandler) Start() {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return
	}
	n.running = true
	// Recreate context to support restart after Stop
	n.ctx, n.cancel = context.WithCancel(context.Background())
	n.mu.Unlock()

	go n.cleanupLoop()
}

// Stop stops the NACK handler.
func (n *NACKHandler) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.running {
		return
	}
	n.running = false
	n.cancel()
}

// cleanupLoop periodically cleans up old packets from the buffer.
func (n *NACKHandler) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			n.cleanupOldPackets()
		}
	}
}

// BufferPacket adds a packet to the buffer for potential retransmission.
// The packet and raw bytes are deep copied to prevent data races if caller reuses buffers.
func (n *NACKHandler) BufferPacket(ssrc uint32, packet *rtp.Packet, raw []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.packetBuffer[ssrc] == nil {
		n.packetBuffer[ssrc] = make(map[uint16]*BufferedPacket)
	}

	// Deep copy packet to prevent data races
	packetCopy := &rtp.Packet{
		Header: rtp.Header{
			Version:          packet.Version,
			Padding:          packet.Padding,
			Extension:        packet.Extension,
			Marker:           packet.Marker,
			PayloadType:      packet.PayloadType,
			SequenceNumber:   packet.SequenceNumber,
			Timestamp:        packet.Timestamp,
			SSRC:             packet.SSRC,
			ExtensionProfile: packet.ExtensionProfile,
			PaddingSize:      packet.Header.PaddingSize,
		},
	}
	// Copy CSRC slice
	if len(packet.CSRC) > 0 {
		packetCopy.CSRC = make([]uint32, len(packet.CSRC))
		copy(packetCopy.CSRC, packet.CSRC)
	}
	// Copy header extensions (important for TWCC/abs-send-time)
	// Use GetExtensionIDs and GetExtension methods since Extension fields are private
	if ids := packet.Header.GetExtensionIDs(); len(ids) > 0 {
		for _, id := range ids {
			if payload := packet.Header.GetExtension(id); len(payload) > 0 {
				payloadCopy := make([]byte, len(payload))
				copy(payloadCopy, payload)
				// Use SetExtensionWithProfile to preserve the original extension profile
				_ = packetCopy.Header.SetExtensionWithProfile(id, payloadCopy, packet.ExtensionProfile)
			}
		}
	}
	// Copy payload
	if len(packet.Payload) > 0 {
		packetCopy.Payload = make([]byte, len(packet.Payload))
		copy(packetCopy.Payload, packet.Payload)
	}

	// Deep copy raw bytes
	var rawCopy []byte
	if len(raw) > 0 {
		rawCopy = make([]byte, len(raw))
		copy(rawCopy, raw)
	}

	// Store packet
	n.packetBuffer[ssrc][packet.SequenceNumber] = &BufferedPacket{
		Packet:    packetCopy,
		Raw:       rawCopy,
		Timestamp: time.Now(),
	}

	// Enforce buffer size limit (remove oldest packets)
	n.enforceBufferLimit(ssrc)
}

// enforceBufferLimit removes oldest packets if buffer exceeds limit.
// Uses timestamp-based eviction to remove the oldest packets first.
func (n *NACKHandler) enforceBufferLimit(ssrc uint32) {
	buffer := n.packetBuffer[ssrc]
	if len(buffer) <= n.config.MaxBufferSize {
		return
	}

	// Find oldest packets to remove using timestamp-based eviction
	toRemove := len(buffer) - n.config.MaxBufferSize

	// Build a list of (seqNum, timestamp) pairs
	type entry struct {
		seqNum    uint16
		timestamp time.Time
	}
	entries := make([]entry, 0, len(buffer))
	for seqNum, pkt := range buffer {
		entries = append(entries, entry{seqNum: seqNum, timestamp: pkt.Timestamp})
	}

	// Sort by timestamp (oldest first)
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].timestamp.Before(entries[i].timestamp) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Remove oldest packets
	for i := 0; i < toRemove && i < len(entries); i++ {
		delete(buffer, entries[i].seqNum)
	}
}

// HandleNACK processes a NACK packet and triggers retransmissions.
func (n *NACKHandler) HandleNACK(nack *rtcp.TransportLayerNack) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.stats.TotalNACKsReceived++

	ssrc := nack.MediaSSRC

	// Process each NACK pair
	for _, pair := range nack.Nacks {
		// Get all sequence numbers from the NACK pair
		seqNums := pair.PacketList()
		n.stats.TotalPacketsRequested += uint64(len(seqNums))

		for _, seqNum := range seqNums {
			n.handleRetransmitRequest(ssrc, seqNum)
		}
	}
}

// handleRetransmitRequest handles a single retransmission request.
func (n *NACKHandler) handleRetransmitRequest(ssrc uint32, seqNum uint16) {
	buffer := n.packetBuffer[ssrc]
	if buffer == nil {
		n.stats.TotalPacketsMissing++
		return
	}

	bufferedPkt, ok := buffer[seqNum]
	if !ok {
		n.stats.TotalPacketsMissing++
		return
	}

	// Check retry limit
	if bufferedPkt.RetransmitCount >= n.config.MaxRetries {
		return
	}

	// Check timeout since last retransmit
	if time.Since(bufferedPkt.LastRetransmit) < n.config.RetransmitTimeout {
		return
	}

	// Update retransmit tracking
	bufferedPkt.RetransmitCount++
	bufferedPkt.LastRetransmit = time.Now()
	n.stats.TotalPacketsRetransmitted++

	// Trigger retransmission callback
	if n.onRetransmit != nil {
		n.onRetransmit(ssrc, bufferedPkt.Packet, bufferedPkt.Raw)
	}
}

// cleanupOldPackets removes packets that are too old from the buffer.
func (n *NACKHandler) cleanupOldPackets() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Keep packets for up to 2 seconds (enough for NACK latency)
	cutoff := time.Now().Add(-2 * time.Second)

	for ssrc, buffer := range n.packetBuffer {
		for seqNum, pkt := range buffer {
			if pkt.Timestamp.Before(cutoff) {
				delete(buffer, seqNum)
			}
		}
		// Remove empty SSRC entries
		if len(buffer) == 0 {
			delete(n.packetBuffer, ssrc)
		}
	}
}

// GetStats returns NACK statistics.
func (n *NACKHandler) GetStats() NACKStats {
	n.mu.RLock()
	defer n.mu.RUnlock()

	stats := n.stats
	// Calculate current buffer size
	for _, buffer := range n.packetBuffer {
		stats.CurrentBufferSize += len(buffer)
	}
	return stats
}

// SetOnRetransmit sets the callback for packet retransmission.
func (n *NACKHandler) SetOnRetransmit(cb func(ssrc uint32, packet *rtp.Packet, raw []byte)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onRetransmit = cb
}

// GenerateNACK generates a NACK packet for missing sequence numbers.
func (n *NACKHandler) GenerateNACK(senderSSRC, mediaSSRC uint32, missingSeqNums []uint16) *rtcp.TransportLayerNack {
	if len(missingSeqNums) == 0 {
		return nil
	}

	// Convert sequence numbers to NACK pairs
	nacks := sequenceNumbersToNACKPairs(missingSeqNums)

	return &rtcp.TransportLayerNack{
		SenderSSRC: senderSSRC,
		MediaSSRC:  mediaSSRC,
		Nacks:      nacks,
	}
}

// seqNumDistance calculates the distance between two sequence numbers,
// accounting for uint16 wraparound. Returns a signed distance where
// positive means b is ahead of a.
func seqNumDistance(a, b uint16) int16 {
	return int16(b - a)
}

// seqNumLess compares two sequence numbers accounting for wraparound.
// Returns true if a comes before b in sequence space.
func seqNumLess(a, b uint16) bool {
	return seqNumDistance(a, b) > 0
}

// sequenceNumbersToNACKPairs converts a list of sequence numbers to NACK pairs.
func sequenceNumbersToNACKPairs(seqNums []uint16) []rtcp.NackPair {
	if len(seqNums) == 0 {
		return nil
	}

	var pairs []rtcp.NackPair

	// Sort sequence numbers with wraparound awareness
	sorted := make([]uint16, len(seqNums))
	copy(sorted, seqNums)
	// Simple bubble sort for small lists, using wraparound-aware comparison
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if seqNumLess(sorted[j], sorted[i]) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Group into NACK pairs
	for i := 0; i < len(sorted); {
		base := sorted[i]
		var blp uint16 // Bitmask of following 16 sequence numbers

		// Find all sequence numbers within 16 of base (using wraparound-aware distance)
		for j := i + 1; j < len(sorted); j++ {
			diff := seqNumDistance(base, sorted[j])
			if diff > 0 && diff <= 16 {
				blp |= 1 << (diff - 1)
				i = j
			} else {
				break
			}
		}

		pairs = append(pairs, rtcp.NackPair{
			PacketID:    base,
			LostPackets: rtcp.PacketBitmap(blp),
		})
		i++
	}

	return pairs
}

// CreateRTXPacket creates an RTX packet for retransmission.
func (n *NACKHandler) CreateRTXPacket(original *rtp.Packet, rtxSSRC uint32, rtxPayloadType uint8, rtxSequenceNumber uint16) *rtp.Packet {
	// RTX packet format:
	// - Same RTP header but with RTX SSRC and payload type
	// - Payload: Original sequence number (2 bytes) + Original payload

	rtxPayload := make([]byte, 2+len(original.Payload))
	rtxPayload[0] = byte(original.SequenceNumber >> 8)
	rtxPayload[1] = byte(original.SequenceNumber)
	copy(rtxPayload[2:], original.Payload)

	return &rtp.Packet{
		Header: rtp.Header{
			Version:        original.Version,
			Padding:        original.Padding,
			Extension:      original.Extension,
			Marker:         false, // RTX packets typically don't have marker
			PayloadType:    rtxPayloadType,
			SequenceNumber: rtxSequenceNumber,
			Timestamp:      original.Timestamp,
			SSRC:           rtxSSRC,
			CSRC:           original.CSRC,
		},
		Payload: rtxPayload,
	}
}

// ExtractOriginalFromRTX extracts the original packet from an RTX packet.
func (n *NACKHandler) ExtractOriginalFromRTX(rtxPacket *rtp.Packet, originalSSRC uint32, originalPayloadType uint8) *rtp.Packet {
	if len(rtxPacket.Payload) < 2 {
		return nil
	}

	// Extract original sequence number
	originalSeqNum := uint16(rtxPacket.Payload[0])<<8 | uint16(rtxPacket.Payload[1])

	// Extract original payload
	originalPayload := rtxPacket.Payload[2:]

	return &rtp.Packet{
		Header: rtp.Header{
			Version:        rtxPacket.Version,
			Padding:        rtxPacket.Padding,
			Extension:      rtxPacket.Extension,
			Marker:         rtxPacket.Marker,
			PayloadType:    originalPayloadType,
			SequenceNumber: originalSeqNum,
			Timestamp:      rtxPacket.Timestamp,
			SSRC:           originalSSRC,
			CSRC:           rtxPacket.CSRC,
		},
		Payload: originalPayload,
	}
}

// GetBufferedPacket retrieves a buffered packet by SSRC and sequence number.
func (n *NACKHandler) GetBufferedPacket(ssrc uint32, seqNum uint16) *BufferedPacket {
	n.mu.RLock()
	defer n.mu.RUnlock()

	buffer := n.packetBuffer[ssrc]
	if buffer == nil {
		return nil
	}

	if pkt, ok := buffer[seqNum]; ok {
		// Return a copy
		return &BufferedPacket{
			Packet:          pkt.Packet,
			Raw:             pkt.Raw,
			Timestamp:       pkt.Timestamp,
			RetransmitCount: pkt.RetransmitCount,
			LastRetransmit:  pkt.LastRetransmit,
		}
	}

	return nil
}

// IsRTXEnabled returns whether RTX is enabled.
func (n *NACKHandler) IsRTXEnabled() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config.RTXEnabled
}

// GetRTXPayloadType returns the RTX payload type.
func (n *NACKHandler) GetRTXPayloadType() uint8 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config.RTXPayloadType
}
