package quality

import (
	"sync"
	"testing"
	"time"
)

func TestNewCalculator(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		c := NewCalculator(nil)
		if c == nil {
			t.Fatal("expected non-nil calculator")
		}
		if c.config == nil {
			t.Fatal("expected non-nil config")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &CalculatorConfig{
			UpdateInterval: 500 * time.Millisecond,
			ExcellentThreshold: QualityThresholds{
				MaxPacketLossRate: 0.001,
				MaxRTT:            50,
				MaxJitter:         10,
			},
		}
		c := NewCalculator(cfg)
		if c.config.UpdateInterval != 500*time.Millisecond {
			t.Errorf("expected update interval 500ms, got %v", c.config.UpdateInterval)
		}
	})
}

func TestCalculator_UpdateMetrics(t *testing.T) {
	c := NewCalculator(nil)

	metrics := QualityMetrics{
		PacketLossRate: 0.0001,
		RTT:            30,
		Jitter:         5,
		Timestamp:      time.Now(),
	}

	result := c.UpdateMetrics("participant-1", metrics)

	if result.ParticipantID != "participant-1" {
		t.Errorf("expected participant-1, got %s", result.ParticipantID)
	}
	if result.Quality != ConnectionQualityExcellent {
		t.Errorf("expected excellent quality, got %s", result.Quality)
	}
}

func TestCalculator_QualityLevels(t *testing.T) {
	tests := []struct {
		name     string
		metrics  QualityMetrics
		expected ConnectionQuality
	}{
		{
			name: "excellent quality",
			metrics: QualityMetrics{
				PacketLossRate: 0.0005,
				RTT:            40,
				Jitter:         5,
			},
			expected: ConnectionQualityExcellent,
		},
		{
			name: "good quality",
			metrics: QualityMetrics{
				PacketLossRate: 0.005,
				RTT:            100,
				Jitter:         20,
			},
			expected: ConnectionQualityGood,
		},
		{
			name: "fair quality",
			metrics: QualityMetrics{
				PacketLossRate: 0.03,
				RTT:            200,
				Jitter:         40,
			},
			expected: ConnectionQualityFair,
		},
		{
			name: "poor quality - high loss",
			metrics: QualityMetrics{
				PacketLossRate: 0.10,
				RTT:            100,
				Jitter:         20,
			},
			expected: ConnectionQualityPoor,
		},
		{
			name: "poor quality - high RTT",
			metrics: QualityMetrics{
				PacketLossRate: 0.01,
				RTT:            500,
				Jitter:         20,
			},
			expected: ConnectionQualityPoor,
		},
		{
			name: "poor quality - high jitter",
			metrics: QualityMetrics{
				PacketLossRate: 0.01,
				RTT:            100,
				Jitter:         100,
			},
			expected: ConnectionQualityPoor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCalculator(nil)
			result := c.UpdateMetrics("test-participant", tt.metrics)
			if result.Quality != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result.Quality)
			}
		})
	}
}

func TestCalculator_QualityChanged(t *testing.T) {
	c := NewCalculator(nil)

	var changes []ConnectionQualityResult
	c.SetOnQualityChanged(func(result ConnectionQualityResult) {
		changes = append(changes, result)
	})

	// First update - no previous quality, so Changed should be false
	metrics1 := QualityMetrics{
		PacketLossRate: 0.0001,
		RTT:            30,
		Jitter:         5,
	}
	c.UpdateMetrics("participant-1", metrics1)

	// Second update with different quality - Changed should be true
	metrics2 := QualityMetrics{
		PacketLossRate: 0.10,
		RTT:            500,
		Jitter:         100,
	}
	c.UpdateMetrics("participant-1", metrics2)

	if len(changes) != 1 {
		t.Errorf("expected 1 change notification, got %d", len(changes))
	}
	if len(changes) > 0 {
		if changes[0].PreviousQuality != ConnectionQualityExcellent {
			t.Errorf("expected previous quality excellent, got %s", changes[0].PreviousQuality)
		}
		if changes[0].Quality != ConnectionQualityPoor {
			t.Errorf("expected current quality poor, got %s", changes[0].Quality)
		}
	}
}

