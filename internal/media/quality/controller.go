package quality

import (
	"context"
	"sync"
	"time"

	"github.com/HMasataka/choice/internal/media/rtcp"
	"github.com/HMasataka/choice/internal/media/simulcast"
)

// ControllerConfig contains configuration for the quality controller.
type ControllerConfig struct {
	// BandwidthEstimatorConfig is the configuration for bandwidth estimation.
	BandwidthEstimatorConfig *BandwidthEstimatorConfig

	// AdjusterConfig is the configuration for quality adjustment.
	AdjusterConfig *AdjusterConfig

	// CalculatorConfig is the configuration for quality calculation.
	CalculatorConfig *CalculatorConfig

	// MetricsUpdateInterval is how often metrics are collected from RTCP.
	MetricsUpdateInterval time.Duration
}

// DefaultControllerConfig returns the default configuration.
func DefaultControllerConfig() *ControllerConfig {
	return &ControllerConfig{
		BandwidthEstimatorConfig: DefaultBandwidthEstimatorConfig(),
		AdjusterConfig:           DefaultAdjusterConfig(),
		CalculatorConfig:         DefaultCalculatorConfig(),
		MetricsUpdateInterval:    100 * time.Millisecond,
	}
}

// Controller orchestrates quality control for media streams.
// It integrates bandwidth estimation, automatic quality adjustment,
// and connection quality calculation.
type Controller struct {
	mu     sync.RWMutex
	config *ControllerConfig

	// bandwidthEstimator provides hybrid TWCC/REMB bandwidth estimation.
	bandwidthEstimator *BandwidthEstimator

	// adjuster handles automatic quality adjustment.
	adjuster *Adjuster

	// calculator calculates and tracks connection quality.
	calculator *Calculator

	// rtcpHandlers maps participantID to their RTCP handler.
	rtcpHandlers map[string]*rtcp.Handler

	// ctx for background processing
	ctx    context.Context
	cancel context.CancelFunc

	// running indicates if the controller is running.
	running bool

	// callbacks
	onLayerChanged   func(subscriberID string, result simulcast.LayerChangeResult)
	onQualityChanged func(result ConnectionQualityResult)
}

// NewController creates a new quality Controller.
func NewController(cfg *ControllerConfig, simulcastController simulcast.Controller) *Controller {
	if cfg == nil {
		cfg = DefaultControllerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Controller{
		config:             cfg,
		bandwidthEstimator: NewBandwidthEstimator(cfg.BandwidthEstimatorConfig),
		adjuster:           NewAdjuster(cfg.AdjusterConfig, simulcastController),
		calculator:         NewCalculator(cfg.CalculatorConfig),
		rtcpHandlers:       make(map[string]*rtcp.Handler),
		ctx:                ctx,
		cancel:             cancel,
	}

	// Wire up internal callbacks
	// Note: Bandwidth updates are already handled via RTCP handler callbacks in RegisterParticipant.
	// The metricsLoop collects and processes metrics periodically.

	c.adjuster.SetOnAdjustment(func(result AdjustmentResult) {
		// Capture callback outside of adjuster's lock
		c.mu.RLock()
		cb := c.onLayerChanged
		c.mu.RUnlock()

		if cb != nil {
			// Convert AdjustmentResult to LayerChangeResult
			layerResult := simulcast.LayerChangeResult{
				Changed:       true,
				PreviousLayer: result.PreviousLayer,
				CurrentLayer:  result.CurrentLayer,
				Reason:        simulcast.LayerChangeReason(result.Reason),
			}
			cb(result.SubscriberID, layerResult)
		}
	})

	c.calculator.SetOnQualityChanged(func(result ConnectionQualityResult) {
		// Capture callback - calculator already releases lock before calling
		c.mu.RLock()
		cb := c.onQualityChanged
		c.mu.RUnlock()
		if cb != nil {
			cb(result)
		}
	})

	return c
}

// Start starts the quality controller.
func (c *Controller) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.mu.Unlock()

	// Start the calculator
	c.calculator.Start()

	// Start metrics collection loop
	go c.metricsLoop()
}

// Stop stops the quality controller.
func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}
	c.running = false
	c.cancel()

	// Stop the calculator
	c.calculator.Stop()
}

// metricsLoop periodically collects metrics from RTCP handlers.
func (c *Controller) metricsLoop() {
	ticker := time.NewTicker(c.config.MetricsUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.collectMetrics()
		}
	}
}

