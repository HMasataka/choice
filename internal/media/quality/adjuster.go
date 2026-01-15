package quality

import (
	"context"
	"sync"
	"time"

	"github.com/HMasataka/choice/internal/media"
	"github.com/HMasataka/choice/internal/media/simulcast"
)

// AdjusterConfig contains configuration for quality adjustment.
type AdjusterConfig struct {
	// PacketLossThreshold is the packet loss rate that triggers downgrade.
	// Per tasks.md: 5% packet loss triggers low layer switch.
	PacketLossThreshold float64

	// RTTThreshold is the RTT (in milliseconds) that triggers downgrade.
	// Per tasks.md: 300ms RTT triggers low layer switch.
	RTTThreshold float64

	// RecoveryPacketLossThreshold is the packet loss rate below which recovery is considered.
	// Per tasks.md: <1% packet loss allows recovery.
	RecoveryPacketLossThreshold float64

	// RecoveryRTTThreshold is the RTT (in milliseconds) below which recovery is considered.
	RecoveryRTTThreshold float64

	// HysteresisWindow is the time window to prevent layer thrashing.
	HysteresisWindow time.Duration

	// MinDowngradeDuration is the minimum time between consecutive downgrades.
	MinDowngradeDuration time.Duration

	// MinUpgradeDuration is the minimum time to stay at a layer before upgrading.
	MinUpgradeDuration time.Duration
}

// DefaultAdjusterConfig returns the default configuration.
func DefaultAdjusterConfig() *AdjusterConfig {
	return &AdjusterConfig{
		PacketLossThreshold:         0.05, // 5%
		RTTThreshold:                300,  // 300ms
		RecoveryPacketLossThreshold: 0.01, // 1%
		RecoveryRTTThreshold:        150,  // 150ms
		HysteresisWindow:            2 * time.Second,
		MinDowngradeDuration:        1 * time.Second,
		MinUpgradeDuration:          5 * time.Second,
	}
}

// QualityMetrics contains the metrics used for quality adjustment.
type QualityMetrics struct {
	// PacketLossRate is the fraction of packets lost (0.0-1.0).
	PacketLossRate float64

	// RTT is the round-trip time in milliseconds.
	RTT float64

	// Jitter is the jitter in milliseconds.
	Jitter float64

	// Bandwidth is the available bandwidth in bits per second.
	Bandwidth uint64

	// Timestamp is when these metrics were collected.
	Timestamp time.Time
}

// AdjustmentReason indicates why a quality adjustment was made.
type AdjustmentReason string

const (
	// AdjustmentReasonPacketLoss indicates adjustment due to packet loss.
	AdjustmentReasonPacketLoss AdjustmentReason = "packet_loss"

	// AdjustmentReasonRTT indicates adjustment due to high RTT.
	AdjustmentReasonRTT AdjustmentReason = "rtt"

	// AdjustmentReasonBandwidth indicates adjustment due to bandwidth constraints.
	AdjustmentReasonBandwidth AdjustmentReason = "bandwidth"

	// AdjustmentReasonRecovery indicates adjustment due to quality recovery.
	AdjustmentReasonRecovery AdjustmentReason = "recovery"
)

// AdjustmentResult contains the result of a quality adjustment.
type AdjustmentResult struct {
	SubscriberID  string
	PreviousLayer media.SimulcastLayer
	CurrentLayer  media.SimulcastLayer
	Reason        AdjustmentReason
	Metrics       QualityMetrics
	Timestamp     time.Time
}

// subscriberState tracks the state for a single subscriber.
type subscriberState struct {
	lastDowngradeTime time.Time
	lastUpgradeTime   time.Time
	lastLayerChange   time.Time
	currentMetrics    QualityMetrics
}

// Adjuster handles automatic quality adjustment based on network conditions.
// Per tasks.md 3.3.2: Implements automatic quality adjustment logic.
type Adjuster struct {
	mu     sync.RWMutex
	config *AdjusterConfig

	// simulcastController handles the actual layer switching.
	simulcastController simulcast.Controller

	// subscriberStates tracks state per subscriber for hysteresis.
	subscriberStates map[string]*subscriberState

	// onAdjustment is called when a quality adjustment is made.
	onAdjustment func(result AdjustmentResult)
}

// NewAdjuster creates a new Adjuster.
func NewAdjuster(cfg *AdjusterConfig, controller simulcast.Controller) *Adjuster {
	if cfg == nil {
		cfg = DefaultAdjusterConfig()
	}
	return &Adjuster{
		config:              cfg,
		simulcastController: controller,
		subscriberStates:    make(map[string]*subscriberState),
	}
}

