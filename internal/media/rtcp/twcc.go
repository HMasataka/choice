package rtcp

import (
	"context"
	"sync"
	"time"

	"github.com/pion/rtcp"
)

// TWCCConfig contains configuration for TWCC handling.
type TWCCConfig struct {
	// UpdateInterval is the interval for bandwidth estimation updates.
	// Per tasks.md: 100ms update interval.
	UpdateInterval time.Duration

	// WindowSize is the size of the sliding window for bandwidth estimation.
	WindowSize time.Duration

	// MinBitrate is the minimum estimated bitrate in bps.
	MinBitrate uint64

	// MaxBitrate is the maximum estimated bitrate in bps.
	MaxBitrate uint64
}

// DefaultTWCCConfig returns the default TWCC configuration.
func DefaultTWCCConfig() *TWCCConfig {
	return &TWCCConfig{
		UpdateInterval: 100 * time.Millisecond,
		WindowSize:     1 * time.Second,
		MinBitrate:     100_000,    // 100 Kbps minimum
		MaxBitrate:     50_000_000, // 50 Mbps maximum
	}
}

// PacketInfo contains information about a sent packet for TWCC tracking.
type PacketInfo struct {
	// SequenceNumber is the transport-wide sequence number.
	SequenceNumber uint16

	// Size is the packet size in bytes.
	Size int

	// SentAt is the time when the packet was sent.
	SentAt time.Time

	// ReceivedAt is the time when the acknowledgment was received.
	// Zero if not yet acknowledged.
	ReceivedAt time.Time

	// Acknowledged indicates if the packet was acknowledged.
	Acknowledged bool

	// Lost indicates if the packet was determined to be lost.
	Lost bool
}

// TWCCHandler handles Transport-wide Congestion Control feedback.
// Per tasks.md 3.2.2: Implements TWCC processing with bandwidth estimation.
type TWCCHandler struct {
	mu     sync.RWMutex
	config *TWCCConfig

	// packets stores sent packet information for tracking.
	// Key is the transport-wide sequence number.
	packets map[uint16]*PacketInfo

	// lastSequenceNumber is the last assigned sequence number.
	lastSequenceNumber uint16

	// bandwidthEstimate is the current estimated bandwidth in bps.
	bandwidthEstimate uint64

	// lastFeedbackTime is the time of the last received feedback.
	lastFeedbackTime time.Time

	// packetsInFlight counts packets awaiting acknowledgment.
	packetsInFlight int

	// totalBytesSent counts total bytes sent.
	totalBytesSent uint64

	// totalBytesAcked counts total bytes acknowledged.
	totalBytesAcked uint64

	// totalPacketsLost counts total packets lost.
	totalPacketsLost uint64

	// ctx is the context for cancellation.
	ctx    context.Context
	cancel context.CancelFunc

	// running indicates if the handler is running.
	running bool

	// onBandwidthUpdate is called when bandwidth estimate is updated.
	onBandwidthUpdate func(bitrate uint64)
}

// NewTWCCHandler creates a new TWCC handler.
func NewTWCCHandler(cfg *TWCCConfig) *TWCCHandler {
	if cfg == nil {
		cfg = DefaultTWCCConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TWCCHandler{
		config:            cfg,
		packets:           make(map[uint16]*PacketInfo),
		bandwidthEstimate: cfg.MaxBitrate / 2, // Start at half of max
		ctx:               ctx,
		cancel:            cancel,
	}
}

// Start starts the TWCC handler's background goroutine.
func (t *TWCCHandler) Start() {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	// Recreate context to support restart after Stop
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.mu.Unlock()

	go t.estimationLoop()
}

// Stop stops the TWCC handler.
func (t *TWCCHandler) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}
	t.running = false
	t.cancel()
}

// estimationLoop periodically updates bandwidth estimation.
func (t *TWCCHandler) estimationLoop() {
	ticker := time.NewTicker(t.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.updateBandwidthEstimate()
		}
	}
}

// RecordPacketSent records a sent packet for TWCC tracking.
func (t *TWCCHandler) RecordPacketSent(size int) uint16 {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastSequenceNumber++
	seqNum := t.lastSequenceNumber

	t.packets[seqNum] = &PacketInfo{
		SequenceNumber: seqNum,
		Size:           size,
		SentAt:         time.Now(),
	}

	t.packetsInFlight++
	t.totalBytesSent += uint64(size)

	// Clean up old packets outside the window
	t.cleanupOldPackets()

	return seqNum
}