// collectMetrics collects metrics from all RTCP handlers.
func (c *Controller) collectMetrics() {
	c.mu.RLock()
	handlers := make(map[string]*rtcp.Handler, len(c.rtcpHandlers))
	for k, v := range c.rtcpHandlers {
		handlers[k] = v
	}
	c.mu.RUnlock()

	for participantID, handler := range handlers {
		// Skip nil handlers to prevent panic
		if handler == nil {
			continue
		}

		// Get bandwidth estimate
		estimate := c.bandwidthEstimator.GetEstimate()

		// Get stats from RTCP handler
		allStats := handler.GetAllStats()

		// Aggregate stats across all SSRCs
		var totalLoss float64
		var maxRTT float64
		var avgJitter float64
		var statsCount int

		for _, stats := range allStats {
			totalLoss += stats.FractionLost
			if stats.RTT > maxRTT {
				maxRTT = stats.RTT
			}
			avgJitter += stats.Jitter
			statsCount++
		}

		if statsCount > 0 {
			totalLoss /= float64(statsCount)
			avgJitter /= float64(statsCount)
		} else {
			// Skip participants with no stats to avoid false upgrades
			continue
		}

		// Create metrics
		metrics := QualityMetrics{
			PacketLossRate: totalLoss,
			RTT:            maxRTT,
			Jitter:         avgJitter,
			Bandwidth:      estimate.Bandwidth,
			Timestamp:      time.Now(),
		}

		// Process metrics for quality adjustment
		c.adjuster.ProcessMetrics(c.ctx, participantID, metrics)

		// Update calculator for connection quality
		c.calculator.UpdateMetrics(participantID, metrics)
	}
}

// RegisterParticipant registers a participant with their RTCP handler.
func (c *Controller) RegisterParticipant(participantID string, handler *rtcp.Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.rtcpHandlers[participantID] = handler

	// Set up callbacks on the RTCP handler
	if handler != nil {
		// Wire bandwidth estimate callbacks from RTCP handler's TWCC/REMB
		if twcc := handler.GetTWCCHandler(); twcc != nil {
			twcc.SetOnBandwidthUpdate(func(bitrate uint64) {
				c.bandwidthEstimator.UpdateTWCC(bitrate)
			})
		}
		if remb := handler.GetREMBHandler(); remb != nil {
			remb.SetOnBandwidthUpdate(func(bitrate uint64) {
				c.bandwidthEstimator.UpdateREMB(bitrate)
			})
		}
	}
}

// UnregisterParticipant removes a participant.
func (c *Controller) UnregisterParticipant(participantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.rtcpHandlers, participantID)
	c.adjuster.RemoveSubscriber(participantID)
	c.calculator.RemoveParticipant(participantID)
}

// UpdateTWCC updates TWCC bandwidth estimate.
func (c *Controller) UpdateTWCC(bandwidth uint64) {
	c.bandwidthEstimator.UpdateTWCC(bandwidth)
}

// UpdateREMB updates REMB bandwidth estimate.
func (c *Controller) UpdateREMB(bandwidth uint64) {
	c.bandwidthEstimator.UpdateREMB(bandwidth)
}

// GetBandwidthEstimate returns the current bandwidth estimate.
func (c *Controller) GetBandwidthEstimate() BandwidthEstimate {
	return c.bandwidthEstimator.GetEstimate()
}

// GetConnectionQuality returns the connection quality for a participant.
func (c *Controller) GetConnectionQuality(participantID string) (ConnectionQuality, bool) {
	return c.calculator.GetQuality(participantID)
}

// GetAllConnectionQualities returns connection quality for all participants.
func (c *Controller) GetAllConnectionQualities() map[string]ConnectionQuality {
	return c.calculator.GetAllQualities()
}

// SetOnLayerChanged sets the callback for layer changes.
func (c *Controller) SetOnLayerChanged(cb func(subscriberID string, result simulcast.LayerChangeResult)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onLayerChanged = cb
}

// SetOnQualityChanged sets the callback for quality changes.
func (c *Controller) SetOnQualityChanged(cb func(result ConnectionQualityResult)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onQualityChanged = cb
}

// GetBandwidthEstimator returns the bandwidth estimator.
func (c *Controller) GetBandwidthEstimator() *BandwidthEstimator {
	return c.bandwidthEstimator
}

// GetAdjuster returns the quality adjuster.
func (c *Controller) GetAdjuster() *Adjuster {
	return c.adjuster
}

// GetCalculator returns the connection quality calculator.
func (c *Controller) GetCalculator() *Calculator {
	return c.calculator
}
