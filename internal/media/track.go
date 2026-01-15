package media

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

// TrackKind represents the type of media track.
type TrackKind string

const (
	// TrackKindVideo represents a video track.
	TrackKindVideo TrackKind = "video"
	// TrackKindAudio represents an audio track.
	TrackKindAudio TrackKind = "audio"
)

// String returns the string representation of TrackKind.
func (k TrackKind) String() string {
	return string(k)
}

// Validate checks if the TrackKind is valid.
func (k TrackKind) Validate() error {
	switch k {
	case TrackKindVideo, TrackKindAudio:
		return nil
	default:
		return fmt.Errorf("invalid track kind: %s", k)
	}
}

// SimulcastLayer represents a simulcast quality layer.
type SimulcastLayer string

const (
	// SimulcastLayerHigh represents the high quality layer (h).
	// Per requirements.md: 1280x720, 2.5Mbps, 30fps
	SimulcastLayerHigh SimulcastLayer = "h"
	// SimulcastLayerMedium represents the medium quality layer (m).
	// Per requirements.md: 640x360, 500Kbps, 30fps
	SimulcastLayerMedium SimulcastLayer = "m"
	// SimulcastLayerLow represents the low quality layer (l).
	// Per requirements.md: 320x180, 150Kbps, 15fps
	SimulcastLayerLow SimulcastLayer = "l"
)

// String returns the string representation of SimulcastLayer.
func (l SimulcastLayer) String() string {
	return string(l)
}

// Validate checks if the SimulcastLayer is valid.
func (l SimulcastLayer) Validate() error {
	switch l {
	case SimulcastLayerHigh, SimulcastLayerMedium, SimulcastLayerLow:
		return nil
	default:
		return fmt.Errorf("invalid simulcast layer: %s", l)
	}
}

// TrackID is a unique identifier for a track.
// Generated server-side using UUID v4.
type TrackID string

// GenerateTrackID generates a new unique track ID.
func GenerateTrackID() TrackID {
	return TrackID(uuid.New().String())
}

// String returns the string representation of TrackID.
func (id TrackID) String() string {
	return string(id)
}

// Validate checks if the TrackID is valid (non-empty and valid UUID v4 format).
func (id TrackID) Validate() error {
	if id == "" {
		return fmt.Errorf("track ID cannot be empty")
	}
	// Validate UUID format
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return fmt.Errorf("track ID must be a valid UUID: %w", err)
	}
	// Ensure UUID version 4 (per requirements.md)
	if parsed.Version() != 4 {
		return fmt.Errorf("track ID must be UUID v4, got v%d", parsed.Version())
	}
	return nil
}

// TrackMetadata contains metadata about a track.
type TrackMetadata struct {
	// Label is a user-defined label for the track (e.g., "camera", "screen").
	Label string

	// Simulcast indicates whether simulcast is enabled for this track.
	Simulcast bool

	// Layers contains the available simulcast layers (if simulcast is enabled).
	// Per requirements.md: h (high), m (medium), l (low)
	Layers []SimulcastLayer

	// MID is the Media ID from SDP (m-line identifier).
	MID string

	// SSRC is the Synchronization Source identifier for RTP.
	SSRC uint32

	// Custom contains additional custom metadata.
	Custom map[string]interface{}
}

// Copy creates a deep copy of TrackMetadata.
func (m *TrackMetadata) Copy() *TrackMetadata {
	if m == nil {
		return nil
	}

	layers := make([]SimulcastLayer, len(m.Layers))
	copy(layers, m.Layers)

	custom := make(map[string]interface{}, len(m.Custom))
	for k, v := range m.Custom {
		custom[k] = v
	}

	return &TrackMetadata{
		Label:     m.Label,
		Simulcast: m.Simulcast,
		Layers:    layers,
		MID:       m.MID,
		SSRC:      m.SSRC,
		Custom:    custom,
	}
}

// LocalTrack represents a track published by a participant.
// This is a track received from a publisher client.
// Note: Access metadata only through GetMetadata/UpdateMetadata methods for thread safety.
type LocalTrack struct {
	// ID is the unique identifier for this track (server-generated).
	ID TrackID

	// Kind is the type of track (video or audio).
	Kind TrackKind

	// PublisherID is the ID of the participant who published this track.
	PublisherID string

	// RoomID is the ID of the room this track belongs to.
	RoomID string

	// metadata contains additional information about the track.
	// Use GetMetadata() and UpdateMetadata() for thread-safe access.
	metadata *TrackMetadata

	// Track is the underlying WebRTC track from pion/webrtc.
	Track *webrtc.TrackRemote

	// CreatedAt is the time when this track was created.
	CreatedAt time.Time

	// UpdatedAt is the time when this track was last updated.
	UpdatedAt time.Time

	// mu protects concurrent access to mutable fields.
	mu sync.RWMutex
}

