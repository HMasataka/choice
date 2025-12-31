package sfu

import (
	"log"
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
	log.Printf("[DownTrack] SetTargetLayer: %s -> %s for track %s",
		d.selector.GetCurrentLayer(), layer, d.trackReceiver.TrackID())
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

	currentLayer := d.selector.GetCurrentLayer()
	targetLayer := d.selector.GetTargetLayer()

	// Check if we should switch layers
	if d.selector.NeedsSwitch() && d.selector.CanSwitch() {
		if IsKeyframe(packet.Payload, d.codec) {
			if fromLayer == targetLayer {
				log.Printf("[DownTrack] Switching layer on keyframe: %s -> %s for track %s",
					currentLayer, targetLayer, d.trackReceiver.TrackID())
				d.selector.SwitchToTarget()
				currentLayer = targetLayer
			} else {
				log.Printf("[DownTrack] Keyframe from wrong layer: got %s, want %s for track %s",
					fromLayer, targetLayer, d.trackReceiver.TrackID())
			}
		}
	}

	// Check if current layer exists and is active
	currentLayerExists := false
	if layer, ok := d.trackReceiver.GetLayer(currentLayer); ok && layer.IsActive() {
		currentLayerExists = true
	}

	// If current layer doesn't exist, accept any layer (fallback)
	// This handles the case where high layer isn't available yet
	if !currentLayerExists {
		// Accept this packet and switch to this layer on keyframe
		if IsKeyframe(packet.Payload, d.codec) {
			log.Printf("[DownTrack] Fallback: switching from %s to available layer %s for track %s", currentLayer, fromLayer, d.trackReceiver.TrackID())
			d.selector.ForceSwitch(fromLayer)
			currentLayer = fromLayer
		} else {
			// Not a keyframe, but still forward to avoid black screen
			// The sequence numbers will handle discontinuity
		}
	} else {
		// Only forward packets from the current layer
		if fromLayer != currentLayer {
			return nil
		}
	}

	// Rewrite sequence numbers for seamless playback
	ssrc := uint32(d.sender.GetParameters().Encodings[0].SSRC)
	rewritten := d.sequencer.Rewrite(packet, ssrc)

	return d.track.WriteRTP(rewritten)
}

// onLayerSwitch handles layer switch events.
func (d *DownTrack) onLayerSwitch(layer string) {
	log.Printf("[DownTrack] Switched to layer %s for track %s", layer, d.trackReceiver.TrackID())
	d.requestKeyframe(layer)
}

// requestKeyframe sends a PLI to request a keyframe.
func (d *DownTrack) requestKeyframe(layerName string) {
	layer, ok := d.trackReceiver.GetLayer(layerName)
	if !ok {
		log.Printf("[DownTrack] requestKeyframe: layer %s not found for track %s", layerName, d.trackReceiver.TrackID())
		return
	}

	log.Printf("[DownTrack] Sending PLI for layer %s, track %s", layerName, d.trackReceiver.TrackID())
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
		d.subscriber.pc.RemoveTrack(d.sender)
	}

	return nil
}
