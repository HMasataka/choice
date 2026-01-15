package rtcp

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
)

func TestNewPLIFIRHandler(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		h := NewPLIFIRHandler(nil)
		if h == nil {
			t.Fatal("expected non-nil handler")
		}
		if h.config.MinInterval != 100*time.Millisecond {
			t.Errorf("expected 100ms min interval, got %v", h.config.MinInterval)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &PLIFIRConfig{
			MinInterval:        200 * time.Millisecond,
			MaxPendingRequests: 20,
		}
		h := NewPLIFIRHandler(cfg)
		if h.config.MinInterval != 200*time.Millisecond {
			t.Errorf("expected 200ms min interval, got %v", h.config.MinInterval)
		}
	})
}

func TestPLIFIRHandler_HandlePLI(t *testing.T) {
	h := NewPLIFIRHandler(nil)

	var receivedRequest *KeyframeRequest
	h.SetOnKeyframeRequest(func(request *KeyframeRequest) {
		receivedRequest = request
	})

	pli := &rtcp.PictureLossIndication{
		SenderSSRC: 12345,
		MediaSSRC:  67890,
	}
	h.HandlePLI(pli)

	if receivedRequest == nil {
		t.Fatal("expected keyframe request callback")
	}
	if receivedRequest.SSRC != 67890 {
		t.Errorf("expected SSRC 67890, got %d", receivedRequest.SSRC)
	}
	if receivedRequest.Source != "pli" {
		t.Errorf("expected source 'pli', got %s", receivedRequest.Source)
	}
	if receivedRequest.SenderSSRC != 12345 {
		t.Errorf("expected sender SSRC 12345, got %d", receivedRequest.SenderSSRC)
	}

	stats := h.GetStats()
	if stats.TotalPLIsReceived != 1 {
		t.Errorf("expected 1 PLI received, got %d", stats.TotalPLIsReceived)
	}
}

func TestPLIFIRHandler_HandleFIR(t *testing.T) {
	h := NewPLIFIRHandler(nil)

	var receivedRequests []*KeyframeRequest
	h.SetOnKeyframeRequest(func(request *KeyframeRequest) {
		receivedRequests = append(receivedRequests, request)
	})

	fir := &rtcp.FullIntraRequest{
		SenderSSRC: 12345,
		FIR: []rtcp.FIREntry{
			{SSRC: 67890, SequenceNumber: 1},
			{SSRC: 11111, SequenceNumber: 2},
		},
	}
	h.HandleFIR(fir)

	if len(receivedRequests) != 2 {
		t.Fatalf("expected 2 keyframe requests, got %d", len(receivedRequests))
	}

	if receivedRequests[0].SSRC != 67890 {
		t.Errorf("expected first SSRC 67890, got %d", receivedRequests[0].SSRC)
	}
	if receivedRequests[0].Source != "fir" {
		t.Errorf("expected source 'fir', got %s", receivedRequests[0].Source)
	}
	if receivedRequests[0].FIRSequenceNumber != 1 {
		t.Errorf("expected FIR seq 1, got %d", receivedRequests[0].FIRSequenceNumber)
	}

	if receivedRequests[1].SSRC != 11111 {
		t.Errorf("expected second SSRC 11111, got %d", receivedRequests[1].SSRC)
	}

	stats := h.GetStats()
	if stats.TotalFIRsReceived != 1 {
		t.Errorf("expected 1 FIR received, got %d", stats.TotalFIRsReceived)
	}
}

