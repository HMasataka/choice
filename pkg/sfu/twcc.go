package sfu

import (
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/rtcp"
)

// TWCCConfig contains TWCC configuration
type TWCCConfig struct {
	// Initial bitrate estimate
	InitialBitrate uint64
	// Minimum bitrate
	MinBitrate uint64
	// Maximum bitrate
	MaxBitrate uint64
	// Interval for sending TWCC feedback
	FeedbackInterval time.Duration
}

// DefaultTWCCConfig returns the default TWCC configuration
func DefaultTWCCConfig() TWCCConfig {
	return TWCCConfig{
		InitialBitrate:   1_000_000, // 1 Mbps
		MinBitrate:       100_000,   // 100 Kbps
		MaxBitrate:       5_000_000, // 5 Mbps
		FeedbackInterval: 100 * time.Millisecond,
	}
}

// PacketInfo contains information about a received packet for TWCC
type PacketInfo struct {
	SequenceNumber uint16
	ArrivalTime    time.Time
	Size           int
}

// DelayGradientState represents the state of the delay gradient detector
type DelayGradientState int

const (
	DelayStateNormal DelayGradientState = iota
	DelayStateOverusing
	DelayStateUnderusing
)

// TWCCReceiver receives TWCC feedback and estimates bandwidth
type TWCCReceiver struct {
	config           TWCCConfig
	packets          map[uint16]*PacketInfo
	estimatedBitrate uint64
	lossRate         float64
	rtt              time.Duration
	onBitrateChange  func(bitrate uint64)
	mu               sync.RWMutex
	closed           bool
	closeCh          chan struct{}

	// Delay-based estimation (GCC algorithm)
	delayDetector     *DelayBasedDetector
	lastDelayEstimate uint64
}

// NewTWCCReceiver creates a new TWCC receiver
func NewTWCCReceiver(config TWCCConfig) *TWCCReceiver {
	return &TWCCReceiver{
		config:            config,
		packets:           make(map[uint16]*PacketInfo),
		estimatedBitrate:  config.InitialBitrate,
		closeCh:           make(chan struct{}),
		delayDetector:     NewDelayBasedDetector(config),
		lastDelayEstimate: config.InitialBitrate,
	}
}

// OnBitrateChange sets the callback for bitrate changes
func (t *TWCCReceiver) OnBitrateChange(cb func(bitrate uint64)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onBitrateChange = cb
}

// RecordPacket records a received packet
func (t *TWCCReceiver) RecordPacket(seqNum uint16, size int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	t.packets[seqNum] = &PacketInfo{
		SequenceNumber: seqNum,
		ArrivalTime:    time.Now(),
		Size:           size,
	}

	// Clean up old packets (keep last 1000)
	if len(t.packets) > 1000 {
		t.cleanupOldPackets()
	}
}

// cleanupOldPackets removes old packet records
func (t *TWCCReceiver) cleanupOldPackets() {
	threshold := time.Now().Add(-5 * time.Second)
	for seq, pkt := range t.packets {
		if pkt.ArrivalTime.Before(threshold) {
			delete(t.packets, seq)
		}
	}
}

// GetEstimatedBitrate returns the current estimated bitrate
func (t *TWCCReceiver) GetEstimatedBitrate() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.estimatedBitrate
}

// GetLossRate returns the current packet loss rate
func (t *TWCCReceiver) GetLossRate() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lossRate
}

// GetRTT returns the current RTT estimate
func (t *TWCCReceiver) GetRTT() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.rtt
}

