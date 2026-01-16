package rtcp

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
)

func TestNewHandler(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		h := NewHandler(nil)
		if h == nil {
			t.Fatal("expected non-nil handler")
		}
		if h.config == nil {
			t.Fatal("expected non-nil config")
		}
		if h.config.ReportInterval != 100*time.Millisecond {
			t.Errorf("expected 100ms report interval, got %v", h.config.ReportInterval)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &HandlerConfig{
			ReportInterval: 200 * time.Millisecond,
			TWCCEnabled:    false,
			REMBEnabled:    true,
			NACKEnabled:    true,
			PLIFIREnabled:  true,
		}
		h := NewHandler(cfg)
		if h.config.ReportInterval != 200*time.Millisecond {
			t.Errorf("expected 200ms report interval, got %v", h.config.ReportInterval)
		}
		if h.twcc != nil {
			t.Error("expected TWCC handler to be nil when disabled")
		}
	})

	t.Run("sub-handlers initialized based on config", func(t *testing.T) {
		cfg := DefaultHandlerConfig()
		h := NewHandler(cfg)

		if h.twcc == nil {
			t.Error("expected TWCC handler to be initialized")
		}
		if h.remb == nil {
			t.Error("expected REMB handler to be initialized")
		}
		if h.nack == nil {
			t.Error("expected NACK handler to be initialized")
		}
		if h.plifir == nil {
			t.Error("expected PLI/FIR handler to be initialized")
		}
	})
}

func TestHandler_StartStop(t *testing.T) {
	h := NewHandler(nil)

	// Start
	h.Start()
	h.mu.RLock()
	if !h.running {
		t.Error("expected handler to be running after Start")
	}
	h.mu.RUnlock()

	// Start again (should be idempotent)
	h.Start()

	// Stop
	h.Stop()
	h.mu.RLock()
	if h.running {
		t.Error("expected handler to be stopped after Stop")
	}
	h.mu.RUnlock()

	// Stop again (should be idempotent)
	h.Stop()
}

func TestHandler_HandlePackets(t *testing.T) {
	t.Run("processes sender report", func(t *testing.T) {
		h := NewHandler(nil)
		sr := &rtcp.SenderReport{
			SSRC:        12345,
			NTPTime:     0,
			RTPTime:     0,
			PacketCount: 100,
			OctetCount:  10000,
		}

		h.HandlePackets([]rtcp.Packet{sr})

		stats := h.GetStats(12345)
		if stats == nil {
			t.Fatal("expected stats for SSRC 12345")
		}
		if stats.SSRC != 12345 {
			t.Errorf("expected SSRC 12345, got %d", stats.SSRC)
		}
	})

	t.Run("processes receiver report", func(t *testing.T) {
		h := NewHandler(nil)
		rr := &rtcp.ReceiverReport{
			SSRC: 12345,
			Reports: []rtcp.ReceptionReport{
				{
					SSRC:               67890,
					FractionLost:       25, // ~10% loss
					TotalLost:          100,
					LastSequenceNumber: 1000,
					Jitter:             50,
				},
			},
		}

		h.HandlePackets([]rtcp.Packet{rr})

		stats := h.GetStats(67890)
		if stats == nil {
			t.Fatal("expected stats for SSRC 67890")
		}
		if stats.PacketsLost != 100 {
			t.Errorf("expected 100 packets lost, got %d", stats.PacketsLost)
		}
		// FractionLost is 25/256 ≈ 0.098
		if stats.FractionLost < 0.09 || stats.FractionLost > 0.11 {
			t.Errorf("expected fraction lost ~0.1, got %f", stats.FractionLost)
		}
	})

	t.Run("callback invoked", func(t *testing.T) {
		h := NewHandler(nil)
		var called bool
		var receivedPackets []rtcp.Packet

		h.SetOnPacketCallback(func(packets []rtcp.Packet) {
			called = true
			receivedPackets = packets
		})

		sr := &rtcp.SenderReport{SSRC: 12345}
		h.HandlePackets([]rtcp.Packet{sr})

		if !called {
			t.Error("expected callback to be invoked")
		}
		if len(receivedPackets) != 1 {
			t.Errorf("expected 1 packet, got %d", len(receivedPackets))
		}
	})
}

