package sfu

import (
	"log/slog"
	"maps"
	"sync"

	"github.com/pion/webrtc/v4"
)

// TrackReceiver manages multiple quality layers for a single track.
// Video tracks have multiple layers (low/mid/high), while audio tracks have one layer.
type TrackReceiver struct {
	trackID    string
	streamID   string
	kind       webrtc.RTPCodecType
	layers     map[string]*Layer
	downTracks []*DownTrack
	mu         sync.RWMutex
	closed     bool
	closeCh    chan struct{}
}

// NewTrackReceiver creates a new track receiver.
func NewTrackReceiver(trackID, streamID string, kind webrtc.RTPCodecType) *TrackReceiver {
	return &TrackReceiver{
		trackID:  trackID,
		streamID: streamID,
		kind:     kind,
		layers:   make(map[string]*Layer),
		closeCh:  make(chan struct{}),
	}
}

// TrackID returns the track ID.
func (t *TrackReceiver) TrackID() string {
	return t.trackID
}

// StreamID returns the stream ID.
func (t *TrackReceiver) StreamID() string {
	return t.streamID
}

// Kind returns the track kind (audio/video).
func (t *TrackReceiver) Kind() webrtc.RTPCodecType {
	return t.kind
}

// AddLayer adds a new layer to the track.
func (t *TrackReceiver) AddLayer(name string, receiver *LayerReceiver) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}

	layer := NewLayer(name, receiver)
	t.layers[name] = layer

	slog.Info("Added layer for track", slog.String("trackID", t.trackID), slog.String("layer", name))
}

// GetLayer returns a specific layer by name.
func (t *TrackReceiver) GetLayer(name string) (*Layer, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	layer, ok := t.layers[name]
	return layer, ok
}

// GetLayers returns all layers.
func (t *TrackReceiver) GetLayers() map[string]*Layer {
	t.mu.RLock()
	defer t.mu.RUnlock()

	layers := make(map[string]*Layer, len(t.layers))
	maps.Copy(layers, t.layers)

	return layers
}

// GetBestLayer returns the highest quality active layer.
func (t *TrackReceiver) GetBestLayer() *Layer {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// For audio or single-layer tracks
	if layer, ok := t.layers[LayerDefault]; ok {
		return layer
	}

	// For video: priority high > mid > low
	for _, name := range []string{LayerHigh, LayerMid, LayerLow} {
		if layer, ok := t.layers[name]; ok && layer.IsActive() {
			return layer
		}
	}

	return nil
}

// AddDownTrack registers a downtrack to receive packets from this track.
func (t *TrackReceiver) AddDownTrack(dt *DownTrack) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.downTracks = append(t.downTracks, dt)
}

// RemoveDownTrack unregisters a downtrack.
func (t *TrackReceiver) RemoveDownTrack(dt *DownTrack) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i, d := range t.downTracks {
		if d == dt {
			t.downTracks = append(t.downTracks[:i], t.downTracks[i+1:]...)
			return
		}
	}
}

// Close closes the track receiver and all its layers.
func (t *TrackReceiver) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.closeCh)

	layers := make([]*Layer, 0, len(t.layers))
	for _, layer := range t.layers {
		layers = append(layers, layer)
	}
	t.mu.Unlock()

	for _, layer := range layers {
		if err := layer.receiver.Close(); err != nil {
			slog.Warn("layer receiver close error", slog.String("error", err.Error()))
		}
	}

	return nil
}
