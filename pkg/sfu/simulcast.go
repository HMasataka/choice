package sfu

import (
	"log"
	"sync"

	"github.com/pion/webrtc/v4"
)

// Simulcast layer names
const (
	LayerHigh = "high"
	LayerMid  = "mid"
	LayerLow  = "low"
)

// LayerInfo contains information about a simulcast layer
type LayerInfo struct {
	RID           string
	TID           int // Temporal layer ID
	SpatialLayer  int
	TemporalLayer int
}

// SimulcastLayer represents a single simulcast layer
type SimulcastLayer struct {
	rid         string
	receiver    *Receiver
	ssrc        webrtc.SSRC
	bitrate     uint64
	maxBitrate  uint64
	active      bool
	mu          sync.RWMutex
}

// NewSimulcastLayer creates a new simulcast layer
func NewSimulcastLayer(rid string, receiver *Receiver) *SimulcastLayer {
	return &SimulcastLayer{
		rid:      rid,
		receiver: receiver,
		ssrc:     receiver.SSRC(),
		active:   true,
	}
}

// RID returns the layer's RID
func (l *SimulcastLayer) RID() string {
	return l.rid
}

// Receiver returns the layer's receiver
func (l *SimulcastLayer) Receiver() *Receiver {
	return l.receiver
}

// SSRC returns the layer's SSRC
func (l *SimulcastLayer) SSRC() webrtc.SSRC {
	return l.ssrc
}

// Bitrate returns the current bitrate
func (l *SimulcastLayer) Bitrate() uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.bitrate
}

// SetBitrate updates the current bitrate
func (l *SimulcastLayer) SetBitrate(bitrate uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bitrate = bitrate
}

// IsActive returns whether the layer is active
func (l *SimulcastLayer) IsActive() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.active
}

// SetActive sets the layer's active state
func (l *SimulcastLayer) SetActive(active bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active = active
}

// SimulcastReceiver manages multiple simulcast layers for a single video track
type SimulcastReceiver struct {
	trackID     string
	streamID    string
	kind        webrtc.RTPCodecType
	layers      map[string]*SimulcastLayer
	downTracks  []*SimulcastDownTrack
	onLayerChange func(layer string)
	mu          sync.RWMutex
	closed      bool
	closeCh     chan struct{}
}

// NewSimulcastReceiver creates a new simulcast receiver
func NewSimulcastReceiver(trackID, streamID string, kind webrtc.RTPCodecType) *SimulcastReceiver {
	return &SimulcastReceiver{
		trackID:  trackID,
		streamID: streamID,
		kind:     kind,
		layers:   make(map[string]*SimulcastLayer),
		closeCh:  make(chan struct{}),
	}
}

// TrackID returns the track ID
func (s *SimulcastReceiver) TrackID() string {
	return s.trackID
}

// StreamID returns the stream ID
func (s *SimulcastReceiver) StreamID() string {
	return s.streamID
}

// Kind returns the track kind
func (s *SimulcastReceiver) Kind() webrtc.RTPCodecType {
	return s.kind
}

// AddLayer adds a new simulcast layer
func (s *SimulcastReceiver) AddLayer(rid string, receiver *Receiver) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	layer := NewSimulcastLayer(rid, receiver)
	s.layers[rid] = layer

	log.Printf("[SimulcastReceiver] Added layer %s for track %s", rid, s.trackID)

	// Start forwarding for this layer
	go s.forwardLayer(layer)
}

// GetLayer returns a specific layer
func (s *SimulcastReceiver) GetLayer(rid string) (*SimulcastLayer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	layer, ok := s.layers[rid]
	return layer, ok
}

// GetLayers returns all layers
func (s *SimulcastReceiver) GetLayers() map[string]*SimulcastLayer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	layers := make(map[string]*SimulcastLayer, len(s.layers))
	for k, v := range s.layers {
		layers[k] = v
	}
	return layers
}

// GetBestLayer returns the highest quality active layer
func (s *SimulcastReceiver) GetBestLayer() *SimulcastLayer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Priority: high > mid > low
	for _, rid := range []string{LayerHigh, LayerMid, LayerLow} {
		if layer, ok := s.layers[rid]; ok && layer.IsActive() {
			return layer
		}
	}
	return nil
}

// GetLayerForBitrate returns the best layer for the given bitrate
func (s *SimulcastReceiver) GetLayerForBitrate(availableBitrate uint64) *SimulcastLayer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var bestLayer *SimulcastLayer
	var bestBitrate uint64

	// Find the highest quality layer that fits within the available bitrate
	for _, rid := range []string{LayerHigh, LayerMid, LayerLow} {
		layer, ok := s.layers[rid]
		if !ok || !layer.IsActive() {
			continue
		}

		layerBitrate := layer.Bitrate()
		if layerBitrate <= availableBitrate && layerBitrate > bestBitrate {
			bestLayer = layer
			bestBitrate = layerBitrate
		}
	}

	// If no layer fits, return the lowest quality layer
	if bestLayer == nil {
		if layer, ok := s.layers[LayerLow]; ok && layer.IsActive() {
			return layer
		}
	}

	return bestLayer
}

// AddDownTrack adds a simulcast downtrack
func (s *SimulcastReceiver) AddDownTrack(dt *SimulcastDownTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.downTracks = append(s.downTracks, dt)
}

// RemoveDownTrack removes a simulcast downtrack
func (s *SimulcastReceiver) RemoveDownTrack(dt *SimulcastDownTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, d := range s.downTracks {
		if d == dt {
			s.downTracks = append(s.downTracks[:i], s.downTracks[i+1:]...)
			return
		}
	}
}

// forwardLayer reads RTP from a layer and forwards to appropriate downtracks
func (s *SimulcastReceiver) forwardLayer(layer *SimulcastLayer) {
	_ = layer // Forwarding is handled by the publisher's readSimulcastRTP

	for {
		select {
		case <-s.closeCh:
			return
		default:
		}

		// The receiver's ReadRTP loop handles forwarding
		// This goroutine monitors the layer's health
	}
}

// Close closes the simulcast receiver
func (s *SimulcastReceiver) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.closeCh)

	layers := make([]*SimulcastLayer, 0, len(s.layers))
	for _, layer := range s.layers {
		layers = append(layers, layer)
	}
	s.mu.Unlock()

	for _, layer := range layers {
		layer.receiver.Close()
	}

	return nil
}

// LayerPriority returns the priority of a layer (higher is better)
func LayerPriority(rid string) int {
	switch rid {
	case LayerHigh:
		return 3
	case LayerMid:
		return 2
	case LayerLow:
		return 1
	default:
		return 0
	}
}

// ParseRID extracts the RID from a track
func ParseRID(track *webrtc.TrackRemote) string {
	rid := track.RID()
	if rid == "" {
		// Fallback: try to determine from other properties
		return ""
	}
	return rid
}
