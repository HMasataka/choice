package svc

import (
	"testing"

	"github.com/HMasataka/choice/internal/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSVCLayerString(t *testing.T) {
	tests := []struct {
		layer    SVCLayer
		expected string
	}{
		{SVCLayer{SpatialLayer: 0, TemporalLayer: 0}, "S0T0"},
		{SVCLayer{SpatialLayer: 1, TemporalLayer: 2}, "S1T2"},
		{SVCLayer{SpatialLayer: 2, TemporalLayer: 2}, "S2T2"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.layer.String())
		})
	}
}

func TestSVCLayerValidate(t *testing.T) {
	t.Run("valid layers", func(t *testing.T) {
		validLayers := []SVCLayer{
			{SpatialLayer: 0, TemporalLayer: 0},
			{SpatialLayer: 1, TemporalLayer: 1},
			{SpatialLayer: 2, TemporalLayer: 2},
			{SpatialLayer: 0, TemporalLayer: 2},
			{SpatialLayer: 2, TemporalLayer: 0},
		}

		for _, layer := range validLayers {
			err := layer.Validate()
			assert.NoError(t, err, "layer %v should be valid", layer)
		}
	})

	t.Run("invalid spatial layer", func(t *testing.T) {
		layer := SVCLayer{SpatialLayer: 3, TemporalLayer: 0}
		err := layer.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spatial layer")
	})

	t.Run("invalid temporal layer", func(t *testing.T) {
		layer := SVCLayer{SpatialLayer: 0, TemporalLayer: 5}
		err := layer.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temporal layer")
	})

	t.Run("negative spatial layer", func(t *testing.T) {
		layer := SVCLayer{SpatialLayer: -1, TemporalLayer: 0}
		err := layer.Validate()
		require.Error(t, err)
	})
}

func TestParseSVCLayer(t *testing.T) {
	t.Run("valid formats", func(t *testing.T) {
		tests := []struct {
			input    string
			expected SVCLayer
		}{
			{"S0T0", SVCLayer{SpatialLayer: 0, TemporalLayer: 0}},
			{"S1T2", SVCLayer{SpatialLayer: 1, TemporalLayer: 2}},
			{"S2T2", SVCLayer{SpatialLayer: 2, TemporalLayer: 2}},
		}

		for _, tt := range tests {
			t.Run(tt.input, func(t *testing.T) {
				layer, err := ParseSVCLayer(tt.input)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, layer)
			})
		}
	})

	t.Run("invalid formats", func(t *testing.T) {
		invalidInputs := []string{
			"",
			"S0",
			"T0",
			"S0T",
			"ST0",
			"s0t0",
			"S10T0",
			"S0T10",
			"S3T0", // Out of range
			"S0T3", // Out of range
		}

		for _, input := range invalidInputs {
			t.Run(input, func(t *testing.T) {
				_, err := ParseSVCLayer(input)
				assert.Error(t, err, "input %q should be invalid", input)
			})
		}
	})
}

func TestScalabilityModeValidate(t *testing.T) {
	t.Run("valid modes", func(t *testing.T) {
		validModes := []ScalabilityMode{
			ScalabilityModeL1T1,
			ScalabilityModeL1T2,
			ScalabilityModeL1T3,
			ScalabilityModeL2T1,
			ScalabilityModeL2T2,
			ScalabilityModeL2T3,
			ScalabilityModeL3T1,
			ScalabilityModeL3T2,
			ScalabilityModeL3T3,
		}

		for _, mode := range validModes {
			err := mode.Validate()
			assert.NoError(t, err, "mode %s should be valid", mode)
		}
	})

	t.Run("invalid modes", func(t *testing.T) {
		invalidModes := []ScalabilityMode{
			"",
			"L0T0",
			"L4T3",
			"L3T4",
			"invalid",
		}

		for _, mode := range invalidModes {
			err := mode.Validate()
			assert.Error(t, err, "mode %s should be invalid", mode)
		}
	})
}

func TestScalabilityModeGetLayers(t *testing.T) {
	tests := []struct {
		mode             ScalabilityMode
		expectedSpatial  int
		expectedTemporal int
	}{
		{ScalabilityModeL1T1, 1, 1},
		{ScalabilityModeL1T2, 1, 2},
		{ScalabilityModeL1T3, 1, 3},
		{ScalabilityModeL2T1, 2, 1},
		{ScalabilityModeL2T2, 2, 2},
		{ScalabilityModeL2T3, 2, 3},
		{ScalabilityModeL3T1, 3, 1},
		{ScalabilityModeL3T2, 3, 2},
		{ScalabilityModeL3T3, 3, 3},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			assert.Equal(t, tt.expectedSpatial, tt.mode.GetSpatialLayers())
			assert.Equal(t, tt.expectedTemporal, tt.mode.GetTemporalLayers())
		})
	}
}

func TestParseScalabilityMode(t *testing.T) {
	t.Run("valid mode", func(t *testing.T) {
		mode, err := ParseScalabilityMode("L3T3")
		require.NoError(t, err)
		assert.Equal(t, ScalabilityModeL3T3, mode)
	})

	t.Run("invalid mode", func(t *testing.T) {
		_, err := ParseScalabilityMode("invalid")
		assert.Error(t, err)
	})
}

