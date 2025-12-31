package sfu

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// SimulcastDownTrack extends DownTrack with simulcast layer switching
type SimulcastDownTrack struct {
	subscriber       *Subscriber
	simulcastRecv    *SimulcastReceiver
	track            *webrtc.TrackLocalStaticRTP
	sender           *webrtc.RTPSender
	sequencer        *rtpSequencer
	layerSelector    *LayerSelector
	currentReceiver  *Receiver
	codec            string
	closed           atomic.Bool
	mu               sync.RWMutex
}

// NewSimulcastDownTrack creates a new simulcast downtrack
func NewSimulcastDownTrack(subscriber *Subscriber, simulcastRecv *SimulcastReceiver, codec webrtc.RTPCodecParameters) (*SimulcastDownTrack, error) {
	track, err := webrtc.NewTrackLocalStaticRTP(
		codec.RTPCodecCapability,
		simulcastRecv.TrackID(),
		simulcastRecv.StreamID(),
	)
	if err != nil {
		return nil, err
	}

	sender, err := subscriber.pc.AddTrack(track)
	if err != nil {
		return nil, err
	}

	dt := &SimulcastDownTrack{
		subscriber:    subscriber,
		simulcastRecv: simulcastRecv,
		track:         track,
		sender:        sender,
		sequencer:     newRTPSequencer(),
		layerSelector: NewLayerSelector(simulcastRecv.TrackID()),
		codec:         codec.MimeType,
	}

	// Set up layer switch callback
	dt.layerSelector.OnSwitch(func(layer string) {
		dt.onLayerSwitch(layer)
	})

	// Start with the best available layer
	if layer := simulcastRecv.GetBestLayer(); layer != nil {
		dt.currentReceiver = layer.Receiver()
		dt.layerSelector.SetTargetLayer(layer.RID())
		dt.layerSelector.SwitchToTarget()
	}

	go dt.readRTCP()

	// Request keyframe after a short delay to ensure connection is ready
	go func() {
		time.Sleep(100 * time.Millisecond)
		if !dt.closed.Load() {
			currentLayer := dt.layerSelector.GetCurrentLayer()
			dt.requestKeyframe(currentLayer)
		}
		// Retry after 500ms if still no video
		time.Sleep(500 * time.Millisecond)
		if !dt.closed.Load() {
			currentLayer := dt.layerSelector.GetCurrentLayer()
			dt.requestKeyframe(currentLayer)
		}
	}()

	return dt, nil
}

// readRTCP reads RTCP packets from the sender
func (d *SimulcastDownTrack) readRTCP() {
	for {
		if d.closed.Load() {
			return
		}
		if _, _, err := d.sender.ReadRTCP(); err != nil {
			return
		}
	}
}

// SetTargetLayer sets the target layer for this downtrack
func (d *SimulcastDownTrack) SetTargetLayer(layer string) {
	d.layerSelector.SetTargetLayer(layer)
	log.Printf("[SimulcastDownTrack] Target layer set to %s for track %s",
		layer, d.simulcastRecv.TrackID())
}

// GetCurrentLayer returns the current layer
func (d *SimulcastDownTrack) GetCurrentLayer() string {
	return d.layerSelector.GetCurrentLayer()
}

// GetTargetLayer returns the target layer
func (d *SimulcastDownTrack) GetTargetLayer() string {
	return d.layerSelector.GetTargetLayer()
}

// packetCount is used for debug logging
var packetCount uint64

