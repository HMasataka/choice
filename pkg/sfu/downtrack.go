package sfu

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// DownTrack sends RTP packets to a subscriber with layer switching support.
type DownTrack struct {
	subscriber    *Subscriber
	trackReceiver *TrackReceiver
	track         *webrtc.TrackLocalStaticRTP
	sender        *webrtc.RTPSender
	sequencer     *rtpSequencer
	selector      *LayerSelector
	codec         string
	closed        atomic.Bool
	mu            sync.RWMutex

	// Stats for bandwidth estimation
	bytesSent        uint64
	packetsSent      uint64
	packetsLost      uint64
	lastFractionLost uint8
	lastStatsTime    time.Time
	statsReportMu    sync.Mutex

	// Advanced congestion control
	twccReceiver *TWCCReceiver
}

// NewDownTrack creates a new downtrack.
func NewDownTrack(subscriber *Subscriber, trackReceiver *TrackReceiver, codec webrtc.RTPCodecParameters) (*DownTrack, error) {
	track, err := webrtc.NewTrackLocalStaticRTP(
		codec.RTPCodecCapability,
		trackReceiver.TrackID(),
		trackReceiver.StreamID(),
	)
	if err != nil {
		return nil, err
	}

	sender, err := subscriber.pc.AddTrack(track)
	if err != nil {
		return nil, err
	}

	// Start with mid layer by default
	// Fall back to best available if mid is not available
	initialLayer := LayerMid
	if _, ok := trackReceiver.GetLayer(LayerMid); !ok {
		if layer := trackReceiver.GetBestLayer(); layer != nil {
			initialLayer = layer.Name()
		}
	}

	dt := &DownTrack{
		subscriber:    subscriber,
		trackReceiver: trackReceiver,
		track:         track,
		sender:        sender,
		sequencer:     newRTPSequencer(),
		selector:      NewLayerSelector(trackReceiver.TrackID(), initialLayer),
		codec:         codec.MimeType,
		lastStatsTime: time.Now(),
		twccReceiver:  NewTWCCReceiver(DefaultTWCCConfig()),
	}

	// Set up layer switch callback
	dt.selector.OnSwitch(func(layer string) {
		dt.onLayerSwitch(layer)
	})

	go dt.readRTCP()
	go dt.requestInitialKeyframe()

	return dt, nil
}

// readRTCP reads RTCP packets from the sender and extracts loss information.
func (d *DownTrack) readRTCP() {
	for {
		if d.closed.Load() {
			return
		}

		packets, _, err := d.sender.ReadRTCP()
		if err != nil {
			return
		}

		for _, pkt := range packets {
			switch p := pkt.(type) {
			case *rtcp.ReceiverReport:
				d.handleReceiverReport(p)
			case *rtcp.TransportLayerCC:
				d.handleTWCCFeedback(p)
			}
		}
	}
}

// handleReceiverReport processes RTCP Receiver Report to extract loss information.
func (d *DownTrack) handleReceiverReport(rr *rtcp.ReceiverReport) {
	for _, report := range rr.Reports {
		d.statsReportMu.Lock()
		d.lastFractionLost = report.FractionLost
		d.packetsLost = uint64(report.TotalLost)
		d.statsReportMu.Unlock()

		if report.FractionLost > 0 {
			lossPercent := float64(report.FractionLost) / 256.0 * 100.0
			slog.Debug("[DownTrack] Receiver report",
				slog.String("trackID", d.trackReceiver.TrackID()),
				slog.Float64("lossPercent", lossPercent),
				slog.Uint64("totalLost", uint64(report.TotalLost)),
			)
		}
	}
}