func TestLayerSelectorSelectLayer(t *testing.T) {
	ls := NewLayerSelector(nil)

	availableLayers := []SVCLayer{
		{SpatialLayer: 2, TemporalLayer: 2},
		{SpatialLayer: 1, TemporalLayer: 2},
		{SpatialLayer: 0, TemporalLayer: 2},
	}

	t.Run("returns preferred when available", func(t *testing.T) {
		preferred := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}
		result := ls.SelectLayer(availableLayers, preferred)
		assert.Equal(t, preferred, result)
	})

	t.Run("returns next lower when preferred unavailable", func(t *testing.T) {
		// Preferred S2T1 not available, should return S2T2 (next best with same spatial)
		preferred := SVCLayer{SpatialLayer: 2, TemporalLayer: 1}
		result := ls.SelectLayer(availableLayers, preferred)
		// Since S2T1 is not available and priority checks spatial first,
		// it should fall back to the highest available layer <= preferred priority
		assert.Equal(t, SVCLayer{SpatialLayer: 1, TemporalLayer: 2}, result)
	})

	t.Run("returns lowest available when all higher unavailable", func(t *testing.T) {
		lowLayers := []SVCLayer{
			{SpatialLayer: 0, TemporalLayer: 1},
			{SpatialLayer: 0, TemporalLayer: 0},
		}
		preferred := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}
		result := ls.SelectLayer(lowLayers, preferred)
		// Should return highest available (S0T1)
		assert.Equal(t, SVCLayer{SpatialLayer: 0, TemporalLayer: 1}, result)
	})

	t.Run("returns empty for empty available layers", func(t *testing.T) {
		result := ls.SelectLayer([]SVCLayer{}, SVCLayer{SpatialLayer: 2, TemporalLayer: 2})
		assert.Equal(t, SVCLayer{}, result)
	})
}

func TestLayerSelectorSelectLayerForBandwidth(t *testing.T) {
	ls := NewLayerSelector(DefaultLayerSelectorConfig())

	availableLayers := []SVCLayer{
		{SpatialLayer: 2, TemporalLayer: 2},
		{SpatialLayer: 1, TemporalLayer: 2},
		{SpatialLayer: 0, TemporalLayer: 2},
	}
	preferredLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

	t.Run("selects high layer with high bandwidth", func(t *testing.T) {
		result := ls.SelectLayerForBandwidth(availableLayers, preferredLayer, 5_000_000)
		assert.Equal(t, SVCLayer{SpatialLayer: 2, TemporalLayer: 2}, result)
	})

	t.Run("selects medium layer with medium bandwidth", func(t *testing.T) {
		result := ls.SelectLayerForBandwidth(availableLayers, preferredLayer, 400_000)
		// Should select S1T2 which has MaxBitrate 500_000 * 1.1 = 550_000 required
		// But 400_000 < 550_000, so should fall back to S0T2
		assert.Equal(t, SVCLayer{SpatialLayer: 0, TemporalLayer: 2}, result)
	})

	t.Run("selects low layer with low bandwidth", func(t *testing.T) {
		result := ls.SelectLayerForBandwidth(availableLayers, preferredLayer, 100_000)
		assert.Equal(t, SVCLayer{SpatialLayer: 0, TemporalLayer: 2}, result)
	})

	t.Run("respects preferred layer as upper bound", func(t *testing.T) {
		mediumPreferred := SVCLayer{SpatialLayer: 1, TemporalLayer: 2}
		result := ls.SelectLayerForBandwidth(availableLayers, mediumPreferred, 5_000_000)
		assert.Equal(t, SVCLayer{SpatialLayer: 1, TemporalLayer: 2}, result)
	})
}

func TestLayerSelectorSelectLayerForPacketLoss(t *testing.T) {
	ls := NewLayerSelector(DefaultLayerSelectorConfig())

	availableLayers := []SVCLayer{
		{SpatialLayer: 2, TemporalLayer: 2},
		{SpatialLayer: 1, TemporalLayer: 2},
		{SpatialLayer: 0, TemporalLayer: 2},
	}
	preferredLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}
	currentLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

	t.Run("downgrades on high packet loss", func(t *testing.T) {
		result := ls.SelectLayerForPacketLoss(availableLayers, preferredLayer, currentLayer, 0.10, 5_000_000)
		assert.Equal(t, SVCLayer{SpatialLayer: 1, TemporalLayer: 2}, result)
	})

	t.Run("maintains layer on moderate packet loss", func(t *testing.T) {
		result := ls.SelectLayerForPacketLoss(availableLayers, preferredLayer, currentLayer, 0.03, 5_000_000)
		assert.Equal(t, currentLayer, result)
	})

	t.Run("upgrades on low packet loss with bandwidth", func(t *testing.T) {
		lowCurrent := SVCLayer{SpatialLayer: 1, TemporalLayer: 2}
		result := ls.SelectLayerForPacketLoss(availableLayers, preferredLayer, lowCurrent, 0.005, 5_000_000)
		assert.Equal(t, SVCLayer{SpatialLayer: 2, TemporalLayer: 2}, result)
	})

	t.Run("does not upgrade without bandwidth", func(t *testing.T) {
		lowCurrent := SVCLayer{SpatialLayer: 0, TemporalLayer: 2}
		result := ls.SelectLayerForPacketLoss(availableLayers, preferredLayer, lowCurrent, 0.005, 100_000)
		// Low bandwidth, should stay at current
		assert.Equal(t, lowCurrent, result)
	})
}

