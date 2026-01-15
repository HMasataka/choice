// Package rtcp provides RTCP packet processing for the SFU.
// Per design.md section 3.6.3 and tasks.md Phase 3.2.
package rtcp

import (
	"context"
	"sync"
	"time"

	"github.com/pion/rtcp"
)

// PacketType represents the type of RTCP packet.
type PacketType string

const (
	// PacketTypeSR is a Sender Report.
	PacketTypeSR PacketType = "SR"
	// PacketTypeRR is a Receiver Report.
	PacketTypeRR PacketType = "RR"
	// PacketTypeSDES is a Source Description.
	PacketTypeSDES PacketType = "SDES"
	// PacketTypeBYE is a Goodbye.
	PacketTypeBYE PacketType = "BYE"
	// PacketTypeAPP is an Application-defined packet.
	PacketTypeAPP PacketType = "APP"
	// PacketTypePLI is a Picture Loss Indication.
	PacketTypePLI PacketType = "PLI"
	// PacketTypeFIR is a Full Intra Request.
	PacketTypeFIR PacketType = "FIR"
	// PacketTypeNACK is a Generic NACK.
	PacketTypeNACK PacketType = "NACK"
	// PacketTypeTWCC is a Transport-wide Congestion Control feedback.
	PacketTypeTWCC PacketType = "TWCC"
	// PacketTypeREMB is a Receiver Estimated Maximum Bitrate.
	PacketTypeREMB PacketType = "REMB"
)

// Stats contains packet statistics for a track.
type Stats struct {
	// SSRC is the Synchronization Source identifier.
	SSRC uint32

	// PacketsReceived is the total number of packets received.
	PacketsReceived uint64

	// PacketsLost is the total number of packets lost.
	PacketsLost uint64

	// BytesReceived is the total number of bytes received.
	BytesReceived uint64

	// Jitter is the estimated interarrival jitter in milliseconds.
	Jitter float64

	// RTT is the round-trip time in milliseconds.
	RTT float64

	// FractionLost is the fraction of packets lost since last report (0.0-1.0).
	FractionLost float64

	// LastSenderReport is the timestamp of the last sender report.
	LastSenderReport time.Time

	// LastReceiverReport is the timestamp of the last receiver report.
	LastReceiverReport time.Time

	// UpdatedAt is the time when these stats were last updated.
	UpdatedAt time.Time
}

// HandlerConfig contains configuration for the RTCP handler.
type HandlerConfig struct {
	// ReportInterval is the interval for generating RTCP reports.
	// Per tasks.md: TWCC update interval is 100ms.
	ReportInterval time.Duration

	// MaxPacketBufferSize is the maximum size of the packet buffer for NACK.
	MaxPacketBufferSize int

	// TWCCEnabled enables Transport-wide Congestion Control.
	TWCCEnabled bool

	// REMBEnabled enables Receiver Estimated Maximum Bitrate.
	REMBEnabled bool

	// NACKEnabled enables Generic NACK handling.
	NACKEnabled bool

	// PLIFIREnabled enables PLI/FIR handling.
	PLIFIREnabled bool
}

// DefaultHandlerConfig returns the default RTCP handler configuration.
func DefaultHandlerConfig() *HandlerConfig {
	return &HandlerConfig{
		ReportInterval:      100 * time.Millisecond, // Per tasks.md: 100ms for TWCC
		MaxPacketBufferSize: 500,                    // Buffer up to 500 packets for NACK
		TWCCEnabled:         true,
		REMBEnabled:         true,
		NACKEnabled:         true,
		PLIFIREnabled:       true,
	}
}

// PacketCallback is called when an RTCP packet is received.
type PacketCallback func(packets []rtcp.Packet)

// BandwidthEstimateCallback is called when bandwidth estimate is updated.
type BandwidthEstimateCallback func(ssrc uint32, bitrate uint64)

// KeyframeRequestCallback is called when a keyframe is requested.
type KeyframeRequestCallback func(ssrc uint32)

// NACKCallback is called when a NACK packet is received.
type NACKCallback func(ssrc uint32, nacks []uint16)

