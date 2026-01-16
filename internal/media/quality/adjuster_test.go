package quality

import (
	"context"
	"testing"
	"time"

	"github.com/HMasataka/choice/internal/media"
	"github.com/HMasataka/choice/internal/media/simulcast"
)

// mockSimulcastController is a mock implementation of simulcast.Controller.
type mockSimulcastController struct {
	onPacketLossResults   []simulcast.LayerChangeResult
	onBandwidthResults    []simulcast.LayerChangeResult
	lastPacketLossRate    float64
	lastBandwidthEstimate uint64
	lastPacketLossSubID   string
	lastBandwidthSubID    string
}

func (m *mockSimulcastController) SetPreferredLayer(ctx context.Context, subscriptionID media.SubscriptionID, layer media.SimulcastLayer) error {
	return nil
}

func (m *mockSimulcastController) GetCurrentLayer(subscriptionID media.SubscriptionID) (media.SimulcastLayer, error) {
	return media.SimulcastLayerMedium, nil
}

func (m *mockSimulcastController) GetPreferredLayer(subscriptionID media.SubscriptionID) (media.SimulcastLayer, error) {
	return media.SimulcastLayerHigh, nil
}

func (m *mockSimulcastController) OnBandwidthEstimate(ctx context.Context, subscriberID string, bps uint64) []simulcast.LayerChangeResult {
	m.lastBandwidthSubID = subscriberID
	m.lastBandwidthEstimate = bps
	return m.onBandwidthResults
}

func (m *mockSimulcastController) OnPacketLoss(ctx context.Context, subscriberID string, lossRate float64) []simulcast.LayerChangeResult {
	m.lastPacketLossSubID = subscriberID
	m.lastPacketLossRate = lossRate
	return m.onPacketLossResults
}

func (m *mockSimulcastController) RegisterSubscription(subscriptionID media.SubscriptionID, subscriberID string, trackID media.TrackID, availableLayers []media.SimulcastLayer, preferredLayer media.SimulcastLayer) error {
	return nil
}

func (m *mockSimulcastController) UnregisterSubscription(subscriptionID media.SubscriptionID) error {
	return nil
}

func (m *mockSimulcastController) GetSubscriptionState(subscriptionID media.SubscriptionID) (*simulcast.SubscriptionState, error) {
	return nil, nil
}

