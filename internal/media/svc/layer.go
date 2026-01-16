package svc

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/HMasataka/choice/internal/media"
)

// SVCLayer represents a specific SVC layer identified by spatial and temporal indices.
// SVC (Scalable Video Coding) allows a single encoded stream to be decoded at different
// quality levels by selecting different spatial and temporal layers.
type SVCLayer struct {
	SpatialLayer  int // Spatial layer index (0-2, higher is higher resolution)
	TemporalLayer int // Temporal layer index (0-2, higher is higher framerate)
}

// String returns the string representation of SVCLayer (e.g., "S2T2").
func (l SVCLayer) String() string {
	return fmt.Sprintf("S%dT%d", l.SpatialLayer, l.TemporalLayer)
}

// Validate checks if the SVCLayer has valid indices.
func (l SVCLayer) Validate() error {
	if l.SpatialLayer < 0 || l.SpatialLayer > 2 {
		return fmt.Errorf("spatial layer must be 0-2, got %d", l.SpatialLayer)
	}
	if l.TemporalLayer < 0 || l.TemporalLayer > 2 {
		return fmt.Errorf("temporal layer must be 0-2, got %d", l.TemporalLayer)
	}
	return nil
}

// ParseSVCLayer parses a string like "S2T2" into an SVCLayer.
func ParseSVCLayer(s string) (SVCLayer, error) {
	re := regexp.MustCompile(`^S(\d)T(\d)$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return SVCLayer{}, fmt.Errorf("invalid SVC layer format: %s (expected S<n>T<n>)", s)
	}

	// Regex ensures these are single digits, so Atoi won't fail
	spatial, err := strconv.Atoi(matches[1])
	if err != nil {
		return SVCLayer{}, fmt.Errorf("invalid spatial layer: %w", err)
	}
	temporal, err := strconv.Atoi(matches[2])
	if err != nil {
		return SVCLayer{}, fmt.Errorf("invalid temporal layer: %w", err)
	}

	layer := SVCLayer{SpatialLayer: spatial, TemporalLayer: temporal}
	if err := layer.Validate(); err != nil {
		return SVCLayer{}, err
	}
	return layer, nil
}

// LayerSpec defines SVC layer specifications.
// Per design.md section 4.1: SVC uses VP9/AV1 with scalable layers.
type LayerSpec struct {
	SpatialLayer  int // Spatial layer index
	TemporalLayer int // Temporal layer index
	Width         int // Resolution width
	Height        int // Resolution height
	MaxBitrate    int // Maximum bitrate in bps
	MaxFPS        int // Maximum frames per second
}

// ScalabilityMode represents the scalability mode string (e.g., "L3T3").
// L<n>T<m> means n spatial layers and m temporal layers.
type ScalabilityMode string

const (
	// ScalabilityModeL1T1 is 1 spatial layer, 1 temporal layer (no scalability).
	ScalabilityModeL1T1 ScalabilityMode = "L1T1"
	// ScalabilityModeL1T2 is 1 spatial layer, 2 temporal layers.
	ScalabilityModeL1T2 ScalabilityMode = "L1T2"
	// ScalabilityModeL1T3 is 1 spatial layer, 3 temporal layers.
	ScalabilityModeL1T3 ScalabilityMode = "L1T3"
	// ScalabilityModeL2T1 is 2 spatial layers, 1 temporal layer.
	ScalabilityModeL2T1 ScalabilityMode = "L2T1"
	// ScalabilityModeL2T2 is 2 spatial layers, 2 temporal layers.
	ScalabilityModeL2T2 ScalabilityMode = "L2T2"
	// ScalabilityModeL2T3 is 2 spatial layers, 3 temporal layers.
	ScalabilityModeL2T3 ScalabilityMode = "L2T3"
	// ScalabilityModeL3T1 is 3 spatial layers, 1 temporal layer.
	ScalabilityModeL3T1 ScalabilityMode = "L3T1"
	// ScalabilityModeL3T2 is 3 spatial layers, 2 temporal layers.
	ScalabilityModeL3T2 ScalabilityMode = "L3T2"
	// ScalabilityModeL3T3 is 3 spatial layers, 3 temporal layers.
	ScalabilityModeL3T3 ScalabilityMode = "L3T3"
)

// String returns the string representation of ScalabilityMode.
func (m ScalabilityMode) String() string {
	return string(m)
}

// Validate checks if the ScalabilityMode is valid.
func (m ScalabilityMode) Validate() error {
	switch m {
	case ScalabilityModeL1T1, ScalabilityModeL1T2, ScalabilityModeL1T3,
		ScalabilityModeL2T1, ScalabilityModeL2T2, ScalabilityModeL2T3,
		ScalabilityModeL3T1, ScalabilityModeL3T2, ScalabilityModeL3T3:
		return nil
	default:
		return fmt.Errorf("invalid scalability mode: %s", m)
	}
}

// ParseScalabilityMode parses a scalability mode string.
func ParseScalabilityMode(s string) (ScalabilityMode, error) {
	mode := ScalabilityMode(s)
	if err := mode.Validate(); err != nil {
		return "", err
	}
	return mode, nil
}

// GetSpatialLayers returns the number of spatial layers for this mode.
func (m ScalabilityMode) GetSpatialLayers() int {
	switch m {
	case ScalabilityModeL1T1, ScalabilityModeL1T2, ScalabilityModeL1T3:
		return 1
	case ScalabilityModeL2T1, ScalabilityModeL2T2, ScalabilityModeL2T3:
		return 2
	case ScalabilityModeL3T1, ScalabilityModeL3T2, ScalabilityModeL3T3:
		return 3
	default:
		return 0
	}
}

// GetTemporalLayers returns the number of temporal layers for this mode.
func (m ScalabilityMode) GetTemporalLayers() int {
	switch m {
	case ScalabilityModeL1T1, ScalabilityModeL2T1, ScalabilityModeL3T1:
		return 1
	case ScalabilityModeL1T2, ScalabilityModeL2T2, ScalabilityModeL3T2:
		return 2
	case ScalabilityModeL1T3, ScalabilityModeL2T3, ScalabilityModeL3T3:
		return 3
	default:
		return 0
	}
}

// DefaultL3T3Layers contains the default layer specifications for L3T3 mode.
// These values are based on common VP9/AV1 SVC configurations.
var DefaultL3T3Layers = []LayerSpec{
	// Spatial 2 (highest resolution, e.g., 1280x720)
	{SpatialLayer: 2, TemporalLayer: 2, Width: 1280, Height: 720, MaxBitrate: 2_500_000, MaxFPS: 30},
	{SpatialLayer: 2, TemporalLayer: 1, Width: 1280, Height: 720, MaxBitrate: 1_500_000, MaxFPS: 20},
	{SpatialLayer: 2, TemporalLayer: 0, Width: 1280, Height: 720, MaxBitrate: 800_000, MaxFPS: 10},
	// Spatial 1 (medium resolution, e.g., 640x360)
	{SpatialLayer: 1, TemporalLayer: 2, Width: 640, Height: 360, MaxBitrate: 500_000, MaxFPS: 30},
	{SpatialLayer: 1, TemporalLayer: 1, Width: 640, Height: 360, MaxBitrate: 300_000, MaxFPS: 20},
	{SpatialLayer: 1, TemporalLayer: 0, Width: 640, Height: 360, MaxBitrate: 200_000, MaxFPS: 10},
	// Spatial 0 (lowest resolution, e.g., 320x180)
	{SpatialLayer: 0, TemporalLayer: 2, Width: 320, Height: 180, MaxBitrate: 150_000, MaxFPS: 30},
	{SpatialLayer: 0, TemporalLayer: 1, Width: 320, Height: 180, MaxBitrate: 100_000, MaxFPS: 20},
	{SpatialLayer: 0, TemporalLayer: 0, Width: 320, Height: 180, MaxBitrate: 75_000, MaxFPS: 10},
}

// LayerSelectorConfig contains configuration for SVC layer selection logic.
type LayerSelectorConfig struct {
	// PacketLossThreshold is the packet loss rate threshold for switching to lower layer.
	// Per design.md: 5% packet loss triggers low layer switch.
	PacketLossThreshold float64

	// PacketLossRecoveryThreshold is the threshold for returning to higher layer.
	// Per design.md: <1% packet loss with sufficient bandwidth allows recovery.
	PacketLossRecoveryThreshold float64

	// BandwidthMargin is the multiplier for required bandwidth.
	// A layer requires bitrate * BandwidthMargin to be selected.
	BandwidthMargin float64

	// Layers contains the layer specifications to use.
	Layers []LayerSpec

	// ScalabilityMode is the scalability mode being used.
	ScalabilityMode ScalabilityMode
}

// DefaultLayerSelectorConfig returns the default configuration for SVC layer selection.
func DefaultLayerSelectorConfig() *LayerSelectorConfig {
	return &LayerSelectorConfig{
		PacketLossThreshold:         0.05, // 5%
		PacketLossRecoveryThreshold: 0.01, // 1%
		BandwidthMargin:             1.1,  // 10% margin
		Layers:                      DefaultL3T3Layers,
		ScalabilityMode:             ScalabilityModeL3T3,
	}
}

// LayerSelector handles the logic for selecting SVC layers.
type LayerSelector struct {
	config *LayerSelectorConfig
}

// NewLayerSelector creates a new LayerSelector.
func NewLayerSelector(cfg *LayerSelectorConfig) *LayerSelector {
	if cfg == nil {
		cfg = DefaultLayerSelectorConfig()
	}
	return &LayerSelector{config: cfg}
}

// GetLayerSpec returns the specification for a given SVC layer.
func (ls *LayerSelector) GetLayerSpec(layer SVCLayer) (LayerSpec, bool) {
	for _, spec := range ls.config.Layers {
		if spec.SpatialLayer == layer.SpatialLayer && spec.TemporalLayer == layer.TemporalLayer {
			return spec, true
		}
	}
	return LayerSpec{}, false
}

// GetAvailableLayers returns all available layers from the configuration.
func (ls *LayerSelector) GetAvailableLayers() []SVCLayer {
	layers := make([]SVCLayer, 0, len(ls.config.Layers))
	for _, spec := range ls.config.Layers {
		layers = append(layers, SVCLayer{
			SpatialLayer:  spec.SpatialLayer,
			TemporalLayer: spec.TemporalLayer,
		})
	}
	return layers
}

// SelectLayer selects the best available layer based on preference.
// If the preferred layer is not available, selects the next best available layer.
func (ls *LayerSelector) SelectLayer(availableLayers []SVCLayer, preferredLayer SVCLayer) SVCLayer {
	if len(availableLayers) == 0 {
		return SVCLayer{}
	}

	// Check if preferred layer is available
	for _, layer := range availableLayers {
		if layer.SpatialLayer == preferredLayer.SpatialLayer &&
			layer.TemporalLayer == preferredLayer.TemporalLayer {
			return preferredLayer
		}
	}

	// Preferred layer not available, find the next best
	// Priority: higher spatial > higher temporal
	preferredPriority := ls.layerPriority(preferredLayer)

	var bestLayer SVCLayer
	bestPriority := -1
	for _, layer := range availableLayers {
		priority := ls.layerPriority(layer)
		if priority <= preferredPriority && priority > bestPriority {
			bestLayer = layer
			bestPriority = priority
		}
	}

	// If no lower layer found, pick the lowest available layer
	if bestPriority == -1 && len(availableLayers) > 0 {
		lowestPriority := 999
		for _, layer := range availableLayers {
			priority := ls.layerPriority(layer)
			if priority < lowestPriority {
				bestLayer = layer
				lowestPriority = priority
			}
		}
	}

	return bestLayer
}

// SelectLayerForBandwidth selects a layer based on available bandwidth.
// It respects the preferred layer as the upper bound.
func (ls *LayerSelector) SelectLayerForBandwidth(availableLayers []SVCLayer, preferredLayer SVCLayer, bps uint64) SVCLayer {
	if len(availableLayers) == 0 {
		return SVCLayer{}
	}

	// Get the effective preferred layer
	effectivePreferred := ls.SelectLayer(availableLayers, preferredLayer)
	if effectivePreferred.SpatialLayer == 0 && effectivePreferred.TemporalLayer == 0 {
		// Check if this is really the layer or we found nothing
		found := false
		for _, l := range availableLayers {
			if l.SpatialLayer == 0 && l.TemporalLayer == 0 {
				found = true
				break
			}
		}
		if !found {
			return SVCLayer{}
		}
	}

	// Check if we can afford the effective preferred layer
	spec, ok := ls.GetLayerSpec(effectivePreferred)
	if ok {
		requiredBps := uint64(float64(spec.MaxBitrate) * ls.config.BandwidthMargin)
		if bps >= requiredBps {
			return effectivePreferred
		}
	}

	// Cannot afford preferred layer, find the highest layer we can afford
	preferredPriority := ls.layerPriority(effectivePreferred)
	var bestLayer SVCLayer
	bestPriority := -1

	for _, layer := range availableLayers {
		priority := ls.layerPriority(layer)
		// Only consider layers at or below preferred
		if priority > preferredPriority {
			continue
		}

		spec, ok := ls.GetLayerSpec(layer)
		if !ok {
			continue
		}
		requiredBps := uint64(float64(spec.MaxBitrate) * ls.config.BandwidthMargin)
		if bps >= requiredBps && priority > bestPriority {
			bestLayer = layer
			bestPriority = priority
		}
	}

	// If no layer can be afforded, return the lowest available layer
	if bestPriority == -1 {
		lowestPriority := 999
		for _, layer := range availableLayers {
			priority := ls.layerPriority(layer)
			if priority <= preferredPriority && priority < lowestPriority {
				bestLayer = layer
				lowestPriority = priority
			}
		}
	}

	return bestLayer
}

// SelectLayerForPacketLoss selects a layer based on packet loss rate.
func (ls *LayerSelector) SelectLayerForPacketLoss(
	availableLayers []SVCLayer,
	preferredLayer, currentLayer SVCLayer,
	lossRate float64,
	bps uint64,
) SVCLayer {
	if len(availableLayers) == 0 {
		return SVCLayer{}
	}

	effectivePreferred := ls.SelectLayer(availableLayers, preferredLayer)
	currentPriority := ls.layerPriority(currentLayer)
	preferredPriority := ls.layerPriority(effectivePreferred)

	// High packet loss - switch to lower layer
	if lossRate >= ls.config.PacketLossThreshold {
		// Find the next lower layer
		var bestLayer SVCLayer
		bestPriority := -1
		for _, layer := range availableLayers {
			priority := ls.layerPriority(layer)
			if priority < currentPriority && priority > bestPriority {
				bestLayer = layer
				bestPriority = priority
			}
		}
		if bestPriority >= 0 {
			return bestLayer
		}
		// Already at lowest, stay there
		return currentLayer
	}

	// Low packet loss - consider recovery
	if lossRate < ls.config.PacketLossRecoveryThreshold {
		if currentPriority < preferredPriority {
			// Find the next higher available layer with sufficient bandwidth
			var candidates []SVCLayer
			for _, layer := range availableLayers {
				priority := ls.layerPriority(layer)
				if priority > currentPriority && priority <= preferredPriority {
					candidates = append(candidates, layer)
				}
			}

			// Sort candidates by priority (ascending)
			for i := 0; i < len(candidates); i++ {
				for j := i + 1; j < len(candidates); j++ {
					if ls.layerPriority(candidates[j]) < ls.layerPriority(candidates[i]) {
						candidates[i], candidates[j] = candidates[j], candidates[i]
					}
				}
			}

			// Try each candidate
			for _, c := range candidates {
				spec, ok := ls.GetLayerSpec(c)
				if !ok {
					continue
				}
				requiredBps := uint64(float64(spec.MaxBitrate) * ls.config.BandwidthMargin)
				if bps >= requiredBps {
					return c
				}
			}
		}
	}

	return currentLayer
}

// layerPriority returns the priority of a layer (higher is better).
// Priority = spatial * 10 + temporal.
func (ls *LayerSelector) layerPriority(layer SVCLayer) int {
	return layer.SpatialLayer*10 + layer.TemporalLayer
}

// IsLayerAvailable checks if a layer is in the available layers list.
func (ls *LayerSelector) IsLayerAvailable(availableLayers []SVCLayer, layer SVCLayer) bool {
	for _, l := range availableLayers {
		if l.SpatialLayer == layer.SpatialLayer && l.TemporalLayer == layer.TemporalLayer {
			return true
		}
	}
	return false
}

// SVCLayerToSimulcastLayer converts an SVC layer to the nearest simulcast layer equivalent.
// This is useful for compatibility with systems that expect simulcast layer notation.
func SVCLayerToSimulcastLayer(layer SVCLayer) media.SimulcastLayer {
	switch layer.SpatialLayer {
	case 2:
		return media.SimulcastLayerHigh
	case 1:
		return media.SimulcastLayerMedium
	case 0:
		return media.SimulcastLayerLow
	default:
		return media.SimulcastLayerLow
	}
}

// SimulcastLayerToSVCLayer converts a simulcast layer to an SVC layer.
// Uses the highest temporal layer for the corresponding spatial layer.
func SimulcastLayerToSVCLayer(layer media.SimulcastLayer) SVCLayer {
	switch layer {
	case media.SimulcastLayerHigh:
		return SVCLayer{SpatialLayer: 2, TemporalLayer: 2}
	case media.SimulcastLayerMedium:
		return SVCLayer{SpatialLayer: 1, TemporalLayer: 2}
	case media.SimulcastLayerLow:
		return SVCLayer{SpatialLayer: 0, TemporalLayer: 2}
	default:
		return SVCLayer{SpatialLayer: 0, TemporalLayer: 2}
	}
}