func TestPLIFIRHandler_RateLimiting(t *testing.T) {
	cfg := &PLIFIRConfig{
		MinInterval:        100 * time.Millisecond,
		MaxPendingRequests: 10,
	}
	h := NewPLIFIRHandler(cfg)

	var requestCount int
	h.SetOnKeyframeRequest(func(request *KeyframeRequest) {
		requestCount++
	})

	// First request should go through
	pli1 := &rtcp.PictureLossIndication{MediaSSRC: 12345}
	h.HandlePLI(pli1)

	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}

	// Second request within interval should be throttled
	pli2 := &rtcp.PictureLossIndication{MediaSSRC: 12345}
	h.HandlePLI(pli2)

	if requestCount != 1 {
		t.Errorf("expected still 1 request (throttled), got %d", requestCount)
	}

	stats := h.GetStats()
	if stats.TotalRequestsThrottled != 1 {
		t.Errorf("expected 1 throttled request, got %d", stats.TotalRequestsThrottled)
	}

	// Wait for interval to pass
	time.Sleep(150 * time.Millisecond)

	// Third request should go through
	pli3 := &rtcp.PictureLossIndication{MediaSSRC: 12345}
	h.HandlePLI(pli3)

	if requestCount != 2 {
		t.Errorf("expected 2 requests after interval, got %d", requestCount)
	}
}

func TestPLIFIRHandler_GeneratePLI(t *testing.T) {
	h := NewPLIFIRHandler(nil)

	pli := h.GeneratePLI(12345, 67890)

	if pli.SenderSSRC != 12345 {
		t.Errorf("expected sender SSRC 12345, got %d", pli.SenderSSRC)
	}
	if pli.MediaSSRC != 67890 {
		t.Errorf("expected media SSRC 67890, got %d", pli.MediaSSRC)
	}

	stats := h.GetStats()
	if stats.TotalPLIsSent != 1 {
		t.Errorf("expected 1 PLI sent, got %d", stats.TotalPLIsSent)
	}
}

func TestPLIFIRHandler_GenerateFIR(t *testing.T) {
	h := NewPLIFIRHandler(nil)

	fir := h.GenerateFIR(12345, []uint32{67890, 11111})

	if fir.SenderSSRC != 12345 {
		t.Errorf("expected sender SSRC 12345, got %d", fir.SenderSSRC)
	}
	if len(fir.FIR) != 2 {
		t.Fatalf("expected 2 FIR entries, got %d", len(fir.FIR))
	}
	if fir.FIR[0].SSRC != 67890 {
		t.Errorf("expected first FIR SSRC 67890, got %d", fir.FIR[0].SSRC)
	}
	if fir.FIR[1].SSRC != 11111 {
		t.Errorf("expected second FIR SSRC 11111, got %d", fir.FIR[1].SSRC)
	}

	// Sequence numbers should be sequential
	fir2 := h.GenerateFIR(12345, []uint32{67890})
	if fir2.FIR[0].SequenceNumber != 1 {
		t.Errorf("expected FIR seq 1, got %d", fir2.FIR[0].SequenceNumber)
	}

	stats := h.GetStats()
	if stats.TotalFIRsSent != 2 {
		t.Errorf("expected 2 FIRs sent, got %d", stats.TotalFIRsSent)
	}
}

func TestPLIFIRHandler_RequestKeyframe(t *testing.T) {
	h := NewPLIFIRHandler(nil)

	t.Run("generates PLI", func(t *testing.T) {
		pkt := h.RequestKeyframe(12345, 67890, false)
		pli, ok := pkt.(*rtcp.PictureLossIndication)
		if !ok {
			t.Fatal("expected PLI packet")
		}
		if pli.MediaSSRC != 67890 {
			t.Errorf("expected media SSRC 67890, got %d", pli.MediaSSRC)
		}
	})

	t.Run("generates FIR", func(t *testing.T) {
		pkt := h.RequestKeyframe(12345, 67890, true)
		fir, ok := pkt.(*rtcp.FullIntraRequest)
		if !ok {
			t.Fatal("expected FIR packet")
		}
		if len(fir.FIR) != 1 || fir.FIR[0].SSRC != 67890 {
			t.Errorf("expected FIR for SSRC 67890")
		}
	})
}

func TestPLIFIRHandler_PendingRequests(t *testing.T) {
	h := NewPLIFIRHandler(nil)

	// Handle PLI to create pending request
	pli := &rtcp.PictureLossIndication{
		SenderSSRC: 12345,
		MediaSSRC:  67890,
	}
	h.HandlePLI(pli)

	// Check pending requests
	pending := h.GetPendingRequests(67890)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}
	if pending[0].SSRC != 67890 {
		t.Errorf("expected SSRC 67890, got %d", pending[0].SSRC)
	}

	// Clear pending requests
	h.ClearPendingRequests(67890)

	// Should be empty now
	pending = h.GetPendingRequests(67890)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending requests after clear, got %d", len(pending))
	}
}