// HandleFeedback processes a TWCC feedback packet.
func (t *TWCCHandler) HandleFeedback(feedback *rtcp.TransportLayerCC) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastFeedbackTime = time.Now()

	// Process the feedback
	// TransportLayerCC contains packet status information
	baseSeq := feedback.BaseSequenceNumber
	refTime := feedback.ReferenceTime

	// Process packet chunks to extract receive deltas
	packetStatuses := t.extractPacketStatuses(feedback)

	for i, status := range packetStatuses {
		seqNum := baseSeq + uint16(i)
		info, exists := t.packets[seqNum]
		if !exists {
			continue
		}

		if status.received {
			if !info.Acknowledged {
				info.Acknowledged = true
				info.ReceivedAt = time.Now()
				t.packetsInFlight--
				t.totalBytesAcked += uint64(info.Size)
			}
		} else if !info.Lost && !info.Acknowledged {
			// Mark as lost if not received and not already acknowledged
			info.Lost = true
			t.packetsInFlight--
			t.totalPacketsLost++
		}
	}

	// Use refTime to avoid unused variable warning
	_ = refTime
}

// packetStatus represents the status of a packet in TWCC feedback.
type packetStatus struct {
	received bool
	delta    int64 // receive delta in microseconds
}

// extractPacketStatuses extracts packet statuses from TWCC feedback.
func (t *TWCCHandler) extractPacketStatuses(feedback *rtcp.TransportLayerCC) []packetStatus {
	var statuses []packetStatus

	for _, chunk := range feedback.PacketChunks {
		switch c := chunk.(type) {
		case *rtcp.RunLengthChunk:
			for i := uint16(0); i < c.RunLength; i++ {
				statuses = append(statuses, packetStatus{
					received: c.PacketStatusSymbol != rtcp.TypeTCCPacketNotReceived,
				})
			}
		case *rtcp.StatusVectorChunk:
			for _, symbol := range c.SymbolList {
				statuses = append(statuses, packetStatus{
					received: symbol != rtcp.TypeTCCPacketNotReceived,
				})
			}
		}
	}

	// Apply receive deltas
	deltaIdx := 0
	for i := range statuses {
		if statuses[i].received && deltaIdx < len(feedback.RecvDeltas) {
			statuses[i].delta = int64(feedback.RecvDeltas[deltaIdx].Delta)
			deltaIdx++
		}
	}

	return statuses
}

// updateBandwidthEstimate updates the bandwidth estimate based on collected data.
func (t *TWCCHandler) updateBandwidthEstimate() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Calculate bandwidth based on acknowledged bytes over time window
	windowStart := time.Now().Add(-t.config.WindowSize)
	var bytesInWindow uint64
	var packetsInWindow int
	var lostInWindow int

	for _, info := range t.packets {
		if info.SentAt.After(windowStart) {
			packetsInWindow++
			if info.Acknowledged {
				bytesInWindow += uint64(info.Size)
			} else if info.Lost {
				lostInWindow++
			}
		}
	}

	// Calculate loss rate
	var lossRate float64
	if packetsInWindow > 0 {
		lossRate = float64(lostInWindow) / float64(packetsInWindow)
	}

	// Calculate throughput in bps (bits per second)
	// bytesInWindow * 8 = bits in window
	// Divide by window duration in seconds to get bps
	windowSeconds := t.config.WindowSize.Seconds()
	var throughput uint64
	if windowSeconds > 0 {
		throughput = uint64(float64(bytesInWindow*8) / windowSeconds)
	}

	// Adjust estimate based on throughput and loss
	var newEstimate uint64
	if throughput > 0 {
		// Use measured throughput as baseline
		newEstimate = throughput

		// Reduce estimate if loss is high
		if lossRate > 0.05 {
			// More than 5% loss - reduce by 20%
			newEstimate = uint64(float64(newEstimate) * 0.8)
		} else if lossRate < 0.01 {
			// Less than 1% loss - can increase by 10%
			newEstimate = uint64(float64(newEstimate) * 1.1)
		}
	} else if t.bandwidthEstimate > 0 {
		// No data - keep current estimate
		newEstimate = t.bandwidthEstimate
	}

	// Clamp to configured limits
	if newEstimate < t.config.MinBitrate {
		newEstimate = t.config.MinBitrate
	}
	if newEstimate > t.config.MaxBitrate {
		newEstimate = t.config.MaxBitrate
	}

	// Smooth the estimate (exponential moving average)
	if t.bandwidthEstimate > 0 {
		t.bandwidthEstimate = (t.bandwidthEstimate*7 + newEstimate*3) / 10
	} else {
		t.bandwidthEstimate = newEstimate
	}

	// Notify callback
	if t.onBandwidthUpdate != nil {
		t.onBandwidthUpdate(t.bandwidthEstimate)
	}
}

