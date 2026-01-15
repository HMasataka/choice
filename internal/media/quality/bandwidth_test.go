package quality

import (
	"sync"
	"testing"
	"time"
)

func TestNewBandwidthEstimator(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		e := NewBandwidthEstimator(nil)
		if e == nil {
			t.Fatal("expected non-nil estimator")
		}
		if e.config == nil {
			t.Fatal("expected non-nil config")
		}
		if e.config.DefaultBandwidth != DefaultBandwidthEstimatorConfig().DefaultBandwidth {
			t.Errorf("expected default bandwidth %d, got %d",
				DefaultBandwidthEstimatorConfig().DefaultBandwidth, e.config.DefaultBandwidth)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &BandwidthEstimatorConfig{
			TWCCTimeout:      1 * time.Second,
			REMBTimeout:      2 * time.Second,
			DefaultBandwidth: 500_000,
		}
		e := NewBandwidthEstimator(cfg)
		if e.config.DefaultBandwidth != 500_000 {
			t.Errorf("expected default bandwidth 500000, got %d", e.config.DefaultBandwidth)
		}
	})
}

func TestBandwidthEstimator_UpdateTWCC(t *testing.T) {
	e := NewBandwidthEstimator(nil)

	e.UpdateTWCC(2_000_000)

	estimate := e.GetEstimate()
	if estimate.Bandwidth != 2_000_000 {
		t.Errorf("expected bandwidth 2000000, got %d", estimate.Bandwidth)
	}
	if estimate.Source != BandwidthSourceTWCC {
		t.Errorf("expected source TWCC, got %s", estimate.Source)
	}
}

func TestBandwidthEstimator_UpdateREMB(t *testing.T) {
	e := NewBandwidthEstimator(nil)

	e.UpdateREMB(1_500_000)

	// Without TWCC, REMB should be used
	estimate := e.GetEstimate()
	if estimate.Bandwidth != 1_500_000 {
		t.Errorf("expected bandwidth 1500000, got %d", estimate.Bandwidth)
	}
	if estimate.Source != BandwidthSourceREMB {
		t.Errorf("expected source REMB, got %s", estimate.Source)
	}
}

func TestBandwidthEstimator_TWCCPreferred(t *testing.T) {
	e := NewBandwidthEstimator(nil)

	// Update both TWCC and REMB
	e.UpdateTWCC(2_000_000)
	e.UpdateREMB(1_500_000)

	// TWCC should be preferred
	estimate := e.GetEstimate()
	if estimate.Bandwidth != 2_000_000 {
		t.Errorf("expected TWCC bandwidth 2000000, got %d", estimate.Bandwidth)
	}
	if estimate.Source != BandwidthSourceTWCC {
		t.Errorf("expected source TWCC, got %s", estimate.Source)
	}
}

func TestBandwidthEstimator_FallbackToDefault(t *testing.T) {
	cfg := &BandwidthEstimatorConfig{
		TWCCTimeout:      10 * time.Millisecond,
		REMBTimeout:      10 * time.Millisecond,
		DefaultBandwidth: 1_000_000,
	}
	e := NewBandwidthEstimator(cfg)

	// Update estimates
	e.UpdateTWCC(2_000_000)
	e.UpdateREMB(1_500_000)

	// Wait for them to become stale
	time.Sleep(20 * time.Millisecond)

	// Should fall back to default
	estimate := e.GetEstimate()
	if estimate.Bandwidth != 1_000_000 {
		t.Errorf("expected default bandwidth 1000000, got %d", estimate.Bandwidth)
	}
	if estimate.Source != BandwidthSourceDefault {
		t.Errorf("expected source default, got %s", estimate.Source)
	}
}

func TestBandwidthEstimator_FallbackToREMB(t *testing.T) {
	cfg := &BandwidthEstimatorConfig{
		TWCCTimeout:      10 * time.Millisecond,
		REMBTimeout:      100 * time.Millisecond,
		DefaultBandwidth: 1_000_000,
	}
	e := NewBandwidthEstimator(cfg)

	// Update both
	e.UpdateTWCC(2_000_000)
	e.UpdateREMB(1_500_000)

	// Wait for TWCC to become stale but not REMB
	time.Sleep(20 * time.Millisecond)

	// Should fall back to REMB
	estimate := e.GetEstimate()
	if estimate.Bandwidth != 1_500_000 {
		t.Errorf("expected REMB bandwidth 1500000, got %d", estimate.Bandwidth)
	}
	if estimate.Source != BandwidthSourceREMB {
		t.Errorf("expected source REMB, got %s", estimate.Source)
	}
}

func TestBandwidthEstimator_Callback(t *testing.T) {
	e := NewBandwidthEstimator(nil)

	var lastEstimate BandwidthEstimate
	var callCount int
	e.SetOnBandwidthUpdate(func(estimate BandwidthEstimate) {
		lastEstimate = estimate
		callCount++
	})

	e.UpdateTWCC(2_000_000)

	if callCount != 1 {
		t.Errorf("expected 1 callback call, got %d", callCount)
	}
	if lastEstimate.Bandwidth != 2_000_000 {
		t.Errorf("expected bandwidth 2000000, got %d", lastEstimate.Bandwidth)
	}
}

func TestBandwidthEstimator_REMBCallbackOnlyWhenTWCCStale(t *testing.T) {
	cfg := &BandwidthEstimatorConfig{
		TWCCTimeout:      10 * time.Millisecond,
		REMBTimeout:      100 * time.Millisecond,
		DefaultBandwidth: 1_000_000,
	}
	e := NewBandwidthEstimator(cfg)

	var callCount int
	e.SetOnBandwidthUpdate(func(estimate BandwidthEstimate) {
		callCount++
	})

	// Update TWCC first
	e.UpdateTWCC(2_000_000)
	initialCount := callCount

	// Update REMB - should not trigger callback since TWCC is fresh
	e.UpdateREMB(1_500_000)
	if callCount != initialCount {
		t.Errorf("expected no callback for REMB when TWCC is fresh, got %d calls", callCount-initialCount)
	}

	// Wait for TWCC to become stale
	time.Sleep(20 * time.Millisecond)

	// Now REMB update should trigger callback
	e.UpdateREMB(1_600_000)
	if callCount != initialCount+1 {
		t.Errorf("expected callback for REMB when TWCC is stale")
	}
}

func TestBandwidthEstimator_Reset(t *testing.T) {
	e := NewBandwidthEstimator(nil)

	e.UpdateTWCC(2_000_000)
	e.UpdateREMB(1_500_000)

	e.Reset()

	twcc, twccTime := e.GetTWCCEstimate()
	remb, rembTime := e.GetREMBEstimate()

	if twcc != 0 || !twccTime.IsZero() {
		t.Error("expected TWCC to be reset")
	}
	if remb != 0 || !rembTime.IsZero() {
		t.Error("expected REMB to be reset")
	}
}

func TestBandwidthEstimator_Concurrent(t *testing.T) {
	e := NewBandwidthEstimator(nil)

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent TWCC updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			e.UpdateTWCC(uint64(i * 1000))
		}
	}()

	// Concurrent REMB updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			e.UpdateREMB(uint64(i * 500))
		}
	}()

	// Concurrent reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = e.GetEstimate()
		}
	}()

	wg.Wait()
}
