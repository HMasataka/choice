// Package quality provides quality control for media streams.
// Per design.md section 3.5 and tasks.md Phase 3.3.
package quality

import (
	"sync"
	"time"
)

// BandwidthEstimatorConfig contains configuration for bandwidth estimation.
type BandwidthEstimatorConfig struct {
	// TWCCTimeout is the duration after which TWCC estimates are considered stale.
	TWCCTimeout time.Duration

	// REMBTimeout is the duration after which REMB estimates are considered stale.
	REMBTimeout time.Duration

	// DefaultBandwidth is the default bandwidth used when no estimates are available.
	// Per tasks.md: Use default bandwidth when neither TWCC nor REMB is received.
	DefaultBandwidth uint64

	// UpdateInterval is the interval for bandwidth estimation updates.
	// Per tasks.md: 100ms update interval.
	UpdateInterval time.Duration
}

// DefaultBandwidthEstimatorConfig returns the default configuration.
func DefaultBandwidthEstimatorConfig() *BandwidthEstimatorConfig {
	return &BandwidthEstimatorConfig{
		TWCCTimeout:      2 * time.Second,
		REMBTimeout:      5 * time.Second,
		DefaultBandwidth: 1_000_000, // 1 Mbps default
		UpdateInterval:   100 * time.Millisecond,
	}
}

// BandwidthEstimate contains the current bandwidth estimate and its source.
type BandwidthEstimate struct {
	// Bandwidth is the estimated bandwidth in bits per second.
	Bandwidth uint64

	// Source indicates where the estimate came from.
	Source BandwidthSource

	// Timestamp is when this estimate was last updated.
	Timestamp time.Time
}

// BandwidthSource indicates the source of a bandwidth estimate.
type BandwidthSource string

const (
	// BandwidthSourceTWCC indicates the estimate came from TWCC.
	BandwidthSourceTWCC BandwidthSource = "twcc"

	// BandwidthSourceREMB indicates the estimate came from REMB.
	BandwidthSourceREMB BandwidthSource = "remb"

	// BandwidthSourceDefault indicates using the default bandwidth.
	BandwidthSourceDefault BandwidthSource = "default"
)

// BandwidthEstimator provides hybrid TWCC/REMB bandwidth estimation.
// Per tasks.md 3.3.1: TWCC is preferred, REMB is fallback, default on timeout.
type BandwidthEstimator struct {
	mu     sync.RWMutex
	config *BandwidthEstimatorConfig

	// twccEstimate is the latest TWCC bandwidth estimate.
	twccEstimate uint64
	twccTime     time.Time

	// rembEstimate is the latest REMB bandwidth estimate.
	rembEstimate uint64
	rembTime     time.Time

	// onBandwidthUpdate is called when bandwidth estimate changes.
	onBandwidthUpdate func(estimate BandwidthEstimate)
}

// NewBandwidthEstimator creates a new BandwidthEstimator.
func NewBandwidthEstimator(cfg *BandwidthEstimatorConfig) *BandwidthEstimator {
	if cfg == nil {
		cfg = DefaultBandwidthEstimatorConfig()
	}
	return &BandwidthEstimator{
		config: cfg,
	}
}

// UpdateTWCC updates the TWCC bandwidth estimate.
// Per tasks.md: TWCC is the primary/preferred bandwidth estimation method.
func (e *BandwidthEstimator) UpdateTWCC(bandwidth uint64) {
	e.mu.Lock()
	e.twccEstimate = bandwidth
	e.twccTime = time.Now()
	cb := e.onBandwidthUpdate
	estimate := e.getEstimateLocked()
	e.mu.Unlock()

	// Call callback outside of lock to prevent deadlock
	if cb != nil {
		cb(estimate)
	}
}

// UpdateREMB updates the REMB bandwidth estimate.
// Per tasks.md: REMB is the fallback for legacy clients not supporting TWCC.
func (e *BandwidthEstimator) UpdateREMB(bandwidth uint64) {
	e.mu.Lock()
	e.rembEstimate = bandwidth
	e.rembTime = time.Now()

	// Only notify if TWCC is stale (REMB is fallback)
	var cb func(BandwidthEstimate)
	var estimate BandwidthEstimate
	if e.isTWCCStaleLocked() {
		cb = e.onBandwidthUpdate
		estimate = e.getEstimateLocked()
	}
	e.mu.Unlock()

	// Call callback outside of lock to prevent deadlock
	if cb != nil {
		cb(estimate)
	}
}

// GetEstimate returns the current bandwidth estimate.
// Per tasks.md: TWCC preferred, REMB fallback, default on timeout.
func (e *BandwidthEstimator) GetEstimate() BandwidthEstimate {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.getEstimateLocked()
}

// getEstimateLocked returns the estimate while holding the lock.
func (e *BandwidthEstimator) getEstimateLocked() BandwidthEstimate {
	now := time.Now()

	// Prefer TWCC if available and not stale
	if !e.isTWCCStaleLocked() && e.twccEstimate > 0 {
		return BandwidthEstimate{
			Bandwidth: e.twccEstimate,
			Source:    BandwidthSourceTWCC,
			Timestamp: e.twccTime,
		}
	}

	// Fall back to REMB if available and not stale
	if !e.isREMBStaleLocked() && e.rembEstimate > 0 {
		return BandwidthEstimate{
			Bandwidth: e.rembEstimate,
			Source:    BandwidthSourceREMB,
			Timestamp: e.rembTime,
		}
	}

	// Use default bandwidth
	return BandwidthEstimate{
		Bandwidth: e.config.DefaultBandwidth,
		Source:    BandwidthSourceDefault,
		Timestamp: now,
	}
}

// isTWCCStaleLocked checks if TWCC estimate is stale (lock must be held).
func (e *BandwidthEstimator) isTWCCStaleLocked() bool {
	if e.twccTime.IsZero() {
		return true
	}
	return time.Since(e.twccTime) > e.config.TWCCTimeout
}

// isREMBStaleLocked checks if REMB estimate is stale (lock must be held).
func (e *BandwidthEstimator) isREMBStaleLocked() bool {
	if e.rembTime.IsZero() {
		return true
	}
	return time.Since(e.rembTime) > e.config.REMBTimeout
}

// SetOnBandwidthUpdate sets the callback for bandwidth updates.
func (e *BandwidthEstimator) SetOnBandwidthUpdate(cb func(estimate BandwidthEstimate)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onBandwidthUpdate = cb
}

// GetTWCCEstimate returns the raw TWCC estimate (for debugging/testing).
func (e *BandwidthEstimator) GetTWCCEstimate() (uint64, time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.twccEstimate, e.twccTime
}

// GetREMBEstimate returns the raw REMB estimate (for debugging/testing).
func (e *BandwidthEstimator) GetREMBEstimate() (uint64, time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.rembEstimate, e.rembTime
}

// Reset clears all estimates.
func (e *BandwidthEstimator) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.twccEstimate = 0
	e.twccTime = time.Time{}
	e.rembEstimate = 0
	e.rembTime = time.Time{}
}
