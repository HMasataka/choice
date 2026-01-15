package simulcast

import (
	"github.com/HMasataka/choice/internal/media"
)

// LayerSpec defines simulcast layer specifications.
// Per ADR-0003 and design.md section 3.5.2.
type LayerSpec struct {
	RID        string // RTP Stream ID (h, m, l)
	Width      int    // Resolution width
	Height     int    // Resolution height
	MaxBitrate int    // Maximum bitrate in bps
	MaxFPS     int    // Maximum frames per second
}

// DefaultLayers contains the default layer specifications.
// Per ADR-0003:
// - High (h): 1280x720, 2.5Mbps, 30fps
// - Medium (m): 640x360, 500Kbps, 30fps
// - Low (l): 320x180, 150Kbps, 15fps
var DefaultLayers = map[media.SimulcastLayer]LayerSpec{
	media.SimulcastLayerHigh:   {RID: "h", Width: 1280, Height: 720, MaxBitrate: 2_500_000, MaxFPS: 30},
	media.SimulcastLayerMedium: {RID: "m", Width: 640, Height: 360, MaxBitrate: 500_000, MaxFPS: 30},
	media.SimulcastLayerLow:    {RID: "l", Width: 320, Height: 180, MaxBitrate: 150_000, MaxFPS: 15},
}

// LayerSelectorConfig contains configuration for layer selection logic.
type LayerSelectorConfig struct {
	// PacketLossThreshold is the packet loss rate threshold for switching to lower layer.
	// Per design.md: 5% packet loss triggers low layer switch.
	PacketLossThreshold float64

	// PacketLossRecoveryThreshold is the threshold for returning to higher layer.
	// Per design.md: <1% packet loss with sufficient bandwidth allows recovery.
	PacketLossRecoveryThreshold float64

	// BandwidthMargin is the multiplier for required bandwidth.
	// A layer requires bitrate * BandwidthMargin to be selected.
	// This provides headroom for fluctuations.
	BandwidthMargin float64

	// Layers contains the layer specifications to use.
	Layers map[media.SimulcastLayer]LayerSpec
}

// DefaultLayerSelectorConfig returns the default configuration.
func DefaultLayerSelectorConfig() *LayerSelectorConfig {
	return &LayerSelectorConfig{
		PacketLossThreshold:         0.05, // 5%
		PacketLossRecoveryThreshold: 0.01, // 1%
		BandwidthMargin:             1.1,  // 10% margin
		Layers:                      DefaultLayers,
	}
}

// LayerSelector handles the logic for selecting simulcast layers.
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

