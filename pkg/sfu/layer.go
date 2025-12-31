package sfu

import (
	"sync"

	"github.com/pion/webrtc/v4"
)

// Layer names
const (
	LayerHigh    = "high"
	LayerMid     = "mid"
	LayerLow     = "low"
	LayerDefault = "default" // For non-simulcast tracks (e.g., audio)
)

// Layer represents a single quality layer of a track.
// For video, this corresponds to simulcast layers (low/mid/high).
// For audio, there is only one "default" layer.
type Layer struct {
	name     string
	receiver *LayerReceiver
	ssrc     webrtc.SSRC
	active   bool
	mu       sync.RWMutex
}

// NewLayer creates a new layer.
func NewLayer(name string, receiver *LayerReceiver) *Layer {
	return &Layer{
		name:     name,
		receiver: receiver,
		ssrc:     receiver.SSRC(),
		active:   true,
	}
}

// Name returns the layer name.
func (l *Layer) Name() string {
	return l.name
}

// Receiver returns the layer's receiver.
func (l *Layer) Receiver() *LayerReceiver {
	return l.receiver
}

// SSRC returns the layer's SSRC.
func (l *Layer) SSRC() webrtc.SSRC {
	return l.ssrc
}

// IsActive returns whether the layer is active.
func (l *Layer) IsActive() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.active
}

// SetActive sets the layer's active state.
func (l *Layer) SetActive(active bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active = active
}

// LayerPriority returns the priority of a layer (higher is better).
func LayerPriority(name string) int {
	switch name {
	case LayerHigh:
		return 3
	case LayerMid:
		return 2
	case LayerLow:
		return 1
	case LayerDefault:
		return 1
	default:
		return 0
	}
}