func TestHandler_PLI(t *testing.T) {
	h := NewHandler(nil)
	var keyframeSSRC uint32

	h.SetOnKeyframeRequestCallback(func(ssrc uint32) {
		keyframeSSRC = ssrc
	})

	pli := &rtcp.PictureLossIndication{
		SenderSSRC: 12345,
		MediaSSRC:  67890,
	}

	h.HandlePackets([]rtcp.Packet{pli})

	if keyframeSSRC != 67890 {
		t.Errorf("expected keyframe request for SSRC 67890, got %d", keyframeSSRC)
	}
}

func TestHandler_FIR(t *testing.T) {
	h := NewHandler(nil)
	var keyframeSSRCs []uint32

	h.SetOnKeyframeRequestCallback(func(ssrc uint32) {
		keyframeSSRCs = append(keyframeSSRCs, ssrc)
	})

	fir := &rtcp.FullIntraRequest{
		SenderSSRC: 12345,
		FIR: []rtcp.FIREntry{
			{SSRC: 67890, SequenceNumber: 1},
			{SSRC: 11111, SequenceNumber: 2},
		},
	}

	h.HandlePackets([]rtcp.Packet{fir})

	if len(keyframeSSRCs) != 2 {
		t.Errorf("expected 2 keyframe requests, got %d", len(keyframeSSRCs))
	}
}

func TestHandler_NACK(t *testing.T) {
	h := NewHandler(nil)
	var nackSSRC uint32
	var nackSeqNums []uint16

	h.SetOnNACKCallback(func(ssrc uint32, nacks []uint16) {
		nackSSRC = ssrc
		nackSeqNums = nacks
	})

	nack := &rtcp.TransportLayerNack{
		SenderSSRC: 12345,
		MediaSSRC:  67890,
		Nacks: []rtcp.NackPair{
			{PacketID: 100, LostPackets: 0b0000000000000101}, // 100, 101, 103
		},
	}

	h.HandlePackets([]rtcp.Packet{nack})

	if nackSSRC != 67890 {
		t.Errorf("expected NACK for SSRC 67890, got %d", nackSSRC)
	}
	if len(nackSeqNums) != 3 {
		t.Errorf("expected 3 sequence numbers, got %d: %v", len(nackSeqNums), nackSeqNums)
	}
}

func TestHandler_REMB(t *testing.T) {
	h := NewHandler(nil)
	var estimates []struct {
		ssrc    uint32
		bitrate uint64
	}

	h.SetOnBandwidthEstimateCallback(func(ssrc uint32, bitrate uint64) {
		estimates = append(estimates, struct {
			ssrc    uint32
			bitrate uint64
		}{ssrc, bitrate})
	})

	remb := &rtcp.ReceiverEstimatedMaximumBitrate{
		SenderSSRC: 12345,
		Bitrate:    1_000_000, // 1 Mbps
		SSRCs:      []uint32{67890, 11111},
	}

	h.HandlePackets([]rtcp.Packet{remb})

	if len(estimates) != 2 {
		t.Errorf("expected 2 bandwidth estimates, got %d", len(estimates))
	}
}

func TestHandler_UpdatePacketStats(t *testing.T) {
	h := NewHandler(nil)

	h.UpdatePacketStats(12345, 100, 50000)
	h.UpdatePacketStats(12345, 50, 25000)

	stats := h.GetStats(12345)
	if stats == nil {
		t.Fatal("expected stats for SSRC 12345")
	}
	if stats.PacketsReceived != 150 {
		t.Errorf("expected 150 packets received, got %d", stats.PacketsReceived)
	}
	if stats.BytesReceived != 75000 {
		t.Errorf("expected 75000 bytes received, got %d", stats.BytesReceived)
	}
}