func TestCalculator_GetQuality(t *testing.T) {
	c := NewCalculator(nil)

	// No metrics yet
	_, found := c.GetQuality("participant-1")
	if found {
		t.Error("expected not found for unknown participant")
	}

	// Add metrics
	c.UpdateMetrics("participant-1", QualityMetrics{
		PacketLossRate: 0.0001,
		RTT:            30,
		Jitter:         5,
	})

	quality, found := c.GetQuality("participant-1")
	if !found {
		t.Error("expected to find participant")
	}
	if quality != ConnectionQualityExcellent {
		t.Errorf("expected excellent, got %s", quality)
	}
}

func TestCalculator_GetAllQualities(t *testing.T) {
	c := NewCalculator(nil)

	c.UpdateMetrics("participant-1", QualityMetrics{
		PacketLossRate: 0.0001,
		RTT:            30,
		Jitter:         5,
	})
	c.UpdateMetrics("participant-2", QualityMetrics{
		PacketLossRate: 0.10,
		RTT:            500,
		Jitter:         100,
	})

	qualities := c.GetAllQualities()
	if len(qualities) != 2 {
		t.Errorf("expected 2 qualities, got %d", len(qualities))
	}
	if qualities["participant-1"] != ConnectionQualityExcellent {
		t.Errorf("expected participant-1 excellent, got %s", qualities["participant-1"])
	}
	if qualities["participant-2"] != ConnectionQualityPoor {
		t.Errorf("expected participant-2 poor, got %s", qualities["participant-2"])
	}
}

func TestCalculator_RemoveParticipant(t *testing.T) {
	c := NewCalculator(nil)

	c.UpdateMetrics("participant-1", QualityMetrics{
		PacketLossRate: 0.0001,
		RTT:            30,
		Jitter:         5,
	})

	_, found := c.GetQuality("participant-1")
	if !found {
		t.Error("expected to find participant before removal")
	}

	c.RemoveParticipant("participant-1")

	_, found = c.GetQuality("participant-1")
	if found {
		t.Error("expected not to find participant after removal")
	}
}

func TestCalculator_Score(t *testing.T) {
	c := NewCalculator(nil)

	// Perfect metrics should give high score
	result := c.UpdateMetrics("participant-1", QualityMetrics{
		PacketLossRate: 0,
		RTT:            0,
		Jitter:         0,
	})
	if result.Score != 100 {
		t.Errorf("expected score 100 for perfect metrics, got %d", result.Score)
	}

	// Bad metrics should give low score
	result = c.UpdateMetrics("participant-2", QualityMetrics{
		PacketLossRate: 0.10,
		RTT:            500,
		Jitter:         100,
	})
	if result.Score >= 50 {
		t.Errorf("expected score < 50 for bad metrics, got %d", result.Score)
	}
}

func TestCalculator_Concurrent(t *testing.T) {
	c := NewCalculator(nil)

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent updates
	for i := 0; i < 5; i++ {
		participantID := "participant-" + string(rune('a'+i))
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.UpdateMetrics(id, QualityMetrics{
					PacketLossRate: float64(j) / 1000,
					RTT:            float64(j),
					Jitter:         float64(j) / 10,
				})
			}
		}(participantID)
	}

	// Concurrent reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = c.GetAllQualities()
		}
	}()

	wg.Wait()
}

func TestCalculator_StartStop(t *testing.T) {
	c := NewCalculator(&CalculatorConfig{
		UpdateInterval:     10 * time.Millisecond,
		ExcellentThreshold: DefaultCalculatorConfig().ExcellentThreshold,
		GoodThreshold:      DefaultCalculatorConfig().GoodThreshold,
		FairThreshold:      DefaultCalculatorConfig().FairThreshold,
		StaleTimeout:       DefaultCalculatorConfig().StaleTimeout,
	})

	c.Start()
	c.Start() // Double start should be safe

	// Add some metrics
	c.UpdateMetrics("participant-1", QualityMetrics{
		PacketLossRate: 0.0001,
		RTT:            30,
		Jitter:         5,
		Timestamp:      time.Now(),
	})

	// Let the update loop run a few times
	time.Sleep(50 * time.Millisecond)

	c.Stop()
	c.Stop() // Double stop should be safe
}