// Handler processes RTCP packets for a track.
// It aggregates receiver reports, handles feedback, and updates statistics.
type Handler struct {
	mu     sync.RWMutex
	config *HandlerConfig

	// stats contains per-SSRC statistics.
	stats map[uint32]*Stats

	// callbacks
	onPacket            PacketCallback
	onBandwidthEstimate BandwidthEstimateCallback
	onKeyframeRequest   KeyframeRequestCallback
	onNACK              NACKCallback

	// twcc handles Transport-wide Congestion Control.
	twcc *TWCCHandler

	// remb handles REMB bandwidth estimation.
	remb *REMBHandler

	// nack handles NACK processing.
	nack *NACKHandler

	// plifir handles PLI/FIR processing.
	plifir *PLIFIRHandler

	// ctx is the context for cancellation.
	ctx    context.Context
	cancel context.CancelFunc

	// running indicates if the handler is running.
	running bool
}

// NewHandler creates a new RTCP handler.
func NewHandler(cfg *HandlerConfig) *Handler {
	if cfg == nil {
		cfg = DefaultHandlerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	h := &Handler{
		config: cfg,
		stats:  make(map[uint32]*Stats),
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize sub-handlers based on configuration
	if cfg.TWCCEnabled {
		twccCfg := DefaultTWCCConfig()
		twccCfg.UpdateInterval = cfg.ReportInterval
		h.twcc = NewTWCCHandler(twccCfg)
	}

	if cfg.REMBEnabled {
		h.remb = NewREMBHandler(nil) // Use defaults
	}

	if cfg.NACKEnabled {
		h.nack = NewNACKHandler(&NACKConfig{
			MaxBufferSize: cfg.MaxPacketBufferSize,
		})
	}

	if cfg.PLIFIREnabled {
		h.plifir = NewPLIFIRHandler(&PLIFIRConfig{})
	}

	return h
}

// Start starts the RTCP handler.
func (h *Handler) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		return
	}
	h.running = true

	// Recreate context for proper restart support
	h.ctx, h.cancel = context.WithCancel(context.Background())

	// Start sub-handlers
	if h.twcc != nil {
		h.twcc.Start()
	}
	if h.nack != nil {
		h.nack.Start()
	}
}

// Stop stops the RTCP handler.
func (h *Handler) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return
	}
	h.running = false
	h.cancel()

	// Stop sub-handlers
	if h.twcc != nil {
		h.twcc.Stop()
	}
	if h.nack != nil {
		h.nack.Stop()
	}
}

// nackEntry holds NACK data for a single SSRC.
type nackEntry struct {
	ssrc    uint32
	seqNums []uint16
}

// callbackData holds data for callbacks to be invoked outside of locks.
type callbackData struct {
	keyframeSSRCs      []uint32
	nacks              []nackEntry
	bandwidthEstimates []struct {
		ssrc    uint32
		bitrate uint64
	}
}

// HandlePackets processes incoming RTCP packets.
func (h *Handler) HandlePackets(packets []rtcp.Packet) {
	h.mu.Lock()
	var cbData callbackData
	for _, pkt := range packets {
		h.processPacket(pkt, &cbData)
	}
	// Capture callbacks under lock to avoid race
	onPacket := h.onPacket
	onKeyframeRequest := h.onKeyframeRequest
	onNACK := h.onNACK
	onBandwidthEstimate := h.onBandwidthEstimate
	h.mu.Unlock()

	// Notify callbacks outside of lock to prevent deadlocks
	if onPacket != nil {
		onPacket(packets)
	}
	if onKeyframeRequest != nil {
		for _, ssrc := range cbData.keyframeSSRCs {
			onKeyframeRequest(ssrc)
		}
	}
	if onNACK != nil {
		for _, nack := range cbData.nacks {
			onNACK(nack.ssrc, nack.seqNums)
		}
	}
	if onBandwidthEstimate != nil {
		for _, est := range cbData.bandwidthEstimates {
			onBandwidthEstimate(est.ssrc, est.bitrate)
		}
	}
}

