package rtcp

import (
	"sync"
	"time"

	"github.com/pion/rtcp"
)

// PLIFIRConfig contains configuration for PLI/FIR handling.
type PLIFIRConfig struct {
	// MinInterval is the minimum interval between keyframe requests.
	// This prevents flooding the publisher with requests.
	MinInterval time.Duration

	// MaxPendingRequests is the maximum number of pending requests per SSRC.
	MaxPendingRequests int
}

// DefaultPLIFIRConfig returns the default PLI/FIR configuration.
func DefaultPLIFIRConfig() *PLIFIRConfig {
	return &PLIFIRConfig{
		MinInterval:        100 * time.Millisecond,
		MaxPendingRequests: 10,
	}
}

// KeyframeRequest represents a pending keyframe request.
type KeyframeRequest struct {
	// SSRC is the media SSRC for which keyframe is requested.
	SSRC uint32

	// RequestTime is when the request was made.
	RequestTime time.Time

	// Source indicates the request source ("pli" or "fir").
	Source string

	// SenderSSRC is the SSRC of the requester.
	SenderSSRC uint32

	// FIRSequenceNumber is the sequence number for FIR (only used for FIR).
	FIRSequenceNumber uint8
}

// PLIFIRHandler handles Picture Loss Indication and Full Intra Request.
// Per tasks.md 3.2.5: PLI/FIR processing and keyframe request forwarding.
type PLIFIRHandler struct {
	mu     sync.RWMutex
	config *PLIFIRConfig

	// lastRequestTime stores the last request time per SSRC.
	lastRequestTime map[uint32]time.Time

	// pendingRequests stores pending keyframe requests per SSRC.
	pendingRequests map[uint32][]*KeyframeRequest

	// firSequenceNumbers stores the next FIR sequence number per SSRC.
	firSequenceNumbers map[uint32]uint8

	// stats contains PLI/FIR statistics.
	stats PLIFIRStats

	// onKeyframeRequest is called when a keyframe is requested.
	onKeyframeRequest func(request *KeyframeRequest)
}

// PLIFIRStats contains PLI/FIR statistics.
type PLIFIRStats struct {
	// TotalPLIsReceived is the total number of PLI packets received.
	TotalPLIsReceived uint64

	// TotalFIRsReceived is the total number of FIR packets received.
	TotalFIRsReceived uint64

	// TotalPLIsSent is the total number of PLI packets sent.
	TotalPLIsSent uint64

	// TotalFIRsSent is the total number of FIR packets sent.
	TotalFIRsSent uint64

	// TotalRequestsThrottled is the number of requests throttled due to rate limiting.
	TotalRequestsThrottled uint64

	// LastPLITime is the time of the last PLI received.
	LastPLITime time.Time

	// LastFIRTime is the time of the last FIR received.
	LastFIRTime time.Time
}

// NewPLIFIRHandler creates a new PLI/FIR handler.
func NewPLIFIRHandler(cfg *PLIFIRConfig) *PLIFIRHandler {
	if cfg == nil {
		cfg = DefaultPLIFIRConfig()
	}

	return &PLIFIRHandler{
		config:             cfg,
		lastRequestTime:    make(map[uint32]time.Time),
		pendingRequests:    make(map[uint32][]*KeyframeRequest),
		firSequenceNumbers: make(map[uint32]uint8),
	}
}

// HandlePLI processes a Picture Loss Indication packet.
func (p *PLIFIRHandler) HandlePLI(pli *rtcp.PictureLossIndication) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.TotalPLIsReceived++
	p.stats.LastPLITime = time.Now()

	ssrc := pli.MediaSSRC

	// Check rate limiting
	if !p.shouldAllowRequest(ssrc) {
		p.stats.TotalRequestsThrottled++
		return
	}

	// Update last request time
	p.lastRequestTime[ssrc] = time.Now()

	// Create keyframe request
	request := &KeyframeRequest{
		SSRC:        ssrc,
		RequestTime: time.Now(),
		Source:      "pli",
		SenderSSRC:  pli.SenderSSRC,
	}

	// Add to pending requests
	p.addPendingRequest(ssrc, request)

	// Notify callback
	if p.onKeyframeRequest != nil {
		p.onKeyframeRequest(request)
	}
}

// HandleFIR processes a Full Intra Request packet.
func (p *PLIFIRHandler) HandleFIR(fir *rtcp.FullIntraRequest) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.TotalFIRsReceived++
	p.stats.LastFIRTime = time.Now()

	// Process each FIR entry
	for _, entry := range fir.FIR {
		ssrc := entry.SSRC

		// Check rate limiting
		if !p.shouldAllowRequest(ssrc) {
			p.stats.TotalRequestsThrottled++
			continue
		}

		// Update last request time
		p.lastRequestTime[ssrc] = time.Now()

		// Create keyframe request
		request := &KeyframeRequest{
			SSRC:              ssrc,
			RequestTime:       time.Now(),
			Source:            "fir",
			SenderSSRC:        fir.SenderSSRC,
			FIRSequenceNumber: entry.SequenceNumber,
		}

		// Add to pending requests
		p.addPendingRequest(ssrc, request)

		// Notify callback
		if p.onKeyframeRequest != nil {
			p.onKeyframeRequest(request)
		}
	}
}

