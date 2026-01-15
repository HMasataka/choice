package simulcast

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/HMasataka/choice/internal/media"
)

func TestLayerSelector_SelectLayer_PreferredAvailable(t *testing.T) {
	ls := NewLayerSelector(nil)

	tests := []struct {
		name            string
		availableLayers []media.SimulcastLayer
		preferredLayer  media.SimulcastLayer
		expectedLayer   media.SimulcastLayer
	}{
		{
			name:            "high preferred and available",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			expectedLayer:   media.SimulcastLayerHigh,
		},
		{
			name:            "medium preferred and available",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerMedium,
			expectedLayer:   media.SimulcastLayerMedium,
		},
		{
			name:            "low preferred and available",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerLow,
			expectedLayer:   media.SimulcastLayerLow,
		},
		{
			name:            "only high available",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh},
			preferredLayer:  media.SimulcastLayerHigh,
			expectedLayer:   media.SimulcastLayerHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ls.SelectLayer(tt.availableLayers, tt.preferredLayer)
			assert.Equal(t, tt.expectedLayer, result)
		})
	}
}

func TestLayerSelector_SelectLayer_Fallback(t *testing.T) {
	ls := NewLayerSelector(nil)

	tests := []struct {
		name            string
		availableLayers []media.SimulcastLayer
		preferredLayer  media.SimulcastLayer
		expectedLayer   media.SimulcastLayer
	}{
		{
			name:            "high preferred but only medium/low available - fallback to medium",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			expectedLayer:   media.SimulcastLayerMedium,
		},
		{
			name:            "high preferred but only low available - fallback to low",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			expectedLayer:   media.SimulcastLayerLow,
		},
		{
			name:            "medium preferred but only low available - fallback to low",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerMedium,
			expectedLayer:   media.SimulcastLayerLow,
		},
		{
			name:            "low preferred but only high available - use high (only option)",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh},
			preferredLayer:  media.SimulcastLayerLow,
			expectedLayer:   media.SimulcastLayerHigh,
		},
		{
			name:            "low preferred but only medium/high available - use medium (closest)",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium},
			preferredLayer:  media.SimulcastLayerLow,
			expectedLayer:   media.SimulcastLayerMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ls.SelectLayer(tt.availableLayers, tt.preferredLayer)
			assert.Equal(t, tt.expectedLayer, result)
		})
	}
}

func TestLayerSelector_SelectLayer_EmptyLayers(t *testing.T) {
	ls := NewLayerSelector(nil)

	result := ls.SelectLayer([]media.SimulcastLayer{}, media.SimulcastLayerHigh)
	assert.Equal(t, media.SimulcastLayer(""), result)
}

func TestLayerSelector_SelectLayerForBandwidth(t *testing.T) {
	ls := NewLayerSelector(nil)

	tests := []struct {
		name            string
		availableLayers []media.SimulcastLayer
		preferredLayer  media.SimulcastLayer
		bps             uint64
		expectedLayer   media.SimulcastLayer
	}{
		{
			name:            "plenty of bandwidth for high",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			bps:             3_000_000, // 3Mbps
			expectedLayer:   media.SimulcastLayerHigh,
		},
		{
			name:            "bandwidth for medium only",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			bps:             600_000, // 600Kbps - enough for medium (500Kbps * 1.1 = 550Kbps)
			expectedLayer:   media.SimulcastLayerMedium,
		},
		{
			name:            "bandwidth for low only",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			bps:             200_000, // 200Kbps - enough for low (150Kbps * 1.1 = 165Kbps)
			expectedLayer:   media.SimulcastLayerLow,
		},
		{
			name:            "very low bandwidth - fallback to lowest",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			bps:             50_000, // 50Kbps - not enough for any
			expectedLayer:   media.SimulcastLayerLow, // Still returns low as fallback
		},
		{
			name:            "preferred is medium - respects upper bound",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerMedium,
			bps:             3_000_000, // Plenty of bandwidth
			expectedLayer:   media.SimulcastLayerMedium, // But respects preferred
		},
		{
			name:            "preferred is low - stays at low even with high bandwidth",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerLow,
			bps:             3_000_000, // Plenty of bandwidth
			expectedLayer:   media.SimulcastLayerLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ls.SelectLayerForBandwidth(tt.availableLayers, tt.preferredLayer, tt.bps)
			assert.Equal(t, tt.expectedLayer, result)
		})
	}
}