// SelectLayer selects the best available layer based on preference.
// If the preferred layer is not available, selects the next best available layer.
// Per design.md: If requested layer doesn't exist, select next closest layer.
func (ls *LayerSelector) SelectLayer(availableLayers []media.SimulcastLayer, preferredLayer media.SimulcastLayer) media.SimulcastLayer {
	if len(availableLayers) == 0 {
		return ""
	}

	// Check if preferred layer is available
	for _, layer := range availableLayers {
		if layer == preferredLayer {
			return preferredLayer
		}
	}

	// Preferred layer not available, find the next best
	// Priority: h > m > l
	preferredPriority := layerPriority(preferredLayer)

	// Find the highest priority layer that is <= preferred priority
	var bestLayer media.SimulcastLayer
	bestPriority := -1
	for _, layer := range availableLayers {
		priority := layerPriority(layer)
		if priority <= preferredPriority && priority > bestPriority {
			bestLayer = layer
			bestPriority = priority
		}
	}

	// If no lower layer found, pick the lowest available layer
	if bestLayer == "" {
		lowestPriority := 999
		for _, layer := range availableLayers {
			priority := layerPriority(layer)
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
// Per design.md: Bandwidth shortage triggers automatic switch to lower layer.
func (ls *LayerSelector) SelectLayerForBandwidth(availableLayers []media.SimulcastLayer, preferredLayer media.SimulcastLayer, bps uint64) media.SimulcastLayer {
	if len(availableLayers) == 0 {
		return ""
	}

	// Get the effective preferred layer (what we'd select without bandwidth constraints)
	effectivePreferred := ls.SelectLayer(availableLayers, preferredLayer)
	if effectivePreferred == "" {
		return ""
	}

	// Check if we can afford the effective preferred layer
	preferredSpec, ok := ls.config.Layers[effectivePreferred]
	if !ok {
		// Layer not in config, cannot determine bandwidth requirement
		// Fall through to find an affordable layer
	} else {
		requiredBps := uint64(float64(preferredSpec.MaxBitrate) * ls.config.BandwidthMargin)
		if bps >= requiredBps {
			return effectivePreferred
		}
	}

	// Cannot afford preferred layer, find the highest layer we can afford
	preferredPriority := layerPriority(effectivePreferred)
	var bestLayer media.SimulcastLayer
	bestPriority := -1

	for _, layer := range availableLayers {
		priority := layerPriority(layer)
		// Only consider layers at or below preferred
		if priority > preferredPriority {
			continue
		}

		spec, ok := ls.config.Layers[layer]
		if !ok {
			// Layer not in config, skip it
			continue
		}
		requiredBps := uint64(float64(spec.MaxBitrate) * ls.config.BandwidthMargin)
		if bps >= requiredBps && priority > bestPriority {
			bestLayer = layer
			bestPriority = priority
		}
	}

	// If no layer can be afforded, return the lowest available layer
	// as a fallback (better to have something than nothing)
	if bestLayer == "" {
		lowestPriority := 999
		for _, layer := range availableLayers {
			priority := layerPriority(layer)
			if priority <= preferredPriority && priority < lowestPriority {
				bestLayer = layer
				lowestPriority = priority
			}
		}
	}

	return bestLayer
}

// SelectLayerForPacketLoss selects a layer based on packet loss rate.
// It considers the current layer and may upgrade or downgrade based on loss rate.
// Per design.md:
// - >5% packet loss: switch to lower layer
// - <1% packet loss with bandwidth: recover to higher layer
// Note: bps parameter is used to gate recovery - recovery only happens if
// bandwidth is sufficient for the target layer.
func (ls *LayerSelector) SelectLayerForPacketLoss(availableLayers []media.SimulcastLayer, preferredLayer media.SimulcastLayer, currentLayer media.SimulcastLayer, lossRate float64, bps uint64) media.SimulcastLayer {
	if len(availableLayers) == 0 {
		return ""
	}

	effectivePreferred := ls.SelectLayer(availableLayers, preferredLayer)
	if effectivePreferred == "" {
		return currentLayer
	}

	currentPriority := layerPriority(currentLayer)
	preferredPriority := layerPriority(effectivePreferred)

	// High packet loss - switch to lower layer
	if lossRate >= ls.config.PacketLossThreshold {
		// Find the next lower layer
		for _, layer := range []media.SimulcastLayer{media.SimulcastLayerMedium, media.SimulcastLayerLow} {
			if layerPriority(layer) < currentPriority && ls.isLayerAvailable(availableLayers, layer) {
				return layer
			}
		}
		// Already at lowest, stay there
		return currentLayer
	}

	// Low packet loss - consider recovery (requires sufficient bandwidth)
	// Per design.md: "<1% packet loss with sufficient bandwidth allows recovery"
	if lossRate < ls.config.PacketLossRecoveryThreshold {
		// Try to move to the next higher available layer (up to preferred)
		if currentPriority < preferredPriority {
			// Find the next available layer above current that has a spec and sufficient bandwidth
			// This allows skipping missing intermediate layers (e.g., l -> h when m is unavailable)
			// and also skipping layers without spec definitions
			type candidate struct {
				layer    media.SimulcastLayer
				priority int
			}
			var candidates []candidate
			for _, layer := range availableLayers {
				priority := layerPriority(layer)
				// Layer must be above current and at or below preferred
				if priority > currentPriority && priority <= preferredPriority {
					candidates = append(candidates, candidate{layer: layer, priority: priority})
				}
			}

			// Sort candidates by priority ascending (lowest priority first = smallest step up)
			// Using simple bubble sort for small slice
			for i := 0; i < len(candidates); i++ {
				for j := i + 1; j < len(candidates); j++ {
					if candidates[j].priority < candidates[i].priority {
						candidates[i], candidates[j] = candidates[j], candidates[i]
					}
				}
			}

			// Try each candidate in order until we find one with spec and bandwidth
			for _, c := range candidates {
				spec, ok := ls.config.Layers[c.layer]
				if !ok {
					// Skip layers without spec definition
					continue
				}
				requiredBps := uint64(float64(spec.MaxBitrate) * ls.config.BandwidthMargin)
				if bps >= requiredBps {
					return c.layer
				}
				// Insufficient bandwidth for this layer, try next
			}
			// No suitable layer found, don't upgrade
		}
	}

	// No change needed
	return currentLayer
}

// isLayerAvailable checks if a layer is in the available layers list.
func (ls *LayerSelector) isLayerAvailable(availableLayers []media.SimulcastLayer, layer media.SimulcastLayer) bool {
	for _, l := range availableLayers {
		if l == layer {
			return true
		}
	}
	return false
}

// GetLayerSpec returns the specification for a given layer.
func (ls *LayerSelector) GetLayerSpec(layer media.SimulcastLayer) (LayerSpec, bool) {
	spec, ok := ls.config.Layers[layer]
	return spec, ok
}

// ParseRID parses an RID string to a SimulcastLayer.
// Returns empty string if the RID is not recognized.
func ParseRID(rid string) media.SimulcastLayer {
	switch rid {
	case "h":
		return media.SimulcastLayerHigh
	case "m":
		return media.SimulcastLayerMedium
	case "l":
		return media.SimulcastLayerLow
	default:
		return ""
	}
}

// GetRID returns the RID string for a given layer.
func GetRID(layer media.SimulcastLayer) string {
	switch layer {
	case media.SimulcastLayerHigh:
		return "h"
	case media.SimulcastLayerMedium:
		return "m"
	case media.SimulcastLayerLow:
		return "l"
	default:
		return ""
	}
}