// shouldAllowRequest checks if a request should be allowed based on rate limiting.
func (p *PLIFIRHandler) shouldAllowRequest(ssrc uint32) bool {
	lastTime, ok := p.lastRequestTime[ssrc]
	if !ok {
		return true
	}

	return time.Since(lastTime) >= p.config.MinInterval
}

// addPendingRequest adds a request to the pending queue.
func (p *PLIFIRHandler) addPendingRequest(ssrc uint32, request *KeyframeRequest) {
	p.pendingRequests[ssrc] = append(p.pendingRequests[ssrc], request)

	// Enforce max pending requests
	if len(p.pendingRequests[ssrc]) > p.config.MaxPendingRequests {
		p.pendingRequests[ssrc] = p.pendingRequests[ssrc][1:]
	}
}

// GeneratePLI generates a PLI packet.
func (p *PLIFIRHandler) GeneratePLI(senderSSRC, mediaSSRC uint32) *rtcp.PictureLossIndication {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.TotalPLIsSent++

	return &rtcp.PictureLossIndication{
		SenderSSRC: senderSSRC,
		MediaSSRC:  mediaSSRC,
	}
}

// GenerateFIR generates a FIR packet.
func (p *PLIFIRHandler) GenerateFIR(senderSSRC uint32, mediaSSRCs []uint32) *rtcp.FullIntraRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.TotalFIRsSent++

	var entries []rtcp.FIREntry
	for _, ssrc := range mediaSSRCs {
		// Get and increment sequence number for this SSRC
		seqNum := p.firSequenceNumbers[ssrc]
		p.firSequenceNumbers[ssrc] = seqNum + 1

		entries = append(entries, rtcp.FIREntry{
			SSRC:           ssrc,
			SequenceNumber: seqNum,
		})
	}

	return &rtcp.FullIntraRequest{
		SenderSSRC: senderSSRC,
		MediaSSRC:  0, // Not used when FIR entries are present
		FIR:        entries,
	}
}

// RequestKeyframe requests a keyframe for the given SSRC.
// It will generate either a PLI or FIR based on configuration.
// Returns the generated RTCP packet.
func (p *PLIFIRHandler) RequestKeyframe(senderSSRC, mediaSSRC uint32, useFIR bool) rtcp.Packet {
	if useFIR {
		return p.GenerateFIR(senderSSRC, []uint32{mediaSSRC})
	}
	return p.GeneratePLI(senderSSRC, mediaSSRC)
}

// GetStats returns PLI/FIR statistics.
func (p *PLIFIRHandler) GetStats() PLIFIRStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// GetPendingRequests returns pending keyframe requests for an SSRC.
func (p *PLIFIRHandler) GetPendingRequests(ssrc uint32) []*KeyframeRequest {
	p.mu.RLock()
	defer p.mu.RUnlock()

	requests := p.pendingRequests[ssrc]
	if len(requests) == 0 {
		return nil
	}

	// Return a copy
	result := make([]*KeyframeRequest, len(requests))
	for i, req := range requests {
		result[i] = &KeyframeRequest{
			SSRC:              req.SSRC,
			RequestTime:       req.RequestTime,
			Source:            req.Source,
			SenderSSRC:        req.SenderSSRC,
			FIRSequenceNumber: req.FIRSequenceNumber,
		}
	}
	return result
}

// ClearPendingRequests clears pending requests for an SSRC.
// Call this after successfully sending a keyframe.
func (p *PLIFIRHandler) ClearPendingRequests(ssrc uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pendingRequests, ssrc)
}

// SetOnKeyframeRequest sets the callback for keyframe requests.
func (p *PLIFIRHandler) SetOnKeyframeRequest(cb func(request *KeyframeRequest)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onKeyframeRequest = cb
}

// ShouldRequestKeyframe determines if a keyframe should be requested.
// This is based on various factors like packet loss, new subscriber, etc.
func (p *PLIFIRHandler) ShouldRequestKeyframe(ssrc uint32, reason string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Always allow if this is the first request
	lastTime, ok := p.lastRequestTime[ssrc]
	if !ok {
		return true
	}

	// Check if enough time has passed since last request
	if time.Since(lastTime) < p.config.MinInterval {
		return false
	}

	return true
}

// GetLastRequestTime returns the last keyframe request time for an SSRC.
func (p *PLIFIRHandler) GetLastRequestTime(ssrc uint32) (time.Time, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	t, ok := p.lastRequestTime[ssrc]
	return t, ok
}

// Reset clears all state.
func (p *PLIFIRHandler) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastRequestTime = make(map[uint32]time.Time)
	p.pendingRequests = make(map[uint32][]*KeyframeRequest)
	p.firSequenceNumbers = make(map[uint32]uint8)
	p.stats = PLIFIRStats{}
}
