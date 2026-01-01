package sfu

import (
	"log/slog"
	"sync"
	"time"
)

// LayerAllocation represents the target layer allocation for a subscriber
type LayerAllocation struct {
	TrackID      string
	TargetLayer  string
	CurrentLayer string
	MaxLayer     string
	Paused       bool
}

// BandwidthController manages bandwidth allocation across subscribers
type BandwidthController struct {
	config           TWCCConfig
	estimator        *BandwidthEstimator
	allocations      map[string]*LayerAllocation // key: downtrack ID
	availableBitrate uint64
	onLayerChange    func(trackID, layer string)
	mu               sync.RWMutex
	closed           bool
	closeCh          chan struct{}
}

// NewBandwidthController creates a new bandwidth controller
func NewBandwidthController(config TWCCConfig) *BandwidthController {
	bc := &BandwidthController{
		config:           config,
		estimator:        NewBandwidthEstimator(config),
		allocations:      make(map[string]*LayerAllocation),
		availableBitrate: config.InitialBitrate,
		closeCh:          make(chan struct{}),
	}

	// Set up callback for bandwidth changes
	bc.estimator.OnEstimate(func(bitrate uint64) {
		bc.onBitrateUpdate(bitrate)
	})

	return bc
}

// OnLayerChange sets the callback for layer changes
func (bc *BandwidthController) OnLayerChange(cb func(trackID, layer string)) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.onLayerChange = cb
}

// Start starts the bandwidth controller
func (bc *BandwidthController) Start() {
	go bc.allocationLoop()
}

// allocationLoop periodically recalculates layer allocations
func (bc *BandwidthController) allocationLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-bc.closeCh:
			return
		case <-ticker.C:
			bc.recalculateAllocations()
		}
	}
}

// AddTrack adds a track to the bandwidth controller
func (bc *BandwidthController) AddTrack(trackID string, initialLayer string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.allocations[trackID] = &LayerAllocation{
		TrackID:      trackID,
		TargetLayer:  initialLayer,
		CurrentLayer: initialLayer,
		MaxLayer:     LayerHigh,
		Paused:       false,
	}
}

// RemoveTrack removes a track from the bandwidth controller
func (bc *BandwidthController) RemoveTrack(trackID string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	delete(bc.allocations, trackID)
}

// SetMaxLayer sets the maximum layer for a track
func (bc *BandwidthController) SetMaxLayer(trackID, maxLayer string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if alloc, ok := bc.allocations[trackID]; ok {
		alloc.MaxLayer = maxLayer
		// Ensure current target doesn't exceed max
		if LayerPriority(alloc.TargetLayer) > LayerPriority(maxLayer) {
			alloc.TargetLayer = maxLayer
		}
	}
}

// RequestLayer requests a specific layer (manual override)
func (bc *BandwidthController) RequestLayer(trackID, layer string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if alloc, ok := bc.allocations[trackID]; ok {
		// Don't exceed max layer
		if LayerPriority(layer) > LayerPriority(alloc.MaxLayer) {
			layer = alloc.MaxLayer
		}
		alloc.TargetLayer = layer
	}
}

// GetTargetLayer returns the target layer for a track
func (bc *BandwidthController) GetTargetLayer(trackID string) string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if alloc, ok := bc.allocations[trackID]; ok {
		return alloc.TargetLayer
	}
	return LayerHigh
}

// UpdateBitrate updates the bandwidth estimate
func (bc *BandwidthController) UpdateBitrate(receivedBytes uint64, duration time.Duration, lossRate float64) {
	bc.estimator.Update(receivedBytes, duration, lossRate)
}

// onBitrateUpdate handles bitrate updates from the estimator
func (bc *BandwidthController) onBitrateUpdate(bitrate uint64) {
	bc.mu.Lock()
	bc.availableBitrate = bitrate
	bc.mu.Unlock()

	bc.recalculateAllocations()
}

// recalculateAllocations recalculates layer allocations based on available bandwidth
func (bc *BandwidthController) recalculateAllocations() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.closed {
		return
	}

	numTracks := len(bc.allocations)
	if numTracks == 0 {
		return
	}

	// Calculate per-track budget
	perTrackBudget := bc.availableBitrate / uint64(numTracks)

	// Allocate layers based on budget
	for trackID, alloc := range bc.allocations {
		if alloc.Paused {
			continue
		}

		newLayer := bc.selectLayerForBudget(perTrackBudget, alloc.MaxLayer)

		if newLayer != alloc.TargetLayer {
			slog.Info("[BandwidthController] Changing layer for track", slog.String("trackID", trackID),
				slog.String("from", alloc.TargetLayer),
				slog.String("to", newLayer),
				slog.Uint64("budget_bps", perTrackBudget),
			)

			alloc.TargetLayer = newLayer

			if bc.onLayerChange != nil {
				go bc.onLayerChange(trackID, newLayer)
			}
		}
	}
}

