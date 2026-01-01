package sfu

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

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
	}

	// Set up layer switch callback
	dt.selector.OnSwitch(func(layer string) {
		dt.onLayerSwitch(layer)
	})

	go dt.readRTCP()
	go dt.requestInitialKeyframe()

	return dt, nil
}

// readRTCP reads RTCP packets from the sender.
func (d *DownTrack) readRTCP() {
	for {
		if d.closed.Load() {
			return
		}
		if _, _, err := d.sender.ReadRTCP(); err != nil {
			return
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

	if !d.shouldForwardPacket(packet, fromLayer, currentLayer) {
		return nil
	}

	ssrc := uint32(d.sender.GetParameters().Encodings[0].SSRC)
	rewritten := d.sequencer.Rewrite(packet, ssrc)

	return d.track.WriteRTP(rewritten)
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
	if d.isCurrentLayerActive(currentLayer) {
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

// Close closes the downtrack.
func (d *DownTrack) Close() error {
	if d.closed.Swap(true) {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.trackReceiver != nil {
		d.trackReceiver.RemoveDownTrack(d)
	}

	if d.subscriber != nil && d.sender != nil {
	}

	return nil
}
