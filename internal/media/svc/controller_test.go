package svc

import (
	"context"
	"testing"

	"github.com/HMasataka/choice/internal/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewController(t *testing.T) {
	t.Run("creates controller with default config", func(t *testing.T) {
		ctrl := NewController(nil)
		assert.NotNil(t, ctrl)
	})

	t.Run("creates controller with custom config", func(t *testing.T) {
		cfg := &ControllerConfig{
			LayerSelectorConfig: &LayerSelectorConfig{
				PacketLossThreshold:         0.10,
				PacketLossRecoveryThreshold: 0.02,
				BandwidthMargin:             1.2,
			},
		}
		ctrl := NewController(cfg)
		assert.NotNil(t, ctrl)
	})
}

func TestControllerRegisterSubscription(t *testing.T) {
	ctrl := NewController(nil)

	subID := media.GenerateSubscriptionID()
	trackID := media.GenerateTrackID()
	availableLayers := []SVCLayer{
		{SpatialLayer: 2, TemporalLayer: 2},
		{SpatialLayer: 1, TemporalLayer: 2},
		{SpatialLayer: 0, TemporalLayer: 2},
	}
	preferredLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

	t.Run("registers subscription successfully", func(t *testing.T) {
		err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, preferredLayer)
		require.NoError(t, err)

		state, err := ctrl.GetSubscriptionState(subID)
		require.NoError(t, err)
		assert.Equal(t, subID, state.SubscriptionID)
		assert.Equal(t, "subscriber1", state.SubscriberID)
		assert.Equal(t, trackID, state.TrackID)
		assert.Equal(t, preferredLayer, state.PreferredLayer)
		assert.Equal(t, preferredLayer, state.ActualLayer)
	})

	t.Run("rejects duplicate registration", func(t *testing.T) {
		err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, preferredLayer)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("rejects empty subscriber ID", func(t *testing.T) {
		newSubID := media.GenerateSubscriptionID()
		err := ctrl.RegisterSubscription(newSubID, "", trackID, availableLayers, preferredLayer)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subscriber ID cannot be empty")
	})

	t.Run("rejects empty available layers", func(t *testing.T) {
		newSubID := media.GenerateSubscriptionID()
		err := ctrl.RegisterSubscription(newSubID, "subscriber1", trackID, []SVCLayer{}, preferredLayer)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "available layers cannot be empty")
	})

	t.Run("rejects invalid preferred layer", func(t *testing.T) {
		newSubID := media.GenerateSubscriptionID()
		invalidLayer := SVCLayer{SpatialLayer: 5, TemporalLayer: 0}
		err := ctrl.RegisterSubscription(newSubID, "subscriber1", trackID, availableLayers, invalidLayer)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid preferred layer")
	})

	// Cleanup
	_ = ctrl.UnregisterSubscription(subID)
}

func TestControllerSetPreferredLayer(t *testing.T) {
	ctx := context.Background()

	t.Run("sets preferred layer successfully", func(t *testing.T) {
		var layerChangeResults []LayerChangeResult
		cfg := &ControllerConfig{
			OnLayerChange: func(result LayerChangeResult) {
				layerChangeResults = append(layerChangeResults, result)
			},
		}
		ctrl := NewController(cfg)

		subID := media.GenerateSubscriptionID()
		trackID := media.GenerateTrackID()
		availableLayers := []SVCLayer{
			{SpatialLayer: 2, TemporalLayer: 2},
			{SpatialLayer: 1, TemporalLayer: 2},
			{SpatialLayer: 0, TemporalLayer: 2},
		}
		initialLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

		err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, initialLayer)
		require.NoError(t, err)

		newLayer := SVCLayer{SpatialLayer: 1, TemporalLayer: 2}
		err = ctrl.SetPreferredLayer(ctx, subID, newLayer)
		require.NoError(t, err)

		preferred, err := ctrl.GetPreferredLayer(subID)
		require.NoError(t, err)
		assert.Equal(t, newLayer, preferred)

		current, err := ctrl.GetCurrentLayer(subID)
		require.NoError(t, err)
		assert.Equal(t, newLayer, current)

		// Check callback was invoked
		require.Len(t, layerChangeResults, 1)
		assert.True(t, layerChangeResults[0].Changed)
		assert.Equal(t, initialLayer, layerChangeResults[0].PreviousLayer)
		assert.Equal(t, newLayer, layerChangeResults[0].CurrentLayer)
		assert.Equal(t, LayerChangeReasonRequested, layerChangeResults[0].Reason)
	})

	t.Run("falls back to available layer when preferred is unavailable", func(t *testing.T) {
		var layerChangeResults []LayerChangeResult
		cfg := &ControllerConfig{
			OnLayerChange: func(result LayerChangeResult) {
				layerChangeResults = append(layerChangeResults, result)
			},
		}
		ctrl := NewController(cfg)

		subID := media.GenerateSubscriptionID()
		trackID := media.GenerateTrackID()
		// Only low and medium layers available
		availableLayers := []SVCLayer{
			{SpatialLayer: 1, TemporalLayer: 2},
			{SpatialLayer: 0, TemporalLayer: 2},
		}
		initialLayer := SVCLayer{SpatialLayer: 1, TemporalLayer: 2}

		err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, initialLayer)
		require.NoError(t, err)

		// Try to set high layer which is not available
		highLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}
		err = ctrl.SetPreferredLayer(ctx, subID, highLayer)
		require.NoError(t, err)

		preferred, err := ctrl.GetPreferredLayer(subID)
		require.NoError(t, err)
		assert.Equal(t, highLayer, preferred)

		// Actual layer should be the best available (medium)
		current, err := ctrl.GetCurrentLayer(subID)
		require.NoError(t, err)
		assert.Equal(t, SVCLayer{SpatialLayer: 1, TemporalLayer: 2}, current)

		// Check callback
		require.Len(t, layerChangeResults, 0) // No change because current was already medium
	})
}