// handleTWCCFeedback processes Transport Wide Congestion Control feedback.
func (d *DownTrack) handleTWCCFeedback(twcc *rtcp.TransportLayerCC) {
	// Use TWCCReceiver for advanced congestion control
	// This processes the feedback and updates delay-based bandwidth estimate
	d.twccReceiver.ProcessTWCCFeedback(twcc)

	// Also track packet loss for stats
	var received, lost uint64
	for _, chunk := range twcc.PacketChunks {
		switch c := chunk.(type) {
		case *rtcp.RunLengthChunk:
			if c.PacketStatusSymbol == rtcp.TypeTCCPacketReceivedSmallDelta || c.PacketStatusSymbol == rtcp.TypeTCCPacketReceivedLargeDelta {
				received += uint64(c.RunLength)
			} else if c.PacketStatusSymbol == rtcp.TypeTCCPacketNotReceived {
				lost += uint64(c.RunLength)
			}
		case *rtcp.StatusVectorChunk:
			for _, symbol := range c.SymbolList {
				if symbol == rtcp.TypeTCCPacketReceivedSmallDelta || symbol == rtcp.TypeTCCPacketReceivedLargeDelta {
					received++
				} else if symbol == rtcp.TypeTCCPacketNotReceived {
					lost++
				}
			}
		}
	}

	if received+lost > 0 {
		d.statsReportMu.Lock()
		d.packetsLost += lost
		d.statsReportMu.Unlock()

		if lost > 0 {
			slog.Debug("[DownTrack] TWCC feedback",
				slog.String("trackID", d.trackReceiver.TrackID()),
				slog.Uint64("received", received),
				slog.Uint64("lost", lost),
			)
		}
	}
}

// requestInitialKeyframe requests keyframes with retry.
func (d *DownTrack) requestInitialKeyframe() {
	time.Sleep(100 * time.Millisecond)
	if !d.closed.Load() {
		d.requestKeyframe(d.selector.GetCurrentLayer())
	}

	time.Sleep(500 * time.Millisecond)
	if !d.closed.Load() {
		d.requestKeyframe(d.selector.GetCurrentLayer())
	}
}

// SetTargetLayer sets the target layer.
func (d *DownTrack) SetTargetLayer(layer string) {
	slog.Info("[DownTrack] SetTargetLayer",
		slog.String("from", d.selector.GetCurrentLayer()),
		slog.String("to", layer),
		slog.String("trackID", d.trackReceiver.TrackID()),
	)
	d.selector.SetTargetLayer(layer)

	// Request keyframe from the target layer to speed up switching
	d.requestKeyframe(layer)
}

// GetCurrentLayer returns the current layer.
func (d *DownTrack) GetCurrentLayer() string {
	return d.selector.GetCurrentLayer()
}

// GetTargetLayer returns the target layer.
func (d *DownTrack) GetTargetLayer() string {
	return d.selector.GetTargetLayer()
}