// ProcessTWCCFeedback processes TWCC feedback and updates bandwidth estimate
func (t *TWCCReceiver) ProcessTWCCFeedback(twcc *rtcp.TransportLayerCC) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	// Extract arrival times and calculate inter-arrival deltas
	arrivalDeltas := t.extractArrivalDeltas(twcc)
	if len(arrivalDeltas) == 0 {
		return
	}

	// Calculate packet loss from feedback
	received, lost := t.countPacketStatus(twcc)
	if received+lost > 0 {
		t.lossRate = float64(lost) / float64(received+lost)
	}

	// Update delay-based detector with arrival deltas
	state := t.delayDetector.Update(arrivalDeltas)

	// Adjust bitrate based on delay state
	oldBitrate := t.estimatedBitrate
	switch state {
	case DelayStateOverusing:
		// Reduce bitrate when detecting overuse
		t.estimatedBitrate = uint64(float64(t.estimatedBitrate) * 0.85)
		slog.Debug("[TWCCReceiver] Overuse detected, reducing bitrate",
			slog.Uint64("from", oldBitrate),
			slog.Uint64("to", t.estimatedBitrate))
	case DelayStateUnderusing:
		// Can increase bitrate
		t.estimatedBitrate = uint64(float64(t.estimatedBitrate) * 1.05)
		slog.Debug("[TWCCReceiver] Underuse detected, increasing bitrate",
			slog.Uint64("from", oldBitrate),
			slog.Uint64("to", t.estimatedBitrate))
	}

	// Apply loss-based adjustment on top
	if t.lossRate > 0.1 {
		t.estimatedBitrate = uint64(float64(t.estimatedBitrate) * 0.5)
	} else if t.lossRate > 0.02 {
		t.estimatedBitrate = uint64(float64(t.estimatedBitrate) * 0.85)
	}

	// Clamp to configured bounds
	t.estimatedBitrate = clampBitrate(t.estimatedBitrate, t.config.MinBitrate, t.config.MaxBitrate)
	t.lastDelayEstimate = t.estimatedBitrate

	// Notify callback if bitrate changed significantly
	if t.onBitrateChange != nil && math.Abs(float64(t.estimatedBitrate)-float64(oldBitrate)) > float64(oldBitrate)*0.05 {
		go t.onBitrateChange(t.estimatedBitrate)
	}
}

// extractArrivalDeltas extracts inter-arrival time deltas from TWCC feedback
func (t *TWCCReceiver) extractArrivalDeltas(twcc *rtcp.TransportLayerCC) []time.Duration {
	deltas := make([]time.Duration, 0, len(twcc.RecvDeltas))

	var prevDelta time.Duration
	for i, recvDelta := range twcc.RecvDeltas {
		// RecvDelta.Delta is in 250us units
		arrivalDelta := time.Duration(recvDelta.Delta) * 250 * time.Microsecond

		if i > 0 {
			// Calculate inter-arrival delta (change in arrival time between packets)
			interArrivalDelta := arrivalDelta - prevDelta
			deltas = append(deltas, interArrivalDelta)
		}
		prevDelta = arrivalDelta
	}

	return deltas
}

// countPacketStatus counts received and lost packets from TWCC feedback
func (t *TWCCReceiver) countPacketStatus(twcc *rtcp.TransportLayerCC) (received, lost uint64) {
	for _, chunk := range twcc.PacketChunks {
		switch c := chunk.(type) {
		case *rtcp.RunLengthChunk:
			if c.PacketStatusSymbol == rtcp.TypeTCCPacketReceivedSmallDelta ||
				c.PacketStatusSymbol == rtcp.TypeTCCPacketReceivedLargeDelta {
				received += uint64(c.RunLength)
			} else if c.PacketStatusSymbol == rtcp.TypeTCCPacketNotReceived {
				lost += uint64(c.RunLength)
			}
		case *rtcp.StatusVectorChunk:
			for _, symbol := range c.SymbolList {
				if symbol == rtcp.TypeTCCPacketReceivedSmallDelta ||
					symbol == rtcp.TypeTCCPacketReceivedLargeDelta {
					received++
				} else if symbol == rtcp.TypeTCCPacketNotReceived {
					lost++
				}
			}
		}
	}
	return received, lost
}

// GetDelayEstimate returns the delay-based bandwidth estimate
func (t *TWCCReceiver) GetDelayEstimate() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastDelayEstimate
}

// Close closes the TWCC receiver
func (t *TWCCReceiver) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}
	t.closed = true
	close(t.closeCh)
}