// processPacket processes a single RTCP packet.
func (h *Handler) processPacket(pkt rtcp.Packet, cbData *callbackData) {
	switch p := pkt.(type) {
	case *rtcp.SenderReport:
		h.processSenderReport(p)
	case *rtcp.ReceiverReport:
		h.processReceiverReport(p)
	case *rtcp.PictureLossIndication:
		h.processPLI(p, cbData)
	case *rtcp.FullIntraRequest:
		h.processFIR(p, cbData)
	case *rtcp.TransportLayerNack:
		h.processNACK(p, cbData)
	case *rtcp.TransportLayerCC:
		h.processTWCC(p)
	case *rtcp.ReceiverEstimatedMaximumBitrate:
		h.processREMB(p, cbData)
	}
}

// processSenderReport processes a Sender Report.
func (h *Handler) processSenderReport(sr *rtcp.SenderReport) {
	stats := h.getOrCreateStats(sr.SSRC)
	stats.LastSenderReport = time.Now()
	stats.UpdatedAt = time.Now()
}

// processReceiverReport processes a Receiver Report.
func (h *Handler) processReceiverReport(rr *rtcp.ReceiverReport) {
	for _, report := range rr.Reports {
		stats := h.getOrCreateStats(report.SSRC)
		stats.FractionLost = float64(report.FractionLost) / 256.0
		stats.PacketsLost = uint64(report.TotalLost)
		stats.Jitter = float64(report.Jitter)
		stats.LastReceiverReport = time.Now()
		stats.UpdatedAt = time.Now()

		// Calculate RTT if delay is available
		if report.Delay != 0 && report.LastSenderReport != 0 {
			// RTT calculation based on DLSR and LSR
			// RTT = current_time - LSR - DLSR
			// DLSR is in 1/65536 seconds
			dlsr := float64(report.Delay) / 65536.0
			stats.RTT = dlsr * 1000 // Convert to milliseconds
		}
	}
}

// processPLI processes a Picture Loss Indication.
func (h *Handler) processPLI(pli *rtcp.PictureLossIndication, cbData *callbackData) {
	if h.plifir != nil {
		h.plifir.HandlePLI(pli)
	}

	// Collect callback data (actual callback invoked outside lock)
	cbData.keyframeSSRCs = append(cbData.keyframeSSRCs, pli.MediaSSRC)
}

// processFIR processes a Full Intra Request.
func (h *Handler) processFIR(fir *rtcp.FullIntraRequest, cbData *callbackData) {
	if h.plifir != nil {
		h.plifir.HandleFIR(fir)
	}

	// FIR can contain multiple entries - collect for callback
	for _, entry := range fir.FIR {
		cbData.keyframeSSRCs = append(cbData.keyframeSSRCs, entry.SSRC)
	}
}

// processNACK processes a Generic NACK.
func (h *Handler) processNACK(nack *rtcp.TransportLayerNack, cbData *callbackData) {
	if h.nack != nil {
		h.nack.HandleNACK(nack)
	}

	// Convert NACK pairs to sequence numbers
	var seqNums []uint16
	for _, pair := range nack.Nacks {
		seqNums = append(seqNums, pair.PacketList()...)
	}

	if len(seqNums) == 0 {
		return
	}

	// Find existing entry for this SSRC or create new one
	found := false
	for i := range cbData.nacks {
		if cbData.nacks[i].ssrc == nack.MediaSSRC {
			cbData.nacks[i].seqNums = append(cbData.nacks[i].seqNums, seqNums...)
			found = true
			break
		}
	}
	if !found {
		cbData.nacks = append(cbData.nacks, nackEntry{
			ssrc:    nack.MediaSSRC,
			seqNums: seqNums,
		})
	}
}

// processTWCC processes a Transport-wide Congestion Control feedback.
func (h *Handler) processTWCC(twcc *rtcp.TransportLayerCC) {
	if h.twcc != nil {
		h.twcc.HandleFeedback(twcc)
	}
}

// processREMB processes a Receiver Estimated Maximum Bitrate.
func (h *Handler) processREMB(remb *rtcp.ReceiverEstimatedMaximumBitrate, cbData *callbackData) {
	if h.remb != nil {
		h.remb.HandleREMB(remb)
	}

	// Collect bandwidth estimates for callback
	for _, ssrc := range remb.SSRCs {
		cbData.bandwidthEstimates = append(cbData.bandwidthEstimates, struct {
			ssrc    uint32
			bitrate uint64
		}{ssrc, uint64(remb.Bitrate)})
	}
}

