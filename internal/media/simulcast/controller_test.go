package simulcast

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/HMasataka/choice/internal/media"
)

func generateSubscriptionID() media.SubscriptionID {
	return media.SubscriptionID(uuid.New().String())
}

func generateTrackID() media.TrackID {
	return media.TrackID(uuid.New().String())
}

func TestController_RegisterSubscription(t *testing.T) {
	ctrl := NewController(nil)

	subID := generateSubscriptionID()
	trackID := generateTrackID()
	subscriberID := "subscriber-1"
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow}
	preferredLayer := media.SimulcastLayerHigh

	err := ctrl.RegisterSubscription(subID, subscriberID, trackID, availableLayers, preferredLayer)
	require.NoError(t, err)

	// Verify state
	state, err := ctrl.GetSubscriptionState(subID)
	require.NoError(t, err)
	assert.Equal(t, subID, state.SubscriptionID)
	assert.Equal(t, subscriberID, state.SubscriberID)
	assert.Equal(t, trackID, state.TrackID)
	assert.Equal(t, preferredLayer, state.PreferredLayer)
	assert.Equal(t, preferredLayer, state.ActualLayer) // Should match since it's available
	assert.ElementsMatch(t, availableLayers, state.AvailableLayers)
}

func TestController_RegisterSubscription_UnavailablePreferred(t *testing.T) {
	ctrl := NewController(nil)

	subID := generateSubscriptionID()
	trackID := generateTrackID()
	subscriberID := "subscriber-1"
	// Only medium and low available
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerMedium, media.SimulcastLayerLow}
	preferredLayer := media.SimulcastLayerHigh

	err := ctrl.RegisterSubscription(subID, subscriberID, trackID, availableLayers, preferredLayer)
	require.NoError(t, err)

	state, err := ctrl.GetSubscriptionState(subID)
	require.NoError(t, err)
	assert.Equal(t, preferredLayer, state.PreferredLayer)          // Still stores preferred
	assert.Equal(t, media.SimulcastLayerMedium, state.ActualLayer) // Falls back to medium
}