// TWCCSender sends TWCC feedback
type TWCCSender struct {
	config        TWCCConfig
	referenceTime time.Time
	packets       []*PacketInfo
	feedbackCount uint8
	onFeedback    func([]rtcp.Packet)
	mu            sync.Mutex
	closed        bool
	closeCh       chan struct{}
}

// NewTWCCSender creates a new TWCC sender
func NewTWCCSender(config TWCCConfig) *TWCCSender {
	return &TWCCSender{
		config:        config,
		referenceTime: time.Now(),
		packets:       make([]*PacketInfo, 0, 256),
		closeCh:       make(chan struct{}),
	}
}

// OnFeedback sets the callback for sending feedback
func (t *TWCCSender) OnFeedback(cb func([]rtcp.Packet)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onFeedback = cb
}

// RecordPacket records a sent packet
func (t *TWCCSender) RecordPacket(seqNum uint16, size int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	t.packets = append(t.packets, &PacketInfo{
		SequenceNumber: seqNum,
		ArrivalTime:    time.Now(),
		Size:           size,
	})
}

// Start starts the feedback loop
func (t *TWCCSender) Start() {
	go t.feedbackLoop()
}

// feedbackLoop periodically sends TWCC feedback
func (t *TWCCSender) feedbackLoop() {
	ticker := time.NewTicker(t.config.FeedbackInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.closeCh:
			return
		case <-ticker.C:
			t.sendFeedback()
		}
	}
}

// sendFeedback generates and sends TWCC feedback
func (t *TWCCSender) sendFeedback() {
	t.mu.Lock()
	if t.closed || len(t.packets) == 0 {
		t.mu.Unlock()
		return
	}

	packets := t.packets
	t.packets = make([]*PacketInfo, 0, 256)
	callback := t.onFeedback
	t.feedbackCount++
	t.mu.Unlock()

	if callback == nil {
		return
	}

	// Build TWCC feedback packet
	feedback := t.buildFeedback(packets)
	if feedback != nil {
		callback([]rtcp.Packet{feedback})
	}
}

// buildFeedback creates a TWCC feedback packet
func (t *TWCCSender) buildFeedback(packets []*PacketInfo) rtcp.Packet {
	if len(packets) == 0 {
		return nil
	}

	// Find base sequence number
	baseSeq := packets[0].SequenceNumber
	for _, p := range packets {
		if p.SequenceNumber < baseSeq {
			baseSeq = p.SequenceNumber
		}
	}

	// Build packet status chunks
	recvDeltas := make([]*rtcp.RecvDelta, 0, len(packets))
	for _, p := range packets {
		delta := p.ArrivalTime.Sub(t.referenceTime)
		recvDeltas = append(recvDeltas, &rtcp.RecvDelta{
			Type:  rtcp.TypeTCCPacketReceivedSmallDelta,
			Delta: delta.Microseconds() * 250, // 250us units
		})
	}

	return &rtcp.TransportLayerCC{
		Header: rtcp.Header{
			Count:  rtcp.FormatTCC,
			Type:   rtcp.TypeTransportSpecificFeedback,
			Length: 0, // Will be calculated
		},
		MediaSSRC:          0, // Set by caller
		BaseSequenceNumber: baseSeq,
		PacketStatusCount:  uint16(len(packets)),
		ReferenceTime:      uint32(t.referenceTime.UnixNano() / 64000), // 64ms units
		FbPktCount:         t.feedbackCount,
		RecvDeltas:         recvDeltas,
	}
}

// Close closes the TWCC sender
func (t *TWCCSender) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}
	t.closed = true
	close(t.closeCh)
}

// BandwidthEstimator estimates bandwidth using various signals
type BandwidthEstimator struct {
	config             TWCCConfig
	estimatedBitrate   uint64
	targetBitrate      uint64
	lossBasedEstimate  uint64
	delayBasedEstimate uint64
	lastUpdate         time.Time
	history            []bitratePoint
	onEstimate         func(bitrate uint64)
	mu                 sync.RWMutex
}

type bitratePoint struct {
	timestamp time.Time
	bitrate   uint64
	lossRate  float64
}

