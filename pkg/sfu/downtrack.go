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

	dt := &DownTrack{
		subscriber:    subscriber,
		trackReceiver: trackReceiver,
		track:         track,
		sender:        sender,
		sequencer:     newRTPSequencer(),
		selector:      NewLayerSelector(trackReceiver.TrackID()),
		codec:         codec.MimeType,
	}

	// Set up layer switch callback
	dt.selector.OnSwitch(func(layer string) {
		dt.onLayerSwitch(layer)
	})

	// Start with the best available layer
	if layer := trackReceiver.GetBestLayer(); layer != nil {
		dt.selector.SetTargetLayer(layer.Name())
		dt.selector.SwitchToTarget()
	}

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
	d.selector.SetTargetLayer(layer)
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

	// Check if we should switch layers
	if d.selector.NeedsSwitch() && d.selector.CanSwitch() {
		if IsKeyframe(packet.Payload, d.codec) {
			targetLayer := d.selector.GetTargetLayer()
			if fromLayer == targetLayer {
				d.selector.SwitchToTarget()
				currentLayer = targetLayer
			}
		}
	}

	// Only forward packets from the current layer
	if fromLayer != currentLayer {
		return nil
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
		return
	}

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