func TestController_RegisterSubscription_Duplicate(t *testing.T) {
	ctrl := NewController(nil)

	subID := generateSubscriptionID()
	trackID := generateTrackID()
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh}

	err := ctrl.RegisterSubscription(subID, "subscriber-1", trackID, availableLayers, media.SimulcastLayerHigh)
	require.NoError(t, err)

	// Try to register again
	err = ctrl.RegisterSubscription(subID, "subscriber-2", trackID, availableLayers, media.SimulcastLayerHigh)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestController_RegisterSubscription_InvalidParams(t *testing.T) {
	ctrl := NewController(nil)
	trackID := generateTrackID()
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh}

	tests := []struct {
		name            string
		subID           media.SubscriptionID
		subscriberID    string
		trackID         media.TrackID
		availableLayers []media.SimulcastLayer
		preferredLayer  media.SimulcastLayer
		wantErr         string
	}{
		{
			name:            "invalid subscription ID",
			subID:           media.SubscriptionID("invalid"),
			subscriberID:    "subscriber-1",
			trackID:         trackID,
			availableLayers: availableLayers,
			preferredLayer:  media.SimulcastLayerHigh,
			wantErr:         "invalid subscription ID",
		},
		{
			name:            "empty subscriber ID",
			subID:           generateSubscriptionID(),
			subscriberID:    "",
			trackID:         trackID,
			availableLayers: availableLayers,
			preferredLayer:  media.SimulcastLayerHigh,
			wantErr:         "subscriber ID cannot be empty",
		},
		{
			name:            "invalid track ID",
			subID:           generateSubscriptionID(),
			subscriberID:    "subscriber-1",
			trackID:         media.TrackID("invalid"),
			availableLayers: availableLayers,
			preferredLayer:  media.SimulcastLayerHigh,
			wantErr:         "invalid track ID",
		},
		{
			name:            "empty available layers",
			subID:           generateSubscriptionID(),
			subscriberID:    "subscriber-1",
			trackID:         trackID,
			availableLayers: []media.SimulcastLayer{},
			preferredLayer:  media.SimulcastLayerHigh,
			wantErr:         "available layers cannot be empty",
		},
		{
			name:            "invalid preferred layer",
			subID:           generateSubscriptionID(),
			subscriberID:    "subscriber-1",
			trackID:         trackID,
			availableLayers: availableLayers,
			preferredLayer:  media.SimulcastLayer("invalid"),
			wantErr:         "invalid preferred layer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ctrl.RegisterSubscription(tt.subID, tt.subscriberID, tt.trackID, tt.availableLayers, tt.preferredLayer)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestController_UnregisterSubscription(t *testing.T) {
	ctrl := NewController(nil)

	subID := generateSubscriptionID()
	trackID := generateTrackID()
	subscriberID := "subscriber-1"
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium}

	err := ctrl.RegisterSubscription(subID, subscriberID, trackID, availableLayers, media.SimulcastLayerHigh)
	require.NoError(t, err)

	err = ctrl.UnregisterSubscription(subID)
	require.NoError(t, err)

	// Verify it's gone
	_, err = ctrl.GetSubscriptionState(subID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestController_UnregisterSubscription_NotFound(t *testing.T) {
	ctrl := NewController(nil)

	err := ctrl.UnregisterSubscription(generateSubscriptionID())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestController_SetPreferredLayer(t *testing.T) {
	var layerChanges []LayerChangeResult
	ctrl := NewController(&ControllerConfig{
		OnLayerChange: func(result LayerChangeResult) {
			layerChanges = append(layerChanges, result)
		},
	})

	subID := generateSubscriptionID()
	trackID := generateTrackID()
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow}

	err := ctrl.RegisterSubscription(subID, "subscriber-1", trackID, availableLayers, media.SimulcastLayerHigh)
	require.NoError(t, err)
	layerChanges = nil // Clear any registration callbacks

	// Change to medium
	ctx := context.Background()
	err = ctrl.SetPreferredLayer(ctx, subID, media.SimulcastLayerMedium)
	require.NoError(t, err)

	// Verify layer change
	current, err := ctrl.GetCurrentLayer(subID)
	require.NoError(t, err)
	assert.Equal(t, media.SimulcastLayerMedium, current)

	preferred, err := ctrl.GetPreferredLayer(subID)
	require.NoError(t, err)
	assert.Equal(t, media.SimulcastLayerMedium, preferred)

	// Verify callback was called
	require.Len(t, layerChanges, 1)
	assert.Equal(t, subID, layerChanges[0].SubscriptionID)
	assert.True(t, layerChanges[0].Changed)
	assert.Equal(t, media.SimulcastLayerHigh, layerChanges[0].PreviousLayer)
	assert.Equal(t, media.SimulcastLayerMedium, layerChanges[0].CurrentLayer)
	assert.Equal(t, LayerChangeReasonRequested, layerChanges[0].Reason)
}

func TestController_SetPreferredLayer_Unavailable(t *testing.T) {
	var layerChanges []LayerChangeResult
	ctrl := NewController(&ControllerConfig{
		OnLayerChange: func(result LayerChangeResult) {
			layerChanges = append(layerChanges, result)
		},
	})

	subID := generateSubscriptionID()
	trackID := generateTrackID()
	// Only low available
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerLow}

	err := ctrl.RegisterSubscription(subID, "subscriber-1", trackID, availableLayers, media.SimulcastLayerLow)
	require.NoError(t, err)
	layerChanges = nil

	// Request high (unavailable)
	ctx := context.Background()
	err = ctrl.SetPreferredLayer(ctx, subID, media.SimulcastLayerHigh)
	require.NoError(t, err)

	// Should still be on low
	current, err := ctrl.GetCurrentLayer(subID)
	require.NoError(t, err)
	assert.Equal(t, media.SimulcastLayerLow, current)

	// Preferred should be high
	preferred, err := ctrl.GetPreferredLayer(subID)
	require.NoError(t, err)
	assert.Equal(t, media.SimulcastLayerHigh, preferred)

	// No callback since actual layer didn't change
	assert.Len(t, layerChanges, 0)
}

func TestController_OnBandwidthEstimate(t *testing.T) {
	var layerChanges []LayerChangeResult
	ctrl := NewController(&ControllerConfig{
		OnLayerChange: func(result LayerChangeResult) {
			layerChanges = append(layerChanges, result)
		},
	})

	subID := generateSubscriptionID()
	trackID := generateTrackID()
	subscriberID := "subscriber-1"
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow}

	err := ctrl.RegisterSubscription(subID, subscriberID, trackID, availableLayers, media.SimulcastLayerHigh)
	require.NoError(t, err)
	layerChanges = nil

	ctx := context.Background()

	// High bandwidth - should stay on high
	results := ctrl.OnBandwidthEstimate(ctx, subscriberID, 3_000_000) // 3Mbps
	assert.Len(t, results, 0)

	current, _ := ctrl.GetCurrentLayer(subID)
	assert.Equal(t, media.SimulcastLayerHigh, current)

	// Low bandwidth - should drop to medium
	results = ctrl.OnBandwidthEstimate(ctx, subscriberID, 600_000) // 600Kbps
	require.Len(t, results, 1)
	assert.True(t, results[0].Changed)
	assert.Equal(t, media.SimulcastLayerHigh, results[0].PreviousLayer)
	assert.Equal(t, media.SimulcastLayerMedium, results[0].CurrentLayer)
	assert.Equal(t, LayerChangeReasonBandwidth, results[0].Reason)

	current, _ = ctrl.GetCurrentLayer(subID)
	assert.Equal(t, media.SimulcastLayerMedium, current)

	// Very low bandwidth - should drop to low
	layerChanges = nil
	results = ctrl.OnBandwidthEstimate(ctx, subscriberID, 100_000) // 100Kbps
	require.Len(t, results, 1)
	assert.Equal(t, media.SimulcastLayerLow, results[0].CurrentLayer)

	current, _ = ctrl.GetCurrentLayer(subID)
	assert.Equal(t, media.SimulcastLayerLow, current)
}