// cleanupOldPackets removes packets outside the tracking window.
func (t *TWCCHandler) cleanupOldPackets() {
	cutoff := time.Now().Add(-t.config.WindowSize * 2)
	for seqNum, info := range t.packets {
		if info.SentAt.Before(cutoff) {
			delete(t.packets, seqNum)
		}
	}
}

// GetBandwidthEstimate returns the current bandwidth estimate in bps.
func (t *TWCCHandler) GetBandwidthEstimate() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.bandwidthEstimate
}

// GetPacketLossRate returns the current packet loss rate (0.0-1.0).
func (t *TWCCHandler) GetPacketLossRate() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	windowStart := time.Now().Add(-t.config.WindowSize)
	var totalPackets int
	var lostPackets int

	for _, info := range t.packets {
		if info.SentAt.After(windowStart) {
			totalPackets++
			if info.Lost {
				lostPackets++
			}
		}
	}

	if totalPackets == 0 {
		return 0
	}
	return float64(lostPackets) / float64(totalPackets)
}

// GetStats returns TWCC statistics.
func (t *TWCCHandler) GetStats() TWCCStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TWCCStats{
		BandwidthEstimate:  t.bandwidthEstimate,
		PacketsInFlight:    t.packetsInFlight,
		TotalBytesSent:     t.totalBytesSent,
		TotalBytesAcked:    t.totalBytesAcked,
		TotalPacketsLost:   t.totalPacketsLost,
		LastFeedbackTime:   t.lastFeedbackTime,
		LastSequenceNumber: t.lastSequenceNumber,
	}
}

// TWCCStats contains TWCC statistics.
type TWCCStats struct {
	BandwidthEstimate  uint64
	PacketsInFlight    int
	TotalBytesSent     uint64
	TotalBytesAcked    uint64
	TotalPacketsLost   uint64
	LastFeedbackTime   time.Time
	LastSequenceNumber uint16
}

// SetOnBandwidthUpdate sets the callback for bandwidth updates.
func (t *TWCCHandler) SetOnBandwidthUpdate(cb func(bitrate uint64)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onBandwidthUpdate = cb
}

// GenerateFeedback generates a TWCC feedback packet.
// This is used when acting as a receiver to report packet reception.
func (t *TWCCHandler) GenerateFeedback(senderSSRC, mediaSSRC uint32) *rtcp.TransportLayerCC {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.packets) == 0 {
		return nil
	}

	// Find the range of packets to report
	var minSeq, maxSeq uint16
	first := true
	for seqNum := range t.packets {
		if first {
			minSeq = seqNum
			maxSeq = seqNum
			first = false
		} else {
			if seqNum < minSeq {
				minSeq = seqNum
			}
			if seqNum > maxSeq {
				maxSeq = seqNum
			}
		}
	}

	// Build packet status chunks
	var chunks []rtcp.PacketStatusChunk
	var recvDeltas []*rtcp.RecvDelta

	// Create a run-length chunk for the packet range
	runLength := maxSeq - minSeq + 1
	chunk := &rtcp.RunLengthChunk{
		PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
		RunLength:          runLength,
	}
	chunks = append(chunks, chunk)

	// Add receive deltas (simplified - using small delta for all)
	for seqNum := minSeq; seqNum <= maxSeq; seqNum++ {
		if info, exists := t.packets[seqNum]; exists && info.Acknowledged {
			recvDeltas = append(recvDeltas, &rtcp.RecvDelta{
				Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
				Delta: 0, // Simplified - would need actual timing
			})
		}
	}

	return &rtcp.TransportLayerCC{
		SenderSSRC:         senderSSRC,
		MediaSSRC:          mediaSSRC,
		BaseSequenceNumber: minSeq,
		PacketStatusCount:  runLength,
		ReferenceTime:      uint32(time.Now().UnixMicro() / 64),
		FbPktCount:         0,
		PacketChunks:       chunks,
		RecvDeltas:         recvDeltas,
	}
}