// ProcessMetrics processes quality metrics and adjusts layers if needed.
// Returns a list of adjustments that were made.
func (a *Adjuster) ProcessMetrics(ctx context.Context, subscriberID string, metrics QualityMetrics) []AdjustmentResult {
	if subscriberID == "" {
		return nil
	}

	a.mu.Lock()

	// Get or create subscriber state
	state := a.getOrCreateStateLocked(subscriberID)
	state.currentMetrics = metrics

	// Check for downgrade conditions
	shouldDowngrade, reason := a.shouldDowngradeLocked(state, metrics)
	shouldUpgrade := false
	if !shouldDowngrade {
		shouldUpgrade = a.shouldUpgradeLocked(state, metrics)
	}

	// Capture controller and callback before releasing lock
	controller := a.simulcastController
	cb := a.onAdjustment
	a.mu.Unlock()

	var results []AdjustmentResult

	// Perform downgrade outside of lock
	if shouldDowngrade && controller != nil {
		layerResults := controller.OnPacketLoss(ctx, subscriberID, metrics.PacketLossRate)

		a.mu.Lock()
		// Re-fetch state after re-acquiring lock
		state = a.getOrCreateStateLocked(subscriberID)
		for _, lr := range layerResults {
			if lr.Changed {
				result := AdjustmentResult{
					SubscriberID:  subscriberID,
					PreviousLayer: lr.PreviousLayer,
					CurrentLayer:  lr.CurrentLayer,
					Reason:        reason,
					Metrics:       metrics,
					Timestamp:     time.Now(),
				}
				results = append(results, result)
				state.lastDowngradeTime = time.Now()
				state.lastLayerChange = time.Now()
			}
		}
		a.mu.Unlock()

		// Call callbacks outside of lock
		if cb != nil {
			for _, result := range results {
				cb(result)
			}
		}
		return results
	}

	// Perform upgrade outside of lock
	if shouldUpgrade && controller != nil {
		layerResults := controller.OnBandwidthEstimate(ctx, subscriberID, metrics.Bandwidth)

		a.mu.Lock()
		// Re-fetch state after re-acquiring lock
		state = a.getOrCreateStateLocked(subscriberID)
		for _, lr := range layerResults {
			if lr.Changed {
				result := AdjustmentResult{
					SubscriberID:  subscriberID,
					PreviousLayer: lr.PreviousLayer,
					CurrentLayer:  lr.CurrentLayer,
					Reason:        AdjustmentReasonRecovery,
					Metrics:       metrics,
					Timestamp:     time.Now(),
				}
				results = append(results, result)
				state.lastUpgradeTime = time.Now()
				state.lastLayerChange = time.Now()
			}
		}
		a.mu.Unlock()

		// Call callbacks outside of lock
		if cb != nil {
			for _, result := range results {
				cb(result)
			}
		}
	}

	return results
}

// shouldDowngradeLocked checks if we should downgrade based on metrics.
func (a *Adjuster) shouldDowngradeLocked(state *subscriberState, metrics QualityMetrics) (bool, AdjustmentReason) {
	// Check hysteresis - don't downgrade too quickly after a change
	if !state.lastLayerChange.IsZero() &&
		time.Since(state.lastLayerChange) < a.config.MinDowngradeDuration {
		return false, ""
	}

	// Check packet loss threshold
	if metrics.PacketLossRate >= a.config.PacketLossThreshold {
		return true, AdjustmentReasonPacketLoss
	}

	// Check RTT threshold
	if metrics.RTT >= a.config.RTTThreshold {
		return true, AdjustmentReasonRTT
	}

	return false, ""
}

// shouldUpgradeLocked checks if we should upgrade based on metrics.
func (a *Adjuster) shouldUpgradeLocked(state *subscriberState, metrics QualityMetrics) bool {
	// Check hysteresis - don't upgrade too quickly after downgrade
	if !state.lastDowngradeTime.IsZero() &&
		time.Since(state.lastDowngradeTime) < a.config.HysteresisWindow {
		return false
	}

	// Check minimum time at current layer
	if !state.lastLayerChange.IsZero() &&
		time.Since(state.lastLayerChange) < a.config.MinUpgradeDuration {
		return false
	}

	// Check recovery conditions: packet loss < 1% AND RTT is acceptable
	if metrics.PacketLossRate < a.config.RecoveryPacketLossThreshold &&
		metrics.RTT < a.config.RecoveryRTTThreshold {
		return true
	}

	return false
}

// getOrCreateStateLocked gets or creates subscriber state (lock must be held).
func (a *Adjuster) getOrCreateStateLocked(subscriberID string) *subscriberState {
	if state, ok := a.subscriberStates[subscriberID]; ok {
		return state
	}
	state := &subscriberState{}
	a.subscriberStates[subscriberID] = state
	return state
}

// SetOnAdjustment sets the callback for adjustment events.
func (a *Adjuster) SetOnAdjustment(cb func(result AdjustmentResult)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onAdjustment = cb
}

// RemoveSubscriber removes a subscriber's state.
func (a *Adjuster) RemoveSubscriber(subscriberID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.subscriberStates, subscriberID)
}

// GetSubscriberMetrics returns the current metrics for a subscriber.
func (a *Adjuster) GetSubscriberMetrics(subscriberID string) (QualityMetrics, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	state, ok := a.subscriberStates[subscriberID]
	if !ok {
		return QualityMetrics{}, false
	}
	return state.currentMetrics, true
}