// WriteRTP writes an RTP packet with layer switching.
func (d *DownTrack) WriteRTP(packet *rtp.Packet, fromLayer string) error {
	if d.closed.Load() {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	currentLayer := d.tryLayerSwitch(packet, fromLayer)

	// Retry keyframe request if needed
	d.retryKeyframeRequestIfNeeded()

	if !d.shouldForwardPacket(packet, fromLayer, currentLayer) {
		return nil
	}

	ssrc := uint32(d.sender.GetParameters().Encodings[0].SSRC)
	rewritten := d.sequencer.Rewrite(packet, ssrc)

	if err := d.track.WriteRTP(rewritten); err != nil {
		return err
	}

	// Track bytes and packets sent for bandwidth estimation
	d.statsReportMu.Lock()
	d.bytesSent += uint64(len(packet.Payload) + 12) // payload + RTP header
	d.packetsSent++
	d.statsReportMu.Unlock()

	return nil
}

// retryKeyframeRequestIfNeeded sends a keyframe request if needed during layer switch.
func (d *DownTrack) retryKeyframeRequestIfNeeded() {
	if !d.selector.NeedsKeyframeRequest() {
		return
	}

	targetLayer := d.selector.GetTargetLayer()
	d.selector.MarkKeyframeRequested()

	// Request keyframe asynchronously to avoid blocking
	go func() {
		layer, ok := d.trackReceiver.GetLayer(targetLayer)
		if !ok {
			return
		}
		slog.Debug("[DownTrack] Retrying keyframe request",
			slog.String("layer", targetLayer),
			slog.String("trackID", d.trackReceiver.TrackID()),
		)
		layer.Receiver().SendPLI()
	}()
}

// tryLayerSwitch attempts to switch layers if conditions are met.
// Returns the current layer after any switch attempt.
func (d *DownTrack) tryLayerSwitch(packet *rtp.Packet, fromLayer string) string {
	currentLayer := d.selector.GetCurrentLayer()
	targetLayer := d.selector.GetTargetLayer()

	if !d.selector.NeedsSwitch() || !d.selector.CanSwitch() {
		return currentLayer
	}

	if !IsKeyframe(packet.Payload, d.codec) {
		return currentLayer
	}

	if fromLayer == targetLayer {
		slog.Info("[DownTrack] Switching layer on keyframe",
			slog.String("from", currentLayer),
			slog.String("to", targetLayer),
			slog.String("trackID", d.trackReceiver.TrackID()),
		)
		d.selector.SwitchToTarget()

		return targetLayer
	}

	slog.Warn("[DownTrack] Ignoring keyframe from non-target layer",
		slog.String("from", fromLayer),
		slog.String("want", targetLayer),
		slog.String("trackID", d.trackReceiver.TrackID()),
	)

	return currentLayer
}

// shouldForwardPacket determines if the packet should be forwarded.
// Also handles fallback layer switching when current layer is unavailable.
func (d *DownTrack) shouldForwardPacket(packet *rtp.Packet, fromLayer, currentLayer string) bool {
	// If current layer is active, forward packets from current layer
	if d.isCurrentLayerActive(currentLayer) {
		// During layer switch, also accept packets from current layer
		// to avoid black screen while waiting for keyframe from target layer
		if d.selector.NeedsSwitch() {
			// Accept both current and target layer packets during transition
			targetLayer := d.selector.GetTargetLayer()
			return fromLayer == currentLayer || fromLayer == targetLayer
		}
		return fromLayer == currentLayer
	}

	// Current layer is not active, handle fallback
	d.tryFallbackSwitch(packet, fromLayer, currentLayer)

	// Forward packet to avoid black screen even during fallback
	return true
}

// isCurrentLayerActive checks if the current layer exists and is active.
func (d *DownTrack) isCurrentLayerActive(currentLayer string) bool {
	layer, ok := d.trackReceiver.GetLayer(currentLayer)

	return ok && layer.IsActive()
}

// tryFallbackSwitch attempts a fallback layer switch on keyframe.
func (d *DownTrack) tryFallbackSwitch(packet *rtp.Packet, fromLayer, currentLayer string) {
	if !IsKeyframe(packet.Payload, d.codec) {
		return
	}

	slog.Info("[DownTrack] Fallback layer switch on keyframe",
		slog.String("from", currentLayer),
		slog.String("to", fromLayer),
		slog.String("trackID", d.trackReceiver.TrackID()),
	)
	d.selector.ForceSwitch(fromLayer)
}

// onLayerSwitch handles layer switch events.
func (d *DownTrack) onLayerSwitch(layer string) {
	slog.Info("[DownTrack] onLayerSwitch", slog.String("to", layer), slog.String("trackID", d.trackReceiver.TrackID()))
	d.requestKeyframe(layer)
}

// requestKeyframe sends a PLI to request a keyframe.
func (d *DownTrack) requestKeyframe(layerName string) {
	layer, ok := d.trackReceiver.GetLayer(layerName)
	if !ok {
		slog.Warn("[DownTrack] requestKeyframe: layer not found", slog.String("layer", layerName), slog.String("trackID", d.trackReceiver.TrackID()))
		return
	}

	slog.Info("[DownTrack] requestKeyframe", slog.String("layer", layerName), slog.String("trackID", d.trackReceiver.TrackID()))
	layer.Receiver().SendPLI()
}

// TrackReceiver returns the track receiver.
func (d *DownTrack) TrackReceiver() *TrackReceiver {
	return d.trackReceiver
}

// TrackID returns the track ID.
func (d *DownTrack) TrackID() string {
	return d.trackReceiver.TrackID()
}

// GetStats returns stats since last call and resets counters.
// Returns bytes sent, duration, loss rate, and delay-based bitrate estimate.
func (d *DownTrack) GetStats() (bytesSent uint64, duration time.Duration, lossRate float64, delayEstimate uint64) {
	d.statsReportMu.Lock()
	defer d.statsReportMu.Unlock()

	now := time.Now()
	duration = now.Sub(d.lastStatsTime)
	bytesSent = d.bytesSent

	// Calculate loss rate from RTCP Receiver Report (FractionLost is 0-255)
	// FractionLost represents the fraction of packets lost since last report
	lossRate = float64(d.lastFractionLost) / 256.0

	// Get delay-based estimate from TWCCReceiver
	delayEstimate = d.twccReceiver.GetDelayEstimate()

	// Reset counters
	d.bytesSent = 0
	d.packetsSent = 0
	d.lastFractionLost = 0
	d.lastStatsTime = now

	return bytesSent, duration, lossRate, delayEstimate
}

// Close closes the downtrack.
func (d *DownTrack) Close() error {
	if d.closed.Swap(true) {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Close TWCCReceiver
	if d.twccReceiver != nil {
		d.twccReceiver.Close()
	}

	return nil
}
