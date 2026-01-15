package rtcp

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
)

func TestNewTWCCHandler(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		h := NewTWCCHandler(nil)
		if h == nil {
			t.Fatal("expected non-nil handler")
		}
		if h.config.UpdateInterval != 100*time.Millisecond {
			t.Errorf("expected 100ms update interval, got %v", h.config.UpdateInterval)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &TWCCConfig{
			UpdateInterval: 200 * time.Millisecond,
			WindowSize:     2 * time.Second,
			MinBitrate:     50_000,
			MaxBitrate:     100_000_000,
		}
		h := NewTWCCHandler(cfg)
		if h.config.UpdateInterval != 200*time.Millisecond {
			t.Errorf("expected 200ms update interval, got %v", h.config.UpdateInterval)
		}
		if h.config.MaxBitrate != 100_000_000 {
			t.Errorf("expected 100Mbps max bitrate, got %d", h.config.MaxBitrate)
		}
	})
}

func TestTWCCHandler_StartStop(t *testing.T) {
	h := NewTWCCHandler(nil)

	h.Start()
	time.Sleep(10 * time.Millisecond) // Allow goroutine to start

	h.mu.RLock()
	if !h.running {
		t.Error("expected handler to be running")
	}
	h.mu.RUnlock()

	h.Stop()

	h.mu.RLock()
	if h.running {
		t.Error("expected handler to be stopped")
	}
	h.mu.RUnlock()
}

func TestTWCCHandler_RecordPacketSent(t *testing.T) {
	h := NewTWCCHandler(nil)

	seq1 := h.RecordPacketSent(1000)
	seq2 := h.RecordPacketSent(1200)
	seq3 := h.RecordPacketSent(800)

	if seq2 != seq1+1 {
		t.Errorf("expected sequential sequence numbers, got %d, %d", seq1, seq2)
	}
	if seq3 != seq2+1 {
		t.Errorf("expected sequential sequence numbers, got %d, %d", seq2, seq3)
	}

	stats := h.GetStats()
	if stats.TotalBytesSent != 3000 {
		t.Errorf("expected 3000 bytes sent, got %d", stats.TotalBytesSent)
	}
	if stats.PacketsInFlight != 3 {
		t.Errorf("expected 3 packets in flight, got %d", stats.PacketsInFlight)
	}
}

func TestTWCCHandler_HandleFeedback(t *testing.T) {
	h := NewTWCCHandler(nil)

	// Record some packets
	seq1 := h.RecordPacketSent(1000)
	seq2 := h.RecordPacketSent(1000)
	seq3 := h.RecordPacketSent(1000)

	// Create feedback indicating all received
	feedback := &rtcp.TransportLayerCC{
		SenderSSRC:         12345,
		MediaSSRC:          67890,
		BaseSequenceNumber: seq1,
		PacketStatusCount:  3,
		ReferenceTime:      0,
		PacketChunks: []rtcp.PacketStatusChunk{
			&rtcp.RunLengthChunk{
				PacketStatusSymbol: rtcp.TypeTCCPacketReceivedSmallDelta,
				RunLength:          3,
			},
		},
		RecvDeltas: []*rtcp.RecvDelta{
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
			{Type: rtcp.TypeTCCPacketReceivedSmallDelta, Delta: 0},
		},
	}

	h.HandleFeedback(feedback)

	stats := h.GetStats()
	// All 3 packets should be acknowledged
	if stats.TotalBytesAcked != 3000 {
		t.Errorf("expected 3000 bytes acked, got %d", stats.TotalBytesAcked)
	}

	_ = seq2
	_ = seq3
}

func TestTWCCHandler_GetBandwidthEstimate(t *testing.T) {
	h := NewTWCCHandler(nil)

	// Initial estimate should be half of max
	estimate := h.GetBandwidthEstimate()
	expectedInitial := h.config.MaxBitrate / 2
	if estimate != expectedInitial {
		t.Errorf("expected initial estimate %d, got %d", expectedInitial, estimate)
	}
}

func TestTWCCHandler_GetPacketLossRate(t *testing.T) {
	h := NewTWCCHandler(nil)

	// No packets - should return 0
	lossRate := h.GetPacketLossRate()
	if lossRate != 0 {
		t.Errorf("expected 0 loss rate with no packets, got %f", lossRate)
	}
}

func TestTWCCHandler_GenerateFeedback(t *testing.T) {
	h := NewTWCCHandler(nil)

	// No packets - should return nil
	feedback := h.GenerateFeedback(12345, 67890)
	if feedback != nil {
		t.Error("expected nil feedback with no packets")
	}

	// Record some packets
	h.RecordPacketSent(1000)
	h.RecordPacketSent(1000)

	// Should now generate feedback
	feedback = h.GenerateFeedback(12345, 67890)
	if feedback == nil {
		t.Error("expected non-nil feedback")
	}
	if feedback.SenderSSRC != 12345 {
		t.Errorf("expected sender SSRC 12345, got %d", feedback.SenderSSRC)
	}
	if feedback.MediaSSRC != 67890 {
		t.Errorf("expected media SSRC 67890, got %d", feedback.MediaSSRC)
	}
}

func TestTWCCHandler_BandwidthUpdateCallback(t *testing.T) {
	h := NewTWCCHandler(&TWCCConfig{
		UpdateInterval: 10 * time.Millisecond,
		WindowSize:     100 * time.Millisecond,
		MinBitrate:     100_000,
		MaxBitrate:     10_000_000,
	})

	var callbackCalled bool
	var lastBitrate uint64
	var mu sync.Mutex

	h.SetOnBandwidthUpdate(func(bitrate uint64) {
		mu.Lock()
		callbackCalled = true
		lastBitrate = bitrate
		mu.Unlock()
	})

	h.Start()
	defer h.Stop()

	// Wait for at least one update cycle
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	called := callbackCalled
	_ = lastBitrate
	mu.Unlock()

	if !called {
		t.Error("expected bandwidth update callback to be called")
	}
}

func TestTWCCHandler_ConcurrentAccess(t *testing.T) {
	h := NewTWCCHandler(nil)
	h.Start()
	defer h.Stop()

	var wg sync.WaitGroup
	numGoroutines := 10
	numIterations := 100

	// Concurrent packet recording
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				h.RecordPacketSent(1000)
			}
		}()
	}

	// Concurrent stats reading
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = h.GetBandwidthEstimate()
				_ = h.GetPacketLossRate()
				_ = h.GetStats()
			}
		}()
	}

	wg.Wait()
}

func TestDefaultTWCCConfig(t *testing.T) {
	cfg := DefaultTWCCConfig()

	if cfg.UpdateInterval != 100*time.Millisecond {
		t.Errorf("expected 100ms update interval, got %v", cfg.UpdateInterval)
	}
	if cfg.WindowSize != 1*time.Second {
		t.Errorf("expected 1s window size, got %v", cfg.WindowSize)
	}
	if cfg.MinBitrate != 100_000 {
		t.Errorf("expected 100Kbps min bitrate, got %d", cfg.MinBitrate)
	}
	if cfg.MaxBitrate != 50_000_000 {
		t.Errorf("expected 50Mbps max bitrate, got %d", cfg.MaxBitrate)
	}
}