// selectLayerForBudget selects the best layer for a given budget
func (bc *BandwidthController) selectLayerForBudget(budget uint64, maxLayer string) string {
	// Typical bitrates for each layer
	layerBitrates := map[string]uint64{
		LayerHigh: 2_500_000, // 2.5 Mbps
		LayerMid:  500_000,   // 500 Kbps
		LayerLow:  150_000,   // 150 Kbps
	}

	maxPriority := LayerPriority(maxLayer)

	// Find the highest layer that fits in the budget
	for _, layer := range []string{LayerHigh, LayerMid, LayerLow} {
		if LayerPriority(layer) > maxPriority {
			continue
		}

		if bitrate, ok := layerBitrates[layer]; ok {
			if bitrate <= budget {
				return layer
			}
		}
	}

	// Default to lowest layer
	return LayerLow
}

// GetAvailableBitrate returns the current available bitrate
func (bc *BandwidthController) GetAvailableBitrate() uint64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.availableBitrate
}

// Close closes the bandwidth controller
func (bc *BandwidthController) Close() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.closed {
		return
	}
	bc.closed = true
	close(bc.closeCh)
}

// LayerSelector handles layer selection for a single subscriber
type LayerSelector struct {
	trackID        string
	currentLayer   string
	targetLayer    string
	pendingSwitch  bool
	lastSwitchTime time.Time
	switchCooldown time.Duration
	onSwitch       func(layer string)
	mu             sync.RWMutex
}

// NewLayerSelector creates a new layer selector
func NewLayerSelector(trackID string, initialLayer string) *LayerSelector {
	if initialLayer == "" {
		initialLayer = LayerHigh
	}
	return &LayerSelector{
		trackID:        trackID,
		currentLayer:   initialLayer,
		targetLayer:    initialLayer,
		switchCooldown: 2 * time.Second, // Minimum time between switches
	}
}

// SetTargetLayer sets the target layer
func (ls *LayerSelector) SetTargetLayer(layer string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if layer != ls.targetLayer {
		ls.targetLayer = layer
		ls.pendingSwitch = true
	}
}

// GetCurrentLayer returns the current layer
func (ls *LayerSelector) GetCurrentLayer() string {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.currentLayer
}

// GetTargetLayer returns the target layer
func (ls *LayerSelector) GetTargetLayer() string {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.targetLayer
}

// NeedsSwitch returns true if a layer switch is pending
func (ls *LayerSelector) NeedsSwitch() bool {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.pendingSwitch && ls.currentLayer != ls.targetLayer
}

// CanSwitch returns true if enough time has passed since the last switch
func (ls *LayerSelector) CanSwitch() bool {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return time.Since(ls.lastSwitchTime) >= ls.switchCooldown
}

// OnSwitch sets the callback for layer switches
func (ls *LayerSelector) OnSwitch(cb func(layer string)) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.onSwitch = cb
}

// SwitchToTarget switches to the target layer (call on keyframe)
func (ls *LayerSelector) SwitchToTarget() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if !ls.pendingSwitch || ls.currentLayer == ls.targetLayer {
		return false
	}

	if time.Since(ls.lastSwitchTime) < ls.switchCooldown {
		return false
	}

	oldLayer := ls.currentLayer
	ls.currentLayer = ls.targetLayer
	ls.pendingSwitch = false
	ls.lastSwitchTime = time.Now()

	slog.Info("[LayerSelector] Switched layer for track", slog.String("trackID", ls.trackID),
		slog.String("from", oldLayer),
		slog.String("to", ls.currentLayer),
	)

	if ls.onSwitch != nil {
		go ls.onSwitch(ls.currentLayer)
	}

	return true
}

// ForceSwitch forces an immediate switch to a layer (for fallback scenarios)
func (ls *LayerSelector) ForceSwitch(layer string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	oldLayer := ls.currentLayer
	ls.currentLayer = layer
	ls.targetLayer = layer
	ls.pendingSwitch = false
	ls.lastSwitchTime = time.Now()

	slog.Info("[LayerSelector] Force switched layer for track", slog.String("trackID", ls.trackID),
		slog.String("from", oldLayer),
		slog.String("to", ls.currentLayer),
	)
}