func TestLayerSelector_SelectLayerForPacketLoss(t *testing.T) {
	ls := NewLayerSelector(nil)
	// High bandwidth to allow recovery
	highBandwidth := uint64(5_000_000) // 5Mbps

	tests := []struct {
		name            string
		availableLayers []media.SimulcastLayer
		preferredLayer  media.SimulcastLayer
		currentLayer    media.SimulcastLayer
		lossRate        float64
		bps             uint64
		expectedLayer   media.SimulcastLayer
	}{
		{
			name:            "high loss - drop from high to medium",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			currentLayer:    media.SimulcastLayerHigh,
			lossRate:        0.06, // 6%
			bps:             highBandwidth,
			expectedLayer:   media.SimulcastLayerMedium,
		},
		{
			name:            "high loss - drop from medium to low",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			currentLayer:    media.SimulcastLayerMedium,
			lossRate:        0.10, // 10%
			bps:             highBandwidth,
			expectedLayer:   media.SimulcastLayerLow,
		},
		{
			name:            "high loss at low - stay at low",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			currentLayer:    media.SimulcastLayerLow,
			lossRate:        0.15, // 15%
			bps:             highBandwidth,
			expectedLayer:   media.SimulcastLayerLow,
		},
		{
			name:            "low loss with high bandwidth - recover from low to medium",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			currentLayer:    media.SimulcastLayerLow,
			lossRate:        0.005, // 0.5%
			bps:             highBandwidth,
			expectedLayer:   media.SimulcastLayerMedium,
		},
		{
			name:            "low loss with high bandwidth - recover from medium to high",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			currentLayer:    media.SimulcastLayerMedium,
			lossRate:        0.005, // 0.5%
			bps:             highBandwidth,
			expectedLayer:   media.SimulcastLayerHigh,
		},
		{
			name:            "low loss but already at preferred - no change",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			currentLayer:    media.SimulcastLayerHigh,
			lossRate:        0.005, // 0.5%
			bps:             highBandwidth,
			expectedLayer:   media.SimulcastLayerHigh,
		},
		{
			name:            "low loss but preferred is medium - recover only to medium",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerMedium,
			currentLayer:    media.SimulcastLayerLow,
			lossRate:        0.005, // 0.5%
			bps:             highBandwidth,
			expectedLayer:   media.SimulcastLayerMedium,
		},
		{
			name:            "moderate loss - no change",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			currentLayer:    media.SimulcastLayerMedium,
			lossRate:        0.03, // 3% - between thresholds
			bps:             highBandwidth,
			expectedLayer:   media.SimulcastLayerMedium,
		},
		{
			name:            "low loss but insufficient bandwidth - no recovery",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			currentLayer:    media.SimulcastLayerLow,
			lossRate:        0.005, // 0.5%
			bps:             100_000, // 100Kbps - not enough for medium
			expectedLayer:   media.SimulcastLayerLow,
		},
		{
			name:            "low loss with medium bandwidth - recover to medium only",
			availableLayers: []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow},
			preferredLayer:  media.SimulcastLayerHigh,
			currentLayer:    media.SimulcastLayerLow,
			lossRate:        0.005, // 0.5%
			bps:             600_000, // 600Kbps - enough for medium but not high
			expectedLayer:   media.SimulcastLayerMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ls.SelectLayerForPacketLoss(tt.availableLayers, tt.preferredLayer, tt.currentLayer, tt.lossRate, tt.bps)
			assert.Equal(t, tt.expectedLayer, result)
		})
	}
}

func TestLayerSelector_SelectLayerForPacketLoss_LimitedLayers(t *testing.T) {
	ls := NewLayerSelector(nil)
	highBandwidth := uint64(5_000_000) // 5Mbps

	// Only medium and low available
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerMedium, media.SimulcastLayerLow}

	// High loss at medium, high not available - drop to low
	result := ls.SelectLayerForPacketLoss(availableLayers, media.SimulcastLayerHigh, media.SimulcastLayerMedium, 0.06, highBandwidth)
	assert.Equal(t, media.SimulcastLayerLow, result)

	// Low loss at low with bandwidth, can recover to medium but not high
	result = ls.SelectLayerForPacketLoss(availableLayers, media.SimulcastLayerHigh, media.SimulcastLayerLow, 0.005, highBandwidth)
	assert.Equal(t, media.SimulcastLayerMedium, result)
}