// NewLocalTrack creates a new LocalTrack.
// Note: The metadata is copied to prevent external modifications.
func NewLocalTrack(publisherID, roomID string, kind TrackKind, track *webrtc.TrackRemote, meta *TrackMetadata) *LocalTrack {
	now := time.Now()
	return &LocalTrack{
		ID:          GenerateTrackID(),
		Kind:        kind,
		PublisherID: publisherID,
		RoomID:      roomID,
		metadata:    meta.Copy(), // Store a copy to prevent external modifications
		Track:       track,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// GetMetadata returns a copy of the track metadata (thread-safe).
func (t *LocalTrack) GetMetadata() *TrackMetadata {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.metadata.Copy()
}

// UpdateMetadata updates the track metadata (thread-safe).
func (t *LocalTrack) UpdateMetadata(meta *TrackMetadata) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.metadata = meta.Copy()
	t.UpdatedAt = time.Now()
}

// GetSSRC returns the SSRC of the track (thread-safe).
func (t *LocalTrack) GetSSRC() uint32 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.metadata != nil {
		return t.metadata.SSRC
	}
	return 0
}

// GetMID returns the MID of the track (thread-safe).
func (t *LocalTrack) GetMID() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.metadata != nil {
		return t.metadata.MID
	}
	return ""
}

// IsSimulcast returns whether the track uses simulcast (thread-safe).
func (t *LocalTrack) IsSimulcast() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.metadata != nil {
		return t.metadata.Simulcast
	}
	return false
}

// GetLayers returns the available simulcast layers (thread-safe).
func (t *LocalTrack) GetLayers() []SimulcastLayer {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.metadata != nil && len(t.metadata.Layers) > 0 {
		layers := make([]SimulcastLayer, len(t.metadata.Layers))
		copy(layers, t.metadata.Layers)
		return layers
	}
	return nil
}

// SetSimulcast sets whether simulcast is enabled for this track (thread-safe).
func (t *LocalTrack) SetSimulcast(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.metadata == nil {
		t.metadata = &TrackMetadata{}
	}
	t.metadata.Simulcast = enabled
	t.UpdatedAt = time.Now()
}

// SetLayers sets the available simulcast layers (thread-safe).
func (t *LocalTrack) SetLayers(layers []SimulcastLayer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.metadata == nil {
		t.metadata = &TrackMetadata{}
	}
	t.metadata.Layers = make([]SimulcastLayer, len(layers))
	copy(t.metadata.Layers, layers)
	t.UpdatedAt = time.Now()
}

// Validate validates the LocalTrack.
func (t *LocalTrack) Validate() error {
	if err := t.ID.Validate(); err != nil {
		return fmt.Errorf("invalid track ID: %w", err)
	}
	if err := t.Kind.Validate(); err != nil {
		return fmt.Errorf("invalid track kind: %w", err)
	}
	if t.PublisherID == "" {
		return fmt.Errorf("publisher ID cannot be empty")
	}
	if t.RoomID == "" {
		return fmt.Errorf("room ID cannot be empty")
	}
	if t.Track == nil {
		return fmt.Errorf("webrtc track cannot be nil")
	}
	// Validate metadata consistency
	if err := t.validateMetadata(); err != nil {
		return fmt.Errorf("invalid metadata: %w", err)
	}
	return nil
}

// validateMetadata validates the track metadata consistency.
func (t *LocalTrack) validateMetadata() error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.metadata == nil {
		return nil // nil metadata is allowed
	}

	// If simulcast is enabled, layers should not be empty
	if t.metadata.Simulcast && len(t.metadata.Layers) == 0 {
		return fmt.Errorf("simulcast enabled but no layers specified")
	}

	// Validate each layer
	for _, layer := range t.metadata.Layers {
		if err := layer.Validate(); err != nil {
			return fmt.Errorf("invalid simulcast layer: %w", err)
		}
	}

	return nil
}

// TrackInfo contains information about a track for serialization.
// This is used in signaling responses.
type TrackInfo struct {
	ID          string                 `json:"id"`
	Kind        string                 `json:"kind"`
	PublisherID string                 `json:"publisherId"`
	Label       string                 `json:"label,omitempty"`
	Simulcast   bool                   `json:"simulcast"`
	Layers      []string               `json:"layers,omitempty"`
	MID         string                 `json:"mid,omitempty"`
	Custom      map[string]interface{} `json:"custom,omitempty"`
}

// ToTrackInfo converts LocalTrack to TrackInfo for serialization.
// Returns a deep copy of all data to prevent external modifications.
func (t *LocalTrack) ToTrackInfo() *TrackInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	info := &TrackInfo{
		ID:          t.ID.String(),
		Kind:        t.Kind.String(),
		PublisherID: t.PublisherID,
	}

	if t.metadata != nil {
		info.Label = t.metadata.Label
		info.Simulcast = t.metadata.Simulcast
		info.MID = t.metadata.MID

		// Deep copy Custom map to prevent external modifications
		if t.metadata.Custom != nil {
			info.Custom = make(map[string]interface{}, len(t.metadata.Custom))
			for k, v := range t.metadata.Custom {
				info.Custom[k] = v
			}
		}

		if len(t.metadata.Layers) > 0 {
			info.Layers = make([]string, len(t.metadata.Layers))
			for i, layer := range t.metadata.Layers {
				info.Layers[i] = layer.String()
			}
		}
	}

	return info
}