func TestControllerUnregisterSubscription(t *testing.T) {
	ctrl := NewController(nil)

	subID := media.GenerateSubscriptionID()
	trackID := media.GenerateTrackID()
	availableLayers := []SVCLayer{
		{SpatialLayer: 2, TemporalLayer: 2},
		{SpatialLayer: 1, TemporalLayer: 2},
	}
	preferredLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

	t.Run("unregisters subscription successfully", func(t *testing.T) {
		err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, preferredLayer)
		require.NoError(t, err)

		err = ctrl.UnregisterSubscription(subID)
		require.NoError(t, err)

		_, err = ctrl.GetSubscriptionState(subID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("returns error for non-existent subscription", func(t *testing.T) {
		nonExistentID := media.GenerateSubscriptionID()
		err := ctrl.UnregisterSubscription(nonExistentID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestControllerOnBandwidthEstimate(t *testing.T) {
	ctx := context.Background()

	t.Run("downgrades layer on low bandwidth", func(t *testing.T) {
		var layerChangeResults []LayerChangeResult
		cfg := &ControllerConfig{
			OnLayerChange: func(result LayerChangeResult) {
				layerChangeResults = append(layerChangeResults, result)
			},
			LayerSelectorConfig: DefaultLayerSelectorConfig(),
		}
		ctrl := NewController(cfg)

		subID := media.GenerateSubscriptionID()
		trackID := media.GenerateTrackID()
		availableLayers := []SVCLayer{
			{SpatialLayer: 2, TemporalLayer: 2},
			{SpatialLayer: 1, TemporalLayer: 2},
			{SpatialLayer: 0, TemporalLayer: 2},
		}
		preferredLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

		err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, preferredLayer)
		require.NoError(t, err)

		// Simulate low bandwidth (below medium layer requirement)
		results := ctrl.OnBandwidthEstimate(ctx, "subscriber1", 100_000) // 100 Kbps

		// Should have downgraded to lowest layer
		require.Len(t, results, 1)
		assert.True(t, results[0].Changed)
		assert.Equal(t, LayerChangeReasonBandwidth, results[0].Reason)
		assert.Equal(t, SVCLayer{SpatialLayer: 0, TemporalLayer: 2}, results[0].CurrentLayer)
	})

	t.Run("maintains layer when bandwidth is sufficient", func(t *testing.T) {
		ctrl := NewController(nil)

		subID := media.GenerateSubscriptionID()
		trackID := media.GenerateTrackID()
		availableLayers := []SVCLayer{
			{SpatialLayer: 2, TemporalLayer: 2},
			{SpatialLayer: 1, TemporalLayer: 2},
			{SpatialLayer: 0, TemporalLayer: 2},
		}
		preferredLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

		err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, preferredLayer)
		require.NoError(t, err)

		// Simulate high bandwidth
		results := ctrl.OnBandwidthEstimate(ctx, "subscriber1", 5_000_000) // 5 Mbps

		// Should stay at high layer, no changes
		require.Len(t, results, 0)

		current, err := ctrl.GetCurrentLayer(subID)
		require.NoError(t, err)
		assert.Equal(t, SVCLayer{SpatialLayer: 2, TemporalLayer: 2}, current)
	})
}

func TestControllerOnPacketLoss(t *testing.T) {
	ctx := context.Background()

	t.Run("downgrades layer on high packet loss", func(t *testing.T) {
		var layerChangeResults []LayerChangeResult
		cfg := &ControllerConfig{
			OnLayerChange: func(result LayerChangeResult) {
				layerChangeResults = append(layerChangeResults, result)
			},
			LayerSelectorConfig: DefaultLayerSelectorConfig(),
		}
		ctrl := NewController(cfg)

		subID := media.GenerateSubscriptionID()
		trackID := media.GenerateTrackID()
		availableLayers := []SVCLayer{
			{SpatialLayer: 2, TemporalLayer: 2},
			{SpatialLayer: 1, TemporalLayer: 2},
			{SpatialLayer: 0, TemporalLayer: 2},
		}
		preferredLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

		err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, preferredLayer)
		require.NoError(t, err)

		// Simulate high packet loss (>5%)
		results := ctrl.OnPacketLoss(ctx, "subscriber1", 0.10) // 10% loss

		require.Len(t, results, 1)
		assert.True(t, results[0].Changed)
		assert.Equal(t, LayerChangeReasonPacketLoss, results[0].Reason)
		assert.Equal(t, SVCLayer{SpatialLayer: 2, TemporalLayer: 2}, results[0].PreviousLayer)
		assert.Equal(t, SVCLayer{SpatialLayer: 1, TemporalLayer: 2}, results[0].CurrentLayer)
	})

	t.Run("recovers layer on low packet loss with bandwidth", func(t *testing.T) {
		var layerChangeResults []LayerChangeResult
		cfg := &ControllerConfig{
			OnLayerChange: func(result LayerChangeResult) {
				layerChangeResults = append(layerChangeResults, result)
			},
			LayerSelectorConfig: DefaultLayerSelectorConfig(),
		}
		ctrl := NewController(cfg)

		subID := media.GenerateSubscriptionID()
		trackID := media.GenerateTrackID()
		availableLayers := []SVCLayer{
			{SpatialLayer: 2, TemporalLayer: 2},
			{SpatialLayer: 1, TemporalLayer: 2},
			{SpatialLayer: 0, TemporalLayer: 2},
		}
		// Start at low layer to test recovery
		preferredLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

		err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, preferredLayer)
		require.NoError(t, err)

		// First downgrade twice to reach S0T2
		ctrl.OnPacketLoss(ctx, "subscriber1", 0.10) // S2T2 -> S1T2
		ctrl.OnPacketLoss(ctx, "subscriber1", 0.10) // S1T2 -> S0T2

		// Verify we're at the lowest layer
		current, err := ctrl.GetCurrentLayer(subID)
		require.NoError(t, err)
		assert.Equal(t, SVCLayer{SpatialLayer: 0, TemporalLayer: 2}, current)

		// Set bandwidth estimate
		ctrl.OnBandwidthEstimate(ctx, "subscriber1", 5_000_000)

		layerChangeResults = nil // Reset results

		// Now recover with low packet loss
		results := ctrl.OnPacketLoss(ctx, "subscriber1", 0.005) // 0.5% loss

		require.Len(t, results, 1)
		assert.True(t, results[0].Changed)
		assert.Equal(t, LayerChangeReasonRecovery, results[0].Reason)
		assert.Equal(t, SVCLayer{SpatialLayer: 0, TemporalLayer: 2}, results[0].PreviousLayer)
		assert.Equal(t, SVCLayer{SpatialLayer: 1, TemporalLayer: 2}, results[0].CurrentLayer)
	})
}

func TestControllerSubscriptionState(t *testing.T) {
	ctrl := NewController(nil)

	subID := media.GenerateSubscriptionID()
	trackID := media.GenerateTrackID()
	availableLayers := []SVCLayer{
		{SpatialLayer: 2, TemporalLayer: 2},
		{SpatialLayer: 1, TemporalLayer: 2},
	}
	preferredLayer := SVCLayer{SpatialLayer: 2, TemporalLayer: 2}

	err := ctrl.RegisterSubscription(subID, "subscriber1", trackID, availableLayers, preferredLayer)
	require.NoError(t, err)

	t.Run("returns state copy that does not affect original", func(t *testing.T) {
		state, err := ctrl.GetSubscriptionState(subID)
		require.NoError(t, err)

		// Modify the copy
		state.AvailableLayers = append(state.AvailableLayers, SVCLayer{SpatialLayer: 0, TemporalLayer: 0})
		state.PreferredLayer = SVCLayer{SpatialLayer: 0, TemporalLayer: 0}

		// Original should be unchanged
		original, err := ctrl.GetSubscriptionState(subID)
		require.NoError(t, err)
		assert.Len(t, original.AvailableLayers, 2)
		assert.Equal(t, preferredLayer, original.PreferredLayer)
	})
}