// NewBandwidthEstimator creates a new bandwidth estimator
func NewBandwidthEstimator(config TWCCConfig) *BandwidthEstimator {
	return &BandwidthEstimator{
		config:           config,
		estimatedBitrate: config.InitialBitrate,
		targetBitrate:    config.InitialBitrate,
		history:          make([]bitratePoint, 0, 100),
	}
}

// OnEstimate sets the callback for bandwidth estimates
func (b *BandwidthEstimator) OnEstimate(cb func(bitrate uint64)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onEstimate = cb
}

// Update updates the bandwidth estimate with new data
func (b *BandwidthEstimator) Update(receivedBytes uint64, duration time.Duration, lossRate float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Calculate instantaneous bitrate
	if duration > 0 {
		instantBitrate := uint64(float64(receivedBytes*8) / duration.Seconds())

		// Apply loss-based adjustment
		b.lossBasedEstimate = b.calculateLossBasedEstimate(instantBitrate, lossRate)

		// Combine estimates using weighted average
		b.estimatedBitrate = b.combineEstimates()

		// Clamp to configured bounds
		b.estimatedBitrate = clampBitrate(b.estimatedBitrate, b.config.MinBitrate, b.config.MaxBitrate)

		// Record history
		b.history = append(b.history, bitratePoint{
			timestamp: now,
			bitrate:   b.estimatedBitrate,
			lossRate:  lossRate,
		})

		// Keep only recent history
		if len(b.history) > 100 {
			b.history = b.history[1:]
		}

		b.lastUpdate = now
	}

	// Notify callback
	if b.onEstimate != nil {
		go b.onEstimate(b.estimatedBitrate)
	}
}

// calculateLossBasedEstimate adjusts bitrate based on packet loss
func (b *BandwidthEstimator) calculateLossBasedEstimate(currentBitrate uint64, lossRate float64) uint64 {
	if lossRate > 0.1 { // >10% loss - aggressive decrease
		return uint64(float64(currentBitrate) * 0.5)
	} else if lossRate > 0.02 { // >2% loss - moderate decrease
		return uint64(float64(currentBitrate) * 0.85)
	} else if lossRate < 0.01 { // <1% loss - can increase
		return uint64(float64(currentBitrate) * 1.05)
	}
	return currentBitrate
}

// combineEstimates combines delay-based and loss-based estimates
func (b *BandwidthEstimator) combineEstimates() uint64 {
	// Use the minimum of the two estimates for safety
	if b.delayBasedEstimate > 0 && b.lossBasedEstimate > 0 {
		if b.delayBasedEstimate < b.lossBasedEstimate {
			return b.delayBasedEstimate
		}
		return b.lossBasedEstimate
	}

	if b.lossBasedEstimate > 0 {
		return b.lossBasedEstimate
	}

	return b.estimatedBitrate
}

// GetEstimate returns the current bandwidth estimate
func (b *BandwidthEstimator) GetEstimate() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.estimatedBitrate
}

// SetDelayBasedEstimate sets the delay-based estimate (from TWCC)
func (b *BandwidthEstimator) SetDelayBasedEstimate(bitrate uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.delayBasedEstimate = bitrate
}

// clampBitrate clamps a bitrate to the given bounds
func clampBitrate(bitrate, min, max uint64) uint64 {
	if bitrate < min {
		return min
	}
	if bitrate > max {
		return max
	}
	return bitrate
}

// CongestionController implements congestion control using pion's cc package
type CongestionController struct {
	estimator cc.BandwidthEstimator
	config    TWCCConfig
	mu        sync.RWMutex
}

// NewCongestionController creates a new congestion controller
func NewCongestionController(config TWCCConfig) *CongestionController {
	return &CongestionController{
		config: config,
	}
}

// SetEstimator sets the bandwidth estimator from pion interceptor
func (c *CongestionController) SetEstimator(estimator cc.BandwidthEstimator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.estimator = estimator
}

// GetTargetBitrate returns the target bitrate from the congestion controller
func (c *CongestionController) GetTargetBitrate() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.estimator == nil {
		return int(c.config.InitialBitrate)
	}

	return c.estimator.GetTargetBitrate()
}