func TestHandler_GetAllStats(t *testing.T) {
	h := NewHandler(nil)

	h.UpdatePacketStats(12345, 100, 50000)
	h.UpdatePacketStats(67890, 200, 100000)

	allStats := h.GetAllStats()
	if len(allStats) != 2 {
		t.Errorf("expected 2 SSRCs, got %d", len(allStats))
	}

	if allStats[12345] == nil || allStats[12345].PacketsReceived != 100 {
		t.Error("expected correct stats for SSRC 12345")
	}
	if allStats[67890] == nil || allStats[67890].PacketsReceived != 200 {
		t.Error("expected correct stats for SSRC 67890")
	}
}

func TestHandler_GetBandwidthEstimate(t *testing.T) {
	t.Run("TWCC handler has initial estimate", func(t *testing.T) {
		h := NewHandler(nil)

		// TWCC has an initial bandwidth estimate of MaxBitrate/2
		twccHandler := h.GetTWCCHandler()
		if twccHandler == nil {
			t.Fatal("expected TWCC handler")
		}
		estimate := twccHandler.GetBandwidthEstimate()
		// Initial TWCC bandwidth estimate is MaxBitrate/2 = 25Mbps
		expectedInitial := uint64(50_000_000 / 2)
		if estimate != expectedInitial {
			t.Errorf("expected initial TWCC estimate %d, got %d", expectedInitial, estimate)
		}
	})

	t.Run("REMB handler returns estimate when set", func(t *testing.T) {
		cfg := &HandlerConfig{
			TWCCEnabled: false,
			REMBEnabled: true,
		}
		h := NewHandler(cfg)

		remb := &rtcp.ReceiverEstimatedMaximumBitrate{
			Bitrate: 1_000_000,
			SSRCs:   []uint32{12345},
		}
		h.HandlePackets([]rtcp.Packet{remb})

		// Get REMB handler directly
		rembHandler := h.GetREMBHandler()
		if rembHandler == nil {
			t.Fatal("expected REMB handler")
		}
		estimate := rembHandler.GetBandwidthEstimate()
		if estimate != 1_000_000 {
			t.Errorf("expected REMB estimate 1000000, got %d", estimate)
		}
	})
}

func TestHandler_GetSubHandlers(t *testing.T) {
	h := NewHandler(nil)

	if h.GetTWCCHandler() == nil {
		t.Error("expected non-nil TWCC handler")
	}
	if h.GetREMBHandler() == nil {
		t.Error("expected non-nil REMB handler")
	}
	if h.GetNACKHandler() == nil {
		t.Error("expected non-nil NACK handler")
	}
	if h.GetPLIFIRHandler() == nil {
		t.Error("expected non-nil PLI/FIR handler")
	}
}

func TestHandler_ConcurrentAccess(t *testing.T) {
	h := NewHandler(nil)
	h.Start()
	defer h.Stop()

	var wg sync.WaitGroup
	numGoroutines := 10
	numIterations := 100

	// Concurrent packet handling
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				sr := &rtcp.SenderReport{SSRC: uint32(id*1000 + j)}
				h.HandlePackets([]rtcp.Packet{sr})
			}
		}(i)
	}

	// Concurrent stats reading
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = h.GetAllStats()
				_ = h.GetBandwidthEstimate()
			}
		}()
	}

	wg.Wait()
}

func TestDefaultHandlerConfig(t *testing.T) {
	cfg := DefaultHandlerConfig()

	if cfg.ReportInterval != 100*time.Millisecond {
		t.Errorf("expected 100ms report interval, got %v", cfg.ReportInterval)
	}
	if cfg.MaxPacketBufferSize != 500 {
		t.Errorf("expected 500 max packet buffer size, got %d", cfg.MaxPacketBufferSize)
	}
	if !cfg.TWCCEnabled {
		t.Error("expected TWCC to be enabled by default")
	}
	if !cfg.REMBEnabled {
		t.Error("expected REMB to be enabled by default")
	}
	if !cfg.NACKEnabled {
		t.Error("expected NACK to be enabled by default")
	}
	if !cfg.PLIFIREnabled {
		t.Error("expected PLI/FIR to be enabled by default")
	}
}