func TestController_OnPacketLoss(t *testing.T) {
	var layerChanges []LayerChangeResult
	ctrl := NewController(&ControllerConfig{
		OnLayerChange: func(result LayerChangeResult) {
			layerChanges = append(layerChanges, result)
		},
	})

	subID := generateSubscriptionID()
	trackID := generateTrackID()
	subscriberID := "subscriber-1"
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow}

	err := ctrl.RegisterSubscription(subID, subscriberID, trackID, availableLayers, media.SimulcastLayerHigh)
	require.NoError(t, err)
	layerChanges = nil

	ctx := context.Background()

	// Set high bandwidth first (required for recovery)
	ctrl.OnBandwidthEstimate(ctx, subscriberID, 5_000_000) // 5Mbps

	// High packet loss (>5%) - should drop
	results := ctrl.OnPacketLoss(ctx, subscriberID, 0.06) // 6%
	require.Len(t, results, 1)
	assert.True(t, results[0].Changed)
	assert.Equal(t, media.SimulcastLayerMedium, results[0].CurrentLayer)
	assert.Equal(t, LayerChangeReasonPacketLoss, results[0].Reason)

	// Continue high packet loss - drop again
	layerChanges = nil
	results = ctrl.OnPacketLoss(ctx, subscriberID, 0.10) // 10%
	require.Len(t, results, 1)
	assert.Equal(t, media.SimulcastLayerLow, results[0].CurrentLayer)

	// Low packet loss with bandwidth - should recover
	layerChanges = nil
	results = ctrl.OnPacketLoss(ctx, subscriberID, 0.005) // 0.5%
	require.Len(t, results, 1)
	assert.Equal(t, media.SimulcastLayerMedium, results[0].CurrentLayer)
	assert.Equal(t, LayerChangeReasonRecovery, results[0].Reason)
}