// DelayBasedDetector implements the delay-based congestion detection (GCC algorithm)
// It uses the Trendline filter to detect network congestion based on inter-arrival delays
type DelayBasedDetector struct {
	config TWCCConfig

	// Trendline filter state (exponential moving average)
	trendlineSlope    float64
	smoothedDelay     float64
	varNoise          float64 // Noise variance estimate
	threshold         float64 // Adaptive threshold
	lastState         DelayGradientState
	overuseCounter    int
	underuseCounter   int
	stateHoldDuration int // Number of samples to hold before state change

	// Kalman filter parameters (for noise estimation)
	processNoise     float64
	measurementNoise float64

	mu sync.Mutex
}

// NewDelayBasedDetector creates a new delay-based detector
func NewDelayBasedDetector(config TWCCConfig) *DelayBasedDetector {
	return &DelayBasedDetector{
		config:            config,
		threshold:         12.5, // Initial threshold in ms (from GCC paper)
		varNoise:          50.0, // Initial noise variance
		processNoise:      1e-3,
		measurementNoise:  0.1,
		stateHoldDuration: 3,
	}
}

// Update processes new arrival deltas and returns the current congestion state
func (d *DelayBasedDetector) Update(deltas []time.Duration) DelayGradientState {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(deltas) == 0 {
		return d.lastState
	}

	// Calculate average delay gradient
	var totalGradient float64
	for _, delta := range deltas {
		// Convert to milliseconds for calculation
		gradientMs := float64(delta.Microseconds()) / 1000.0
		totalGradient += gradientMs
	}
	avgGradient := totalGradient / float64(len(deltas))

	// Update smoothed delay using exponential moving average
	alpha := 0.9 // Smoothing factor
	d.smoothedDelay = alpha*d.smoothedDelay + (1-alpha)*avgGradient

	// Update trendline slope (simplified linear regression)
	d.trendlineSlope = 0.9*d.trendlineSlope + 0.1*avgGradient

	// Update noise variance estimate (Kalman-like update)
	residual := math.Abs(avgGradient - d.smoothedDelay)
	d.varNoise = 0.97*d.varNoise + 0.03*residual*residual

	// Update adaptive threshold based on noise variance
	// Higher noise -> higher threshold to avoid false positives
	d.threshold = math.Max(6.0, math.Min(600.0, 12.5+math.Sqrt(d.varNoise)*2))

	// Detect state based on trendline slope vs threshold
	newState := d.detectState()

	// Apply hysteresis to avoid rapid state changes
	d.lastState = d.applyHysteresis(newState)

	return d.lastState
}

// detectState determines the current state based on trendline slope
func (d *DelayBasedDetector) detectState() DelayGradientState {
	if d.trendlineSlope > d.threshold {
		return DelayStateOverusing
	} else if d.trendlineSlope < -d.threshold {
		return DelayStateUnderusing
	}
	return DelayStateNormal
}

// applyHysteresis applies hysteresis to state transitions
func (d *DelayBasedDetector) applyHysteresis(newState DelayGradientState) DelayGradientState {
	switch newState {
	case DelayStateOverusing:
		d.overuseCounter++
		d.underuseCounter = 0
		if d.overuseCounter >= d.stateHoldDuration {
			return DelayStateOverusing
		}
	case DelayStateUnderusing:
		d.underuseCounter++
		d.overuseCounter = 0
		if d.underuseCounter >= d.stateHoldDuration {
			return DelayStateUnderusing
		}
	default:
		d.overuseCounter = 0
		d.underuseCounter = 0
	}

	// Return previous state if not enough samples to confirm change
	if d.lastState == DelayStateNormal {
		return DelayStateNormal
	}
	return d.lastState
}

// GetThreshold returns the current adaptive threshold
func (d *DelayBasedDetector) GetThreshold() float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.threshold
}

// GetTrendlineSlope returns the current trendline slope
func (d *DelayBasedDetector) GetTrendlineSlope() float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.trendlineSlope
}