func TestPLIFIRHandler_MaxPendingRequests(t *testing.T) {
	cfg := &PLIFIRConfig{
		MinInterval:        1 * time.Millisecond,
		MaxPendingRequests: 3,
	}
	h := NewPLIFIRHandler(cfg)

	// Add more than max pending requests
	for i := 0; i < 5; i++ {
		pli := &rtcp.PictureLossIndication{
			SenderSSRC: 12345,
			MediaSSRC:  67890,
		}
		h.HandlePLI(pli)
		time.Sleep(2 * time.Millisecond) // Wait for rate limiter
	}

	pending := h.GetPendingRequests(67890)
	if len(pending) > 3 {
		t.Errorf("expected max 3 pending requests, got %d", len(pending))
	}
}

func TestPLIFIRHandler_ShouldRequestKeyframe(t *testing.T) {
	cfg := &PLIFIRConfig{
		MinInterval: 100 * time.Millisecond,
	}
	h := NewPLIFIRHandler(cfg)

	// First request should be allowed
	if !h.ShouldRequestKeyframe(12345, "new_subscriber") {
		t.Error("expected first keyframe request to be allowed")
	}

	// Handle PLI to update last request time
	pli := &rtcp.PictureLossIndication{MediaSSRC: 12345}
	h.HandlePLI(pli)

	// Immediate second request should not be allowed
	if h.ShouldRequestKeyframe(12345, "packet_loss") {
		t.Error("expected immediate second request to not be allowed")
	}

	// After interval, should be allowed
	time.Sleep(150 * time.Millisecond)
	if !h.ShouldRequestKeyframe(12345, "packet_loss") {
		t.Error("expected request to be allowed after interval")
	}
}

func TestPLIFIRHandler_GetLastRequestTime(t *testing.T) {
	h := NewPLIFIRHandler(nil)

	// No request yet
	_, ok := h.GetLastRequestTime(12345)
	if ok {
		t.Error("expected no last request time for new SSRC")
	}

	// Handle PLI
	pli := &rtcp.PictureLossIndication{MediaSSRC: 12345}
	h.HandlePLI(pli)

	// Should have last request time now
	lastTime, ok := h.GetLastRequestTime(12345)
	if !ok {
		t.Error("expected last request time after PLI")
	}
	if time.Since(lastTime) > 100*time.Millisecond {
		t.Error("last request time should be recent")
	}
}

func TestPLIFIRHandler_Reset(t *testing.T) {
	h := NewPLIFIRHandler(nil)

	// Generate some state
	pli := &rtcp.PictureLossIndication{MediaSSRC: 12345}
	h.HandlePLI(pli)
	h.GenerateFIR(12345, []uint32{67890})

	// Verify state exists
	stats := h.GetStats()
	if stats.TotalPLIsReceived == 0 {
		t.Error("expected non-zero PLIs before reset")
	}

	// Reset
	h.Reset()

	// Verify state cleared
	stats = h.GetStats()
	if stats.TotalPLIsReceived != 0 {
		t.Errorf("expected 0 PLIs after reset, got %d", stats.TotalPLIsReceived)
	}
	if stats.TotalFIRsSent != 0 {
		t.Errorf("expected 0 FIRs sent after reset, got %d", stats.TotalFIRsSent)
	}

	pending := h.GetPendingRequests(12345)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending requests after reset, got %d", len(pending))
	}
}

func TestDefaultPLIFIRConfig(t *testing.T) {
	cfg := DefaultPLIFIRConfig()

	if cfg.MinInterval != 100*time.Millisecond {
		t.Errorf("expected 100ms min interval, got %v", cfg.MinInterval)
	}
	if cfg.MaxPendingRequests != 10 {
		t.Errorf("expected 10 max pending requests, got %d", cfg.MaxPendingRequests)
	}
}