func TestLayerSelector_SelectLayerForPacketLoss_SkipMissingLayer(t *testing.T) {
	ls := NewLayerSelector(nil)
	highBandwidth := uint64(5_000_000) // 5Mbps

	// Only high and low available (medium missing)
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerLow}

	// Low loss at low with bandwidth - should skip to high since medium is unavailable
	result := ls.SelectLayerForPacketLoss(availableLayers, media.SimulcastLayerHigh, media.SimulcastLayerLow, 0.005, highBandwidth)
	assert.Equal(t, media.SimulcastLayerHigh, result)
}

func TestParseRID(t *testing.T) {
	tests := []struct {
		rid      string
		expected media.SimulcastLayer
	}{
		{"h", media.SimulcastLayerHigh},
		{"m", media.SimulcastLayerMedium},
		{"l", media.SimulcastLayerLow},
		{"H", ""}, // Case sensitive
		{"high", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.rid, func(t *testing.T) {
			result := ParseRID(tt.rid)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetRID(t *testing.T) {
	tests := []struct {
		layer    media.SimulcastLayer
		expected string
	}{
		{media.SimulcastLayerHigh, "h"},
		{media.SimulcastLayerMedium, "m"},
		{media.SimulcastLayerLow, "l"},
		{media.SimulcastLayer("invalid"), ""},
		{media.SimulcastLayer(""), ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.layer), func(t *testing.T) {
			result := GetRID(tt.layer)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultLayers(t *testing.T) {
	// Verify default layer specs match ADR-0003
	assert.Equal(t, LayerSpec{RID: "h", Width: 1280, Height: 720, MaxBitrate: 2_500_000, MaxFPS: 30}, DefaultLayers[media.SimulcastLayerHigh])
	assert.Equal(t, LayerSpec{RID: "m", Width: 640, Height: 360, MaxBitrate: 500_000, MaxFPS: 30}, DefaultLayers[media.SimulcastLayerMedium])
	assert.Equal(t, LayerSpec{RID: "l", Width: 320, Height: 180, MaxBitrate: 150_000, MaxFPS: 15}, DefaultLayers[media.SimulcastLayerLow])
}

func TestLayerSelector_GetLayerSpec(t *testing.T) {
	ls := NewLayerSelector(nil)

	spec, ok := ls.GetLayerSpec(media.SimulcastLayerHigh)
	assert.True(t, ok)
	assert.Equal(t, 1280, spec.Width)
	assert.Equal(t, 720, spec.Height)
	assert.Equal(t, 2_500_000, spec.MaxBitrate)

	_, ok = ls.GetLayerSpec(media.SimulcastLayer("invalid"))
	assert.False(t, ok)
}

func TestLayerSelector_CustomConfig(t *testing.T) {
	customLayers := map[media.SimulcastLayer]LayerSpec{
		media.SimulcastLayerHigh: {RID: "h", Width: 1920, Height: 1080, MaxBitrate: 5_000_000, MaxFPS: 30},
		media.SimulcastLayerLow:  {RID: "l", Width: 640, Height: 360, MaxBitrate: 300_000, MaxFPS: 15},
	}

	ls := NewLayerSelector(&LayerSelectorConfig{
		PacketLossThreshold:         0.10,
		PacketLossRecoveryThreshold: 0.02,
		BandwidthMargin:             1.2,
		Layers:                      customLayers,
	})

	// With custom config, high requires more bandwidth
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerLow}

	// 5.5Mbps should not be enough for high (5Mbps * 1.2 = 6Mbps required)
	result := ls.SelectLayerForBandwidth(availableLayers, media.SimulcastLayerHigh, 5_500_000)
	assert.Equal(t, media.SimulcastLayerLow, result)

	// 6.5Mbps should be enough
	result = ls.SelectLayerForBandwidth(availableLayers, media.SimulcastLayerHigh, 6_500_000)
	assert.Equal(t, media.SimulcastLayerHigh, result)
}
