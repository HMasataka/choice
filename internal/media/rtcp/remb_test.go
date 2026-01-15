package rtcp

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
)

func TestNewREMBHandler(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		h := NewREMBHandler(nil)
		if h == nil {
			t.Fatal("expected non-nil handler")
		}
		if h.config.MinBitrate != 100_000 {
			t.Errorf("expected 100Kbps min bitrate, got %d", h.config.MinBitrate)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &REMBConfig{
			MinBitrate:     50_000,
			MaxBitrate:     100_000_000,
			ExpirationTime: 10 * time.Second,
		}
		h := NewREMBHandler(cfg)
		if h.config.MinBitrate != 50_000 {
			t.Errorf("expected 50Kbps min bitrate, got %d", h.config.MinBitrate)
		}
	})
}

func TestREMBHandler_HandleREMB(t *testing.T) {
	h := NewREMBHandler(nil)

	remb := &rtcp.ReceiverEstimatedMaximumBitrate{
		SenderSSRC: 12345,
		Bitrate:    1_000_000, // 1 Mbps
		SSRCs:      []uint32{67890},
	}

	h.HandleREMB(remb)

	estimate := h.GetBandwidthEstimate()
	if estimate != 1_000_000 {
		t.Errorf("expected 1Mbps, got %d", estimate)
	}

	ssrcEstimate := h.GetEstimateForSSRC(67890)
	if ssrcEstimate != 1_000_000 {
		t.Errorf("expected 1Mbps for SSRC 67890, got %d", ssrcEstimate)
	}
}

func TestREMBHandler_ClampsToLimits(t *testing.T) {
	cfg := &REMBConfig{
		MinBitrate:     100_000,
		MaxBitrate:     10_000_000,
		ExpirationTime: 5 * time.Second,
	}
	h := NewREMBHandler(cfg)

	t.Run("clamps to minimum", func(t *testing.T) {
		remb := &rtcp.ReceiverEstimatedMaximumBitrate{
			Bitrate: 10_000, // Below minimum
			SSRCs:   []uint32{12345},
		}
		h.HandleREMB(remb)

		estimate := h.GetBandwidthEstimate()
		if estimate != 100_000 {
			t.Errorf("expected minimum 100Kbps, got %d", estimate)
		}
	})

	t.Run("clamps to maximum", func(t *testing.T) {
		remb := &rtcp.ReceiverEstimatedMaximumBitrate{
			Bitrate: 100_000_000, // Above maximum
			SSRCs:   []uint32{67890},
		}
		h.HandleREMB(remb)

		estimate := h.GetEstimateForSSRC(67890)
		if estimate != 10_000_000 {
			t.Errorf("expected maximum 10Mbps, got %d", estimate)
		}
	})
}

func TestREMBHandler_MultipleSSRCs(t *testing.T) {
	h := NewREMBHandler(nil)

	// First REMB with 2 Mbps
	remb1 := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 2_000_000,
		SSRCs:   []uint32{12345},
	}
	h.HandleREMB(remb1)

	// Second REMB with 1 Mbps
	remb2 := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 1_000_000,
		SSRCs:   []uint32{67890},
	}
	h.HandleREMB(remb2)

	// Aggregate should be minimum (1 Mbps)
	estimate := h.GetBandwidthEstimate()
	if estimate != 1_000_000 {
		t.Errorf("expected minimum of 1Mbps, got %d", estimate)
	}
}

func TestREMBHandler_StaleEstimate(t *testing.T) {
	cfg := &REMBConfig{
		MinBitrate:     100_000,
		MaxBitrate:     50_000_000,
		ExpirationTime: 50 * time.Millisecond, // Short expiration for testing
	}
	h := NewREMBHandler(cfg)

	remb := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 1_000_000,
		SSRCs:   []uint32{12345},
	}
	h.HandleREMB(remb)

	// Should be valid initially
	if h.IsStale() {
		t.Error("expected estimate to not be stale initially")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be stale now
	if !h.IsStale() {
		t.Error("expected estimate to be stale after expiration")
	}

	// GetBandwidthEstimate should return 0 when stale
	estimate := h.GetBandwidthEstimate()
	if estimate != 0 {
		t.Errorf("expected 0 for stale estimate, got %d", estimate)
	}
}

func TestREMBHandler_Reset(t *testing.T) {
	h := NewREMBHandler(nil)

	remb := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 1_000_000,
		SSRCs:   []uint32{12345},
	}
	h.HandleREMB(remb)

	// Verify estimate exists
	if h.GetBandwidthEstimate() == 0 {
		t.Error("expected non-zero estimate before reset")
	}

	// Reset
	h.Reset()

	// Estimate should be 0 after reset
	if h.GetBandwidthEstimate() != 0 {
		t.Error("expected 0 estimate after reset")
	}
}

func TestREMBHandler_GetStats(t *testing.T) {
	h := NewREMBHandler(nil)

	remb := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 1_000_000,
		SSRCs:   []uint32{12345, 67890},
	}
	h.HandleREMB(remb)

	stats := h.GetStats()
	if stats.BandwidthEstimate != 1_000_000 {
		t.Errorf("expected 1Mbps, got %d", stats.BandwidthEstimate)
	}
	if len(stats.SSRCEstimates) != 2 {
		t.Errorf("expected 2 SSRC estimates, got %d", len(stats.SSRCEstimates))
	}
}

func TestREMBHandler_GenerateREMB(t *testing.T) {
	h := NewREMBHandler(nil)

	remb := h.GenerateREMB(12345, []uint32{67890, 11111}, 2_000_000)

	if remb.SenderSSRC != 12345 {
		t.Errorf("expected sender SSRC 12345, got %d", remb.SenderSSRC)
	}
	if remb.Bitrate != 2_000_000 {
		t.Errorf("expected 2Mbps bitrate, got %f", remb.Bitrate)
	}
	if len(remb.SSRCs) != 2 {
		t.Errorf("expected 2 SSRCs, got %d", len(remb.SSRCs))
	}
}

func TestREMBHandler_BandwidthUpdateCallback(t *testing.T) {
	h := NewREMBHandler(nil)

	var callbackCalled bool
	var receivedBitrate uint64

	h.SetOnBandwidthUpdate(func(bitrate uint64) {
		callbackCalled = true
		receivedBitrate = bitrate
	})

	remb := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 1_000_000,
		SSRCs:   []uint32{12345},
	}
	h.HandleREMB(remb)

	if !callbackCalled {
		t.Error("expected callback to be called")
	}
	if receivedBitrate != 1_000_000 {
		t.Errorf("expected 1Mbps in callback, got %d", receivedBitrate)
	}
}

func TestDefaultREMBConfig(t *testing.T) {
	cfg := DefaultREMBConfig()

	if cfg.MinBitrate != 100_000 {
		t.Errorf("expected 100Kbps min bitrate, got %d", cfg.MinBitrate)
	}
	if cfg.MaxBitrate != 50_000_000 {
		t.Errorf("expected 50Mbps max bitrate, got %d", cfg.MaxBitrate)
	}
	if cfg.ExpirationTime != 5*time.Second {
		t.Errorf("expected 5s expiration time, got %v", cfg.ExpirationTime)
	}
}