func TestNewAdjuster(t *testing.T) {
	t.Run("with nil config and controller", func(t *testing.T) {
		a := NewAdjuster(nil, nil)
		if a == nil {
			t.Fatal("expected non-nil adjuster")
		}
		if a.config == nil {
			t.Fatal("expected non-nil config")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &AdjusterConfig{
			PacketLossThreshold: 0.10,
			RTTThreshold:        500,
		}
		a := NewAdjuster(cfg, nil)
		if a.config.PacketLossThreshold != 0.10 {
			t.Errorf("expected packet loss threshold 0.10, got %f", a.config.PacketLossThreshold)
		}
	})
}

func TestAdjuster_ProcessMetrics_EmptySubscriberID(t *testing.T) {
	a := NewAdjuster(nil, nil)
	results := a.ProcessMetrics(context.Background(), "", QualityMetrics{})
	if len(results) != 0 {
		t.Error("expected no results for empty subscriber ID")
	}
}

func TestAdjuster_ProcessMetrics_HighPacketLoss(t *testing.T) {
	mock := &mockSimulcastController{
		onPacketLossResults: []simulcast.LayerChangeResult{
			{
				SubscriptionID: "sub-1",
				Changed:        true,
				PreviousLayer:  media.SimulcastLayerHigh,
				CurrentLayer:   media.SimulcastLayerMedium,
				Reason:         simulcast.LayerChangeReasonPacketLoss,
			},
		},
	}
	a := NewAdjuster(nil, mock)

	metrics := QualityMetrics{
		PacketLossRate: 0.08, // Above 5% threshold
		RTT:            100,
		Bandwidth:      2_000_000,
		Timestamp:      time.Now(),
	}

	results := a.ProcessMetrics(context.Background(), "subscriber-1", metrics)

	if mock.lastPacketLossSubID != "subscriber-1" {
		t.Errorf("expected subscriber-1, got %s", mock.lastPacketLossSubID)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Reason != AdjustmentReasonPacketLoss {
		t.Errorf("expected packet loss reason, got %s", results[0].Reason)
	}
}

func TestAdjuster_ProcessMetrics_HighRTT(t *testing.T) {
	mock := &mockSimulcastController{
		onPacketLossResults: []simulcast.LayerChangeResult{
			{
				SubscriptionID: "sub-1",
				Changed:        true,
				PreviousLayer:  media.SimulcastLayerHigh,
				CurrentLayer:   media.SimulcastLayerLow,
				Reason:         simulcast.LayerChangeReasonPacketLoss,
			},
		},
	}
	a := NewAdjuster(nil, mock)

	metrics := QualityMetrics{
		PacketLossRate: 0.01, // Below threshold
		RTT:            400,  // Above 300ms threshold
		Bandwidth:      2_000_000,
		Timestamp:      time.Now(),
	}

	results := a.ProcessMetrics(context.Background(), "subscriber-1", metrics)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Reason != AdjustmentReasonRTT {
		t.Errorf("expected RTT reason, got %s", results[0].Reason)
	}
}

func TestAdjuster_ProcessMetrics_Recovery(t *testing.T) {
	cfg := &AdjusterConfig{
		PacketLossThreshold:         0.05,
		RTTThreshold:                300,
		RecoveryPacketLossThreshold: 0.01,
		RecoveryRTTThreshold:        150,
		HysteresisWindow:            10 * time.Millisecond,
		MinDowngradeDuration:        1 * time.Millisecond,
		MinUpgradeDuration:          10 * time.Millisecond,
	}
	mock := &mockSimulcastController{
		onBandwidthResults: []simulcast.LayerChangeResult{
			{
				SubscriptionID: "sub-1",
				Changed:        true,
				PreviousLayer:  media.SimulcastLayerLow,
				CurrentLayer:   media.SimulcastLayerMedium,
				Reason:         simulcast.LayerChangeReasonRecovery,
			},
		},
	}
	a := NewAdjuster(cfg, mock)

	// First, simulate a downgrade
	a.ProcessMetrics(context.Background(), "subscriber-1", QualityMetrics{
		PacketLossRate: 0.08,
		RTT:            100,
		Timestamp:      time.Now(),
	})

	// Wait for hysteresis
	time.Sleep(20 * time.Millisecond)

	// Now simulate recovery conditions
	goodMetrics := QualityMetrics{
		PacketLossRate: 0.005, // Below 1%
		RTT:            100,   // Below 150ms
		Bandwidth:      2_000_000,
		Timestamp:      time.Now(),
	}

	results := a.ProcessMetrics(context.Background(), "subscriber-1", goodMetrics)

	if mock.lastBandwidthSubID != "subscriber-1" {
		t.Errorf("expected subscriber-1, got %s", mock.lastBandwidthSubID)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Reason != AdjustmentReasonRecovery {
		t.Errorf("expected recovery reason, got %s", results[0].Reason)
	}
}

func TestAdjuster_Hysteresis(t *testing.T) {
	cfg := &AdjusterConfig{
		PacketLossThreshold:         0.05,
		RTTThreshold:                300,
		RecoveryPacketLossThreshold: 0.01,
		RecoveryRTTThreshold:        150,
		HysteresisWindow:            100 * time.Millisecond,
		MinDowngradeDuration:        50 * time.Millisecond,
		MinUpgradeDuration:          100 * time.Millisecond,
	}
	mock := &mockSimulcastController{
		onPacketLossResults: []simulcast.LayerChangeResult{
			{Changed: true, PreviousLayer: media.SimulcastLayerHigh, CurrentLayer: media.SimulcastLayerLow},
		},
	}
	a := NewAdjuster(cfg, mock)

	// First downgrade
	a.ProcessMetrics(context.Background(), "subscriber-1", QualityMetrics{
		PacketLossRate: 0.08,
		RTT:            100,
		Timestamp:      time.Now(),
	})

	// Try to downgrade again immediately - should be blocked by hysteresis
	mock.onPacketLossResults = []simulcast.LayerChangeResult{
		{Changed: true, PreviousLayer: media.SimulcastLayerLow, CurrentLayer: media.SimulcastLayerLow},
	}
	results := a.ProcessMetrics(context.Background(), "subscriber-1", QualityMetrics{
		PacketLossRate: 0.08,
		RTT:            100,
		Timestamp:      time.Now(),
	})

	if len(results) != 0 {
		t.Error("expected no results due to hysteresis")
	}
}

func TestAdjuster_Callback(t *testing.T) {
	mock := &mockSimulcastController{
		onPacketLossResults: []simulcast.LayerChangeResult{
			{
				SubscriptionID: "sub-1",
				Changed:        true,
				PreviousLayer:  media.SimulcastLayerHigh,
				CurrentLayer:   media.SimulcastLayerLow,
			},
		},
	}
	a := NewAdjuster(nil, mock)

	var callbacks []AdjustmentResult
	a.SetOnAdjustment(func(result AdjustmentResult) {
		callbacks = append(callbacks, result)
	})

	a.ProcessMetrics(context.Background(), "subscriber-1", QualityMetrics{
		PacketLossRate: 0.08,
		RTT:            100,
		Timestamp:      time.Now(),
	})

	if len(callbacks) != 1 {
		t.Errorf("expected 1 callback, got %d", len(callbacks))
	}
}

func TestAdjuster_RemoveSubscriber(t *testing.T) {
	a := NewAdjuster(nil, nil)

	// Add some state
	a.ProcessMetrics(context.Background(), "subscriber-1", QualityMetrics{
		PacketLossRate: 0.01,
		RTT:            100,
		Timestamp:      time.Now(),
	})

	// Verify state exists
	_, found := a.GetSubscriberMetrics("subscriber-1")
	if !found {
		t.Error("expected to find subscriber metrics")
	}

	// Remove subscriber
	a.RemoveSubscriber("subscriber-1")

	// Verify state removed
	_, found = a.GetSubscriberMetrics("subscriber-1")
	if found {
		t.Error("expected subscriber metrics to be removed")
	}
}

func TestAdjuster_NoControllerNilSafe(t *testing.T) {
	a := NewAdjuster(nil, nil) // No controller

	// Should not panic
	results := a.ProcessMetrics(context.Background(), "subscriber-1", QualityMetrics{
		PacketLossRate: 0.08,
		RTT:            100,
		Timestamp:      time.Now(),
	})

	if len(results) != 0 {
		t.Error("expected no results without controller")
	}
}
