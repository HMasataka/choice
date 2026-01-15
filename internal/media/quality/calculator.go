package quality

import (
	"context"
	"sync"
	"time"
)

// ConnectionQuality represents the overall connection quality level.
// Per tasks.md 3.3.3: Quality levels are excellent/good/fair/poor.
type ConnectionQuality string

const (
	// ConnectionQualityExcellent indicates optimal network conditions.
	ConnectionQualityExcellent ConnectionQuality = "excellent"

	// ConnectionQualityGood indicates good network conditions.
	ConnectionQualityGood ConnectionQuality = "good"

	// ConnectionQualityFair indicates acceptable but degraded network conditions.
	ConnectionQualityFair ConnectionQuality = "fair"

	// ConnectionQualityPoor indicates poor network conditions.
	ConnectionQualityPoor ConnectionQuality = "poor"
)

// CalculatorConfig contains configuration for quality calculation.
type CalculatorConfig struct {
	// UpdateInterval is how often quality is recalculated.
	UpdateInterval time.Duration

	// ExcellentThreshold defines the thresholds for excellent quality.
	ExcellentThreshold QualityThresholds

	// GoodThreshold defines the thresholds for good quality.
	GoodThreshold QualityThresholds

	// FairThreshold defines the thresholds for fair quality.
	FairThreshold QualityThresholds

	// StaleTimeout is how long before metrics are considered stale.
	StaleTimeout time.Duration
}

// QualityThresholds defines the maximum values for a quality level.
// All values must be at or below these thresholds to achieve the quality level.
type QualityThresholds struct {
	// MaxPacketLossRate is the maximum packet loss rate (0.0-1.0).
	MaxPacketLossRate float64

	// MaxRTT is the maximum RTT in milliseconds.
	MaxRTT float64

	// MaxJitter is the maximum jitter in milliseconds.
	MaxJitter float64
}

// DefaultCalculatorConfig returns the default configuration.
func DefaultCalculatorConfig() *CalculatorConfig {
	return &CalculatorConfig{
		UpdateInterval: 1 * time.Second,
		// Excellent: <0.1% loss, <50ms RTT, <10ms jitter
		ExcellentThreshold: QualityThresholds{
			MaxPacketLossRate: 0.001,
			MaxRTT:            50,
			MaxJitter:         10,
		},
		// Good: <1% loss, <150ms RTT, <30ms jitter
		GoodThreshold: QualityThresholds{
			MaxPacketLossRate: 0.01,
			MaxRTT:            150,
			MaxJitter:         30,
		},
		// Fair: <5% loss, <300ms RTT, <50ms jitter
		FairThreshold: QualityThresholds{
			MaxPacketLossRate: 0.05,
			MaxRTT:            300,
			MaxJitter:         50,
		},
		// Poor: anything above fair thresholds
		StaleTimeout: 10 * time.Second,
	}
}

// ConnectionQualityResult contains the calculated connection quality.
type ConnectionQualityResult struct {
	// ParticipantID is the ID of the participant.
	ParticipantID string

	// Quality is the calculated quality level.
	Quality ConnectionQuality

	// Score is a numeric score (0-100) representing quality.
	Score int

	// Metrics are the metrics used to calculate quality.
	Metrics QualityMetrics

	// PreviousQuality is the previous quality level (if changed).
	PreviousQuality ConnectionQuality

	// Changed indicates if quality changed from previous calculation.
	Changed bool

	// Timestamp is when this result was calculated.
	Timestamp time.Time
}

// participantQualityState tracks quality state for a participant.
type participantQualityState struct {
	currentQuality ConnectionQuality
	lastMetrics    QualityMetrics
	lastUpdate     time.Time
}

// Calculator calculates connection quality from RTCP statistics.
// Per tasks.md 3.3.3: Calculates quality from packet loss, RTT, and jitter.
type Calculator struct {
	mu     sync.RWMutex
	config *CalculatorConfig

	// participantStates tracks state per participant.
	participantStates map[string]*participantQualityState

	// onQualityChanged is called when connection quality changes.
	// Per tasks.md: Fires connectionQualityChanged notification.
	onQualityChanged func(result ConnectionQualityResult)

	// ctx for background processing
	ctx    context.Context
	cancel context.CancelFunc

	// running indicates if the calculator is running.
	running bool
}

// NewCalculator creates a new Calculator.
func NewCalculator(cfg *CalculatorConfig) *Calculator {
	if cfg == nil {
		cfg = DefaultCalculatorConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Calculator{
		config:            cfg,
		participantStates: make(map[string]*participantQualityState),
		ctx:               ctx,
		cancel:            cancel,
	}
}

// Start starts the calculator's background processing.
func (c *Calculator) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.mu.Unlock()

	go c.updateLoop()
}

// Stop stops the calculator.
func (c *Calculator) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}
	c.running = false
	c.cancel()
}

// updateLoop periodically recalculates quality for all participants.
func (c *Calculator) updateLoop() {
	ticker := time.NewTicker(c.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.recalculateAll()
		}
	}
}

// recalculateAll recalculates quality for all participants.
func (c *Calculator) recalculateAll() {
	c.mu.Lock()

	now := time.Now()
	var changedResults []ConnectionQualityResult
	cb := c.onQualityChanged

	for participantID, state := range c.participantStates {
		// Skip stale metrics
		if now.Sub(state.lastUpdate) > c.config.StaleTimeout {
			continue
		}

		result := c.calculateQualityLocked(participantID, state)
		if result.Changed {
			changedResults = append(changedResults, result)
		}
	}
	c.mu.Unlock()

	// Call callbacks outside of lock to prevent deadlock
	if cb != nil {
		for _, result := range changedResults {
			cb(result)
		}
	}
}