// WriteRTP writes an RTP packet with potential layer switching
func (d *SimulcastDownTrack) WriteRTP(packet *rtp.Packet, fromLayer string) error {
	if d.closed.Load() {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	currentLayer := d.layerSelector.GetCurrentLayer()

	// Debug: log first few packets
	packetCount++
	if packetCount <= 10 || packetCount%1000 == 0 {
		log.Printf("[SimulcastDownTrack] WriteRTP: fromLayer=%s, currentLayer=%s, packet=%d",
			fromLayer, currentLayer, packetCount)
	}

	// Check if we need to switch layers
	if d.layerSelector.NeedsSwitch() && d.layerSelector.CanSwitch() {
		// Check if this packet is a keyframe
		if IsKeyframe(packet.Payload, d.codec) {
			targetLayer := d.layerSelector.GetTargetLayer()
			if fromLayer == targetLayer {
				// Switch on keyframe from target layer
				d.layerSelector.SwitchToTarget()
				currentLayer = targetLayer

				// Update current receiver
				if layer, ok := d.simulcastRecv.GetLayer(currentLayer); ok {
					d.currentReceiver = layer.Receiver()
				}
			}
		}
	}

	// Only forward packets from the current layer
	if fromLayer != currentLayer {
		return nil
	}

	// Rewrite sequence numbers
	ssrc := uint32(d.sender.GetParameters().Encodings[0].SSRC)
	rewritten := d.sequencer.Rewrite(packet, ssrc)

	if packetCount <= 10 || packetCount%1000 == 0 {
		log.Printf("[SimulcastDownTrack] Forwarding packet seq=%d to track", rewritten.SequenceNumber)
	}

	return d.track.WriteRTP(rewritten)
}

// onLayerSwitch handles layer switch events
func (d *SimulcastDownTrack) onLayerSwitch(layer string) {
	log.Printf("[SimulcastDownTrack] Switched to layer %s for track %s",
		layer, d.simulcastRecv.TrackID())

	// Request keyframe from the new layer
	d.requestKeyframe(layer)
}

// requestKeyframe sends a PLI to request a keyframe
func (d *SimulcastDownTrack) requestKeyframe(layer string) {
	// Get the layer's receiver
	layerInfo, ok := d.simulcastRecv.GetLayer(layer)
	if !ok {
		log.Printf("[SimulcastDownTrack] Layer %s not found for keyframe request", layer)
		return
	}

	// Request keyframe via RTCP PLI
	log.Printf("[SimulcastDownTrack] Requesting keyframe for layer %s", layer)
	layerInfo.Receiver().SendPLI()
}

// Track returns the local track
func (d *SimulcastDownTrack) Track() *webrtc.TrackLocalStaticRTP {
	return d.track
}

// Sender returns the RTP sender
func (d *SimulcastDownTrack) Sender() *webrtc.RTPSender {
	return d.sender
}

// SimulcastReceiver returns the simulcast receiver
func (d *SimulcastDownTrack) SimulcastReceiver() *SimulcastReceiver {
	return d.simulcastRecv
}

// Close closes the simulcast downtrack
func (d *SimulcastDownTrack) Close() error {
	if d.closed.Swap(true) {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.simulcastRecv != nil {
		d.simulcastRecv.RemoveDownTrack(d)
	}

	if d.subscriber != nil && d.sender != nil {
		d.subscriber.pc.RemoveTrack(d.sender)
	}

	return nil
}

// SimulcastForwarder manages forwarding from multiple layers to downtracks
type SimulcastForwarder struct {
	simulcastRecv *SimulcastReceiver
	downTracks    map[*SimulcastDownTrack]struct{}
	mu            sync.RWMutex
	closed        bool
	closeCh       chan struct{}
}

// NewSimulcastForwarder creates a new simulcast forwarder
func NewSimulcastForwarder(simulcastRecv *SimulcastReceiver) *SimulcastForwarder {
	return &SimulcastForwarder{
		simulcastRecv: simulcastRecv,
		downTracks:    make(map[*SimulcastDownTrack]struct{}),
		closeCh:       make(chan struct{}),
	}
}

// AddDownTrack adds a downtrack to the forwarder
func (f *SimulcastForwarder) AddDownTrack(dt *SimulcastDownTrack) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return
	}

	f.downTracks[dt] = struct{}{}
}

// RemoveDownTrack removes a downtrack from the forwarder
func (f *SimulcastForwarder) RemoveDownTrack(dt *SimulcastDownTrack) {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.downTracks, dt)
}

// ForwardRTP forwards an RTP packet from a specific layer to all downtracks
func (f *SimulcastForwarder) ForwardRTP(packet *rtp.Packet, fromLayer string) {
	f.mu.RLock()
	downTracks := make([]*SimulcastDownTrack, 0, len(f.downTracks))
	for dt := range f.downTracks {
		downTracks = append(downTracks, dt)
	}
	f.mu.RUnlock()

	for _, dt := range downTracks {
		if err := dt.WriteRTP(packet, fromLayer); err != nil {
			f.RemoveDownTrack(dt)
		}
	}
}

// Close closes the forwarder
func (f *SimulcastForwarder) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return
	}
	f.closed = true
	close(f.closeCh)

	for dt := range f.downTracks {
		dt.Close()
	}
}
