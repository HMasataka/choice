package rtcp

import (
	"sync"
	"time"

	"github.com/pion/rtcp"
)

// REMBConfig contains configuration for REMB handling.
type REMBConfig struct {
	// MinBitrate is the minimum estimated bitrate in bps.
	MinBitrate uint64

	// MaxBitrate is the maximum estimated bitrate in bps.
	MaxBitrate uint64

	// ExpirationTime is the time after which an estimate is considered stale.
	ExpirationTime time.Duration
}

// DefaultREMBConfig returns the default REMB configuration.
func DefaultREMBConfig() *REMBConfig {
	return &REMBConfig{
		MinBitrate:     100_000,       // 100 Kbps minimum
		MaxBitrate:     50_000_000,    // 50 Mbps maximum
		ExpirationTime: 5 * time.Second,
	}
}

// REMBHandler handles Receiver Estimated Maximum Bitrate feedback.
// Per tasks.md 3.2.3: REMB fallback for TWCC non-supporting clients.
type REMBHandler struct {
	mu     sync.RWMutex
	config *REMBConfig

	// bandwidthEstimate is the current estimated bandwidth in bps.
	bandwidthEstimate uint64

	// lastEstimateTime is the time of the last estimate update.
	lastEstimateTime time.Time

	// estimates stores per-SSRC estimates for aggregation.
	estimates map[uint32]*rembEstimate

	// onBandwidthUpdate is called when bandwidth estimate is updated.
	onBandwidthUpdate func(bitrate uint64)
}

// rembEstimate contains an REMB estimate for an SSRC.
type rembEstimate struct {
	bitrate   uint64
	timestamp time.Time
}

// NewREMBHandler creates a new REMB handler.
func NewREMBHandler(cfg *REMBConfig) *REMBHandler {
	if cfg == nil {
		cfg = DefaultREMBConfig()
	}

	return &REMBHandler{
		config:    cfg,
		estimates: make(map[uint32]*rembEstimate),
	}
}

// HandleREMB processes an REMB feedback packet.
func (r *REMBHandler) HandleREMB(remb *rtcp.ReceiverEstimatedMaximumBitrate) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	bitrate := uint64(remb.Bitrate)

	// Clamp to configured limits
	if bitrate < r.config.MinBitrate {
		bitrate = r.config.MinBitrate
	}
	if bitrate > r.config.MaxBitrate {
		bitrate = r.config.MaxBitrate
	}

	// Update per-SSRC estimates
	for _, ssrc := range remb.SSRCs {
		r.estimates[ssrc] = &rembEstimate{
			bitrate:   bitrate,
			timestamp: now,
		}
	}

	// Clean up stale estimates
	r.cleanupStaleEstimates()

	// Calculate aggregate estimate
	r.updateAggregateEstimate()

	// Notify callback
	if r.onBandwidthUpdate != nil {
		r.onBandwidthUpdate(r.bandwidthEstimate)
	}
}

// cleanupStaleEstimates removes estimates that are too old.
func (r *REMBHandler) cleanupStaleEstimates() {
	cutoff := time.Now().Add(-r.config.ExpirationTime)
	for ssrc, estimate := range r.estimates {
		if estimate.timestamp.Before(cutoff) {
			delete(r.estimates, ssrc)
		}
	}
}

// updateAggregateEstimate updates the aggregate bandwidth estimate.
// Uses the minimum estimate across all SSRCs for conservative estimation.
func (r *REMBHandler) updateAggregateEstimate() {
	if len(r.estimates) == 0 {
		return
	}

	// Use minimum estimate (conservative approach for shared bandwidth)
	var minBitrate uint64 = ^uint64(0) // Max uint64
	for _, estimate := range r.estimates {
		if estimate.bitrate < minBitrate {
			minBitrate = estimate.bitrate
		}
	}

	r.bandwidthEstimate = minBitrate
	r.lastEstimateTime = time.Now()
}

// GetBandwidthEstimate returns the current bandwidth estimate in bps.
func (r *REMBHandler) GetBandwidthEstimate() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return 0 if estimate is stale
	if time.Since(r.lastEstimateTime) > r.config.ExpirationTime {
		return 0
	}

	return r.bandwidthEstimate
}

// GetEstimateForSSRC returns the bandwidth estimate for a specific SSRC.
func (r *REMBHandler) GetEstimateForSSRC(ssrc uint32) uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	estimate, ok := r.estimates[ssrc]
	if !ok {
		return 0
	}

	// Return 0 if estimate is stale
	if time.Since(estimate.timestamp) > r.config.ExpirationTime {
		return 0
	}

	return estimate.bitrate
}

// GetStats returns REMB statistics.
func (r *REMBHandler) GetStats() REMBStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ssrcEstimates := make(map[uint32]uint64, len(r.estimates))
	for ssrc, estimate := range r.estimates {
		ssrcEstimates[ssrc] = estimate.bitrate
	}

	return REMBStats{
		BandwidthEstimate: r.bandwidthEstimate,
		LastEstimateTime:  r.lastEstimateTime,
		SSRCEstimates:     ssrcEstimates,
	}
}

// REMBStats contains REMB statistics.
type REMBStats struct {
	BandwidthEstimate uint64
	LastEstimateTime  time.Time
	SSRCEstimates     map[uint32]uint64
}

// SetOnBandwidthUpdate sets the callback for bandwidth updates.
func (r *REMBHandler) SetOnBandwidthUpdate(cb func(bitrate uint64)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onBandwidthUpdate = cb
}

// GenerateREMB generates an REMB packet to send to a peer.
func (r *REMBHandler) GenerateREMB(senderSSRC uint32, mediaSSRCs []uint32, bitrate uint64) *rtcp.ReceiverEstimatedMaximumBitrate {
	return &rtcp.ReceiverEstimatedMaximumBitrate{
		SenderSSRC: senderSSRC,
		Bitrate:    float32(bitrate),
		SSRCs:      mediaSSRCs,
	}
}

// IsStale returns true if the bandwidth estimate is stale.
func (r *REMBHandler) IsStale() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return time.Since(r.lastEstimateTime) > r.config.ExpirationTime
}

// Reset clears all estimates.
func (r *REMBHandler) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.estimates = make(map[uint32]*rembEstimate)
	r.bandwidthEstimate = 0
	r.lastEstimateTime = time.Time{}
}