func TestController_OnPacketLoss_NoRecoveryWithoutBandwidth(t *testing.T) {
	ctrl := NewController(nil)

	subID := generateSubscriptionID()
	trackID := generateTrackID()
	subscriberID := "subscriber-1"
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow}

	err := ctrl.RegisterSubscription(subID, subscriberID, trackID, availableLayers, media.SimulcastLayerHigh)
	require.NoError(t, err)

	ctx := context.Background()

	// Drop to low via packet loss (no bandwidth estimate yet)
	ctrl.OnPacketLoss(ctx, subscriberID, 0.10)
	ctrl.OnPacketLoss(ctx, subscriberID, 0.10)

	current, _ := ctrl.GetCurrentLayer(subID)
	assert.Equal(t, media.SimulcastLayerLow, current)

	// Low packet loss but no bandwidth estimate - should NOT recover
	results := ctrl.OnPacketLoss(ctx, subscriberID, 0.005)
	assert.Len(t, results, 0) // No change because bps=0

	current, _ = ctrl.GetCurrentLayer(subID)
	assert.Equal(t, media.SimulcastLayerLow, current)
}

func TestController_MultipleSubscriptions_SameSubscriber(t *testing.T) {
	ctrl := NewController(nil)
	ctx := context.Background()

	subscriberID := "subscriber-1"
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow}

	// Register multiple subscriptions for same subscriber
	subID1 := generateSubscriptionID()
	subID2 := generateSubscriptionID()
	trackID1 := generateTrackID()
	trackID2 := generateTrackID()

	err := ctrl.RegisterSubscription(subID1, subscriberID, trackID1, availableLayers, media.SimulcastLayerHigh)
	require.NoError(t, err)
	err = ctrl.RegisterSubscription(subID2, subscriberID, trackID2, availableLayers, media.SimulcastLayerHigh)
	require.NoError(t, err)

	// Bandwidth event should affect both
	results := ctrl.OnBandwidthEstimate(ctx, subscriberID, 600_000)
	assert.Len(t, results, 2)

	// Both should be on medium
	current1, _ := ctrl.GetCurrentLayer(subID1)
	current2, _ := ctrl.GetCurrentLayer(subID2)
	assert.Equal(t, media.SimulcastLayerMedium, current1)
	assert.Equal(t, media.SimulcastLayerMedium, current2)
}

func TestController_Concurrency(t *testing.T) {
	ctrl := NewController(nil)
	ctx := context.Background()
	availableLayers := []media.SimulcastLayer{media.SimulcastLayerHigh, media.SimulcastLayerMedium, media.SimulcastLayerLow}

	var wg sync.WaitGroup
	numGoroutines := 10
	numOps := 100

	// Create subscriptions
	subIDs := make([]media.SubscriptionID, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		subIDs[i] = generateSubscriptionID()
		trackID := generateTrackID()
		err := ctrl.RegisterSubscription(subIDs[i], "subscriber-"+string(rune('0'+i)), trackID, availableLayers, media.SimulcastLayerHigh)
		require.NoError(t, err)
	}

	// Concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			subID := subIDs[idx]
			subscriberID := "subscriber-" + string(rune('0'+idx))

			for j := 0; j < numOps; j++ {
				// Mix of operations
				switch j % 4 {
				case 0:
					_ = ctrl.SetPreferredLayer(ctx, subID, media.SimulcastLayerMedium)
				case 1:
					_, _ = ctrl.GetCurrentLayer(subID)
				case 2:
					_ = ctrl.OnBandwidthEstimate(ctx, subscriberID, 1_000_000)
				case 3:
					_, _ = ctrl.GetSubscriptionState(subID)
				}
			}
		}(i)
	}

	wg.Wait()
	// No panics = success
}