// UpdateMetrics updates the metrics for a participant.
func (c *Calculator) UpdateMetrics(participantID string, metrics QualityMetrics) ConnectionQualityResult {
	c.mu.Lock()

	state := c.getOrCreateStateLocked(participantID)
	state.lastMetrics = metrics
	state.lastUpdate = time.Now()

	result := c.calculateQualityLocked(participantID, state)

	// Capture callback before releasing lock
	var cb func(result ConnectionQualityResult)
	if result.Changed {
		cb = c.onQualityChanged
	}
	c.mu.Unlock()

	// Call callback outside of lock to prevent deadlock
	if cb != nil {
		cb(result)
	}

	return result
}

// calculateQualityLocked calculates quality for a participant (lock must be held).
func (c *Calculator) calculateQualityLocked(participantID string, state *participantQualityState) ConnectionQualityResult {
	metrics := state.lastMetrics
	previousQuality := state.currentQuality

	// Calculate quality level based on thresholds
	var quality ConnectionQuality
	if c.meetsThreshold(metrics, c.config.ExcellentThreshold) {
		quality = ConnectionQualityExcellent
	} else if c.meetsThreshold(metrics, c.config.GoodThreshold) {
		quality = ConnectionQualityGood
	} else if c.meetsThreshold(metrics, c.config.FairThreshold) {
		quality = ConnectionQualityFair
	} else {
		quality = ConnectionQualityPoor
	}

	// Calculate numeric score (0-100)
	score := c.calculateScore(metrics)

	// Check if quality changed
	changed := previousQuality != quality && previousQuality != ""

	// Update state
	state.currentQuality = quality

	return ConnectionQualityResult{
		ParticipantID:   participantID,
		Quality:         quality,
		Score:           score,
		Metrics:         metrics,
		PreviousQuality: previousQuality,
		Changed:         changed,
		Timestamp:       time.Now(),
	}
}

// meetsThreshold checks if metrics meet the given threshold.
func (c *Calculator) meetsThreshold(metrics QualityMetrics, threshold QualityThresholds) bool {
	return metrics.PacketLossRate <= threshold.MaxPacketLossRate &&
		metrics.RTT <= threshold.MaxRTT &&
		metrics.Jitter <= threshold.MaxJitter
}

// calculateScore calculates a numeric quality score (0-100).
func (c *Calculator) calculateScore(metrics QualityMetrics) int {
	// Weight factors for each metric
	const (
		lossWeight   = 0.5
		rttWeight    = 0.3
		jitterWeight = 0.2
	)

	// Calculate individual scores (higher is better)
	// Loss score: 100 at 0% loss, 0 at 10% loss
	lossScore := 100.0 - (metrics.PacketLossRate * 1000)
	if lossScore < 0 {
		lossScore = 0
	}

	// RTT score: 100 at 0ms, 0 at 500ms
	rttScore := 100.0 - (metrics.RTT / 5)
	if rttScore < 0 {
		rttScore = 0
	}

	// Jitter score: 100 at 0ms, 0 at 100ms
	jitterScore := 100.0 - metrics.Jitter
	if jitterScore < 0 {
		jitterScore = 0
	}

	// Calculate weighted score
	score := int(lossScore*lossWeight + rttScore*rttWeight + jitterScore*jitterWeight)
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

// getOrCreateStateLocked gets or creates participant state (lock must be held).
func (c *Calculator) getOrCreateStateLocked(participantID string) *participantQualityState {
	if state, ok := c.participantStates[participantID]; ok {
		return state
	}
	state := &participantQualityState{}
	c.participantStates[participantID] = state
	return state
}

// SetOnQualityChanged sets the callback for quality changes.
// Per tasks.md: This callback triggers connectionQualityChanged notification.
func (c *Calculator) SetOnQualityChanged(cb func(result ConnectionQualityResult)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onQualityChanged = cb
}

// GetQuality returns the current quality for a participant.
func (c *Calculator) GetQuality(participantID string) (ConnectionQuality, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state, ok := c.participantStates[participantID]
	if !ok {
		return "", false
	}
	return state.currentQuality, true
}

// GetQualityResult returns the full quality result for a participant.
func (c *Calculator) GetQualityResult(participantID string) (ConnectionQualityResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state, ok := c.participantStates[participantID]
	if !ok {
		return ConnectionQualityResult{}, false
	}

	return ConnectionQualityResult{
		ParticipantID:   participantID,
		Quality:         state.currentQuality,
		Score:           c.calculateScore(state.lastMetrics),
		Metrics:         state.lastMetrics,
		PreviousQuality: "",
		Changed:         false,
		Timestamp:       state.lastUpdate,
	}, true
}

// RemoveParticipant removes a participant's state.
func (c *Calculator) RemoveParticipant(participantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.participantStates, participantID)
}

// GetAllQualities returns the current quality for all participants.
func (c *Calculator) GetAllQualities() map[string]ConnectionQuality {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]ConnectionQuality, len(c.participantStates))
	for participantID, state := range c.participantStates {
		if state.currentQuality != "" {
			result[participantID] = state.currentQuality
		}
	}
	return result
}