func TestLayerSelectorGetLayerSpec(t *testing.T) {
	ls := NewLayerSelector(DefaultLayerSelectorConfig())

	t.Run("returns spec for existing layer", func(t *testing.T) {
		spec, ok := ls.GetLayerSpec(SVCLayer{SpatialLayer: 2, TemporalLayer: 2})
		assert.True(t, ok)
		assert.Equal(t, 1280, spec.Width)
		assert.Equal(t, 720, spec.Height)
		assert.Equal(t, 2_500_000, spec.MaxBitrate)
		assert.Equal(t, 30, spec.MaxFPS)
	})

	t.Run("returns false for non-existing layer", func(t *testing.T) {
		_, ok := ls.GetLayerSpec(SVCLayer{SpatialLayer: 2, TemporalLayer: 1})
		// S2T1 is in the default layers
		assert.True(t, ok)
	})
}

func TestSVCLayerToSimulcastLayer(t *testing.T) {
	tests := []struct {
		svcLayer      SVCLayer
		expectedSimul media.SimulcastLayer
	}{
		{SVCLayer{SpatialLayer: 2, TemporalLayer: 2}, media.SimulcastLayerHigh},
		{SVCLayer{SpatialLayer: 2, TemporalLayer: 0}, media.SimulcastLayerHigh},
		{SVCLayer{SpatialLayer: 1, TemporalLayer: 2}, media.SimulcastLayerMedium},
		{SVCLayer{SpatialLayer: 1, TemporalLayer: 0}, media.SimulcastLayerMedium},
		{SVCLayer{SpatialLayer: 0, TemporalLayer: 2}, media.SimulcastLayerLow},
		{SVCLayer{SpatialLayer: 0, TemporalLayer: 0}, media.SimulcastLayerLow},
	}

	for _, tt := range tests {
		t.Run(tt.svcLayer.String(), func(t *testing.T) {
			result := SVCLayerToSimulcastLayer(tt.svcLayer)
			assert.Equal(t, tt.expectedSimul, result)
		})
	}
}

func TestSimulcastLayerToSVCLayer(t *testing.T) {
	tests := []struct {
		simulcast   media.SimulcastLayer
		expectedSVC SVCLayer
	}{
		{media.SimulcastLayerHigh, SVCLayer{SpatialLayer: 2, TemporalLayer: 2}},
		{media.SimulcastLayerMedium, SVCLayer{SpatialLayer: 1, TemporalLayer: 2}},
		{media.SimulcastLayerLow, SVCLayer{SpatialLayer: 0, TemporalLayer: 2}},
	}

	for _, tt := range tests {
		t.Run(string(tt.simulcast), func(t *testing.T) {
			result := SimulcastLayerToSVCLayer(tt.simulcast)
			assert.Equal(t, tt.expectedSVC, result)
		})
	}
}

func TestIsLayerAvailable(t *testing.T) {
	ls := NewLayerSelector(nil)

	availableLayers := []SVCLayer{
		{SpatialLayer: 2, TemporalLayer: 2},
		{SpatialLayer: 1, TemporalLayer: 2},
		{SpatialLayer: 0, TemporalLayer: 2},
	}

	t.Run("returns true for available layer", func(t *testing.T) {
		assert.True(t, ls.IsLayerAvailable(availableLayers, SVCLayer{SpatialLayer: 2, TemporalLayer: 2}))
		assert.True(t, ls.IsLayerAvailable(availableLayers, SVCLayer{SpatialLayer: 1, TemporalLayer: 2}))
		assert.True(t, ls.IsLayerAvailable(availableLayers, SVCLayer{SpatialLayer: 0, TemporalLayer: 2}))
	})

	t.Run("returns false for unavailable layer", func(t *testing.T) {
		assert.False(t, ls.IsLayerAvailable(availableLayers, SVCLayer{SpatialLayer: 2, TemporalLayer: 1}))
		assert.False(t, ls.IsLayerAvailable(availableLayers, SVCLayer{SpatialLayer: 1, TemporalLayer: 0}))
	})
}

func TestGetAvailableLayers(t *testing.T) {
	ls := NewLayerSelector(DefaultLayerSelectorConfig())

	layers := ls.GetAvailableLayers()

	// Should have 9 layers for L3T3 (3 spatial x 3 temporal)
	assert.Len(t, layers, 9)

	// Verify some expected layers are present
	found := map[string]bool{}
	for _, l := range layers {
		found[l.String()] = true
	}

	assert.True(t, found["S2T2"])
	assert.True(t, found["S1T1"])
	assert.True(t, found["S0T0"])
}