// getOrCreateStats gets or creates stats for an SSRC.
func (h *Handler) getOrCreateStats(ssrc uint32) *Stats {
	if stats, ok := h.stats[ssrc]; ok {
		return stats
	}

	stats := &Stats{
		SSRC:      ssrc,
		UpdatedAt: time.Now(),
	}
	h.stats[ssrc] = stats
	return stats
}

// GetStats returns a copy of the stats for an SSRC.
func (h *Handler) GetStats(ssrc uint32) *Stats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if stats, ok := h.stats[ssrc]; ok {
		// Return a copy
		return &Stats{
			SSRC:               stats.SSRC,
			PacketsReceived:    stats.PacketsReceived,
			PacketsLost:        stats.PacketsLost,
			BytesReceived:      stats.BytesReceived,
			Jitter:             stats.Jitter,
			RTT:                stats.RTT,
			FractionLost:       stats.FractionLost,
			LastSenderReport:   stats.LastSenderReport,
			LastReceiverReport: stats.LastReceiverReport,
			UpdatedAt:          stats.UpdatedAt,
		}
	}
	return nil
}

// GetAllStats returns a copy of stats for all SSRCs.
func (h *Handler) GetAllStats() map[uint32]*Stats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[uint32]*Stats, len(h.stats))
	for ssrc, stats := range h.stats {
		result[ssrc] = &Stats{
			SSRC:               stats.SSRC,
			PacketsReceived:    stats.PacketsReceived,
			PacketsLost:        stats.PacketsLost,
			BytesReceived:      stats.BytesReceived,
			Jitter:             stats.Jitter,
			RTT:                stats.RTT,
			FractionLost:       stats.FractionLost,
			LastSenderReport:   stats.LastSenderReport,
			LastReceiverReport: stats.LastReceiverReport,
			UpdatedAt:          stats.UpdatedAt,
		}
	}
	return result
}

// UpdatePacketStats updates packet statistics for an SSRC.
func (h *Handler) UpdatePacketStats(ssrc uint32, packets uint64, bytes uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	stats := h.getOrCreateStats(ssrc)
	stats.PacketsReceived += packets
	stats.BytesReceived += bytes
	stats.UpdatedAt = time.Now()
}

// SetOnPacketCallback sets the callback for received RTCP packets.
func (h *Handler) SetOnPacketCallback(cb PacketCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onPacket = cb
}

// SetOnBandwidthEstimateCallback sets the callback for bandwidth estimate updates.
func (h *Handler) SetOnBandwidthEstimateCallback(cb BandwidthEstimateCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onBandwidthEstimate = cb
}

// SetOnKeyframeRequestCallback sets the callback for keyframe requests.
func (h *Handler) SetOnKeyframeRequestCallback(cb KeyframeRequestCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onKeyframeRequest = cb
}

// SetOnNACKCallback sets the callback for NACK packets.
func (h *Handler) SetOnNACKCallback(cb NACKCallback) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onNACK = cb
}

// GetTWCCHandler returns the TWCC handler.
func (h *Handler) GetTWCCHandler() *TWCCHandler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.twcc
}

// GetREMBHandler returns the REMB handler.
func (h *Handler) GetREMBHandler() *REMBHandler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.remb
}

// GetNACKHandler returns the NACK handler.
func (h *Handler) GetNACKHandler() *NACKHandler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.nack
}

// GetPLIFIRHandler returns the PLI/FIR handler.
func (h *Handler) GetPLIFIRHandler() *PLIFIRHandler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.plifir
}

// GetBandwidthEstimate returns the current bandwidth estimate.
// Per tasks.md: TWCC is preferred, REMB is fallback.
func (h *Handler) GetBandwidthEstimate() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Prefer TWCC estimate if available
	if h.twcc != nil {
		estimate := h.twcc.GetBandwidthEstimate()
		if estimate > 0 {
			return estimate
		}
	}

	// Fall back to REMB
	if h.remb != nil {
		return h.remb.GetBandwidthEstimate()
	}

	return 0
}

// Ensure Handler implements interceptor.RTCPReader interface pattern.
var _ = (*Handler)(nil)
