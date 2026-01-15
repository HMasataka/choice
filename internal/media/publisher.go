package media

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Publisher manages tracks published by a participant.
// Per tasks.md 3.4.2: Publisher track management.
type Publisher struct {
	mu sync.RWMutex

	// ID is the participant ID of the publisher.
	ID string

	// tracks are the tracks published by this publisher.
	// Key is TrackID.
	tracks map[TrackID]*LocalTrack

	// metadata is publisher-level metadata.
	metadata map[string]string

	// createdAt is when the publisher was created.
	createdAt time.Time

	// updatedAt is when the publisher was last updated.
	updatedAt time.Time

	// onTrackAdded is called when a track is added.
	onTrackAdded func(track *LocalTrack)

	// onTrackRemoved is called when a track is removed.
	onTrackRemoved func(trackID TrackID)
}

// NewPublisher creates a new Publisher.
func NewPublisher(id string) *Publisher {
	now := time.Now()
	return &Publisher{
		ID:        id,
		tracks:    make(map[TrackID]*LocalTrack),
		metadata:  make(map[string]string),
		createdAt: now,
		updatedAt: now,
	}
}

// AddTrack adds a track to the publisher.
func (p *Publisher) AddTrack(ctx context.Context, track *LocalTrack) error {
	if track == nil {
		return fmt.Errorf("track cannot be nil")
	}
	if err := track.Validate(); err != nil {
		return fmt.Errorf("invalid track: %w", err)
	}
	if track.PublisherID != p.ID {
		return fmt.Errorf("track publisher ID %s does not match publisher %s", track.PublisherID, p.ID)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.tracks[track.ID]; exists {
		return fmt.Errorf("track %s already exists", track.ID)
	}

	p.tracks[track.ID] = track
	p.updatedAt = time.Now()

	// Capture callback before releasing lock
	onTrackAdded := p.onTrackAdded
	if onTrackAdded != nil {
		// Call outside of lock to prevent deadlock
		go onTrackAdded(track)
	}

	return nil
}

// RemoveTrack removes a track from the publisher.
func (p *Publisher) RemoveTrack(ctx context.Context, trackID TrackID) error {
	if err := trackID.Validate(); err != nil {
		return fmt.Errorf("invalid track ID: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.tracks[trackID]; !exists {
		return fmt.Errorf("track %s not found", trackID)
	}

	delete(p.tracks, trackID)
	p.updatedAt = time.Now()

	// Capture callback before releasing lock
	onTrackRemoved := p.onTrackRemoved
	if onTrackRemoved != nil {
		// Call outside of lock to prevent deadlock
		go onTrackRemoved(trackID)
	}

	return nil
}

// GetTrack retrieves a track by ID.
func (p *Publisher) GetTrack(trackID TrackID) (*LocalTrack, error) {
	if err := trackID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid track ID: %w", err)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	track, exists := p.tracks[trackID]
	if !exists {
		return nil, fmt.Errorf("track %s not found", trackID)
	}

	return track, nil
}

// ListTracks returns all tracks from this publisher.
func (p *Publisher) ListTracks() []*LocalTrack {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tracks := make([]*LocalTrack, 0, len(p.tracks))
	for _, track := range p.tracks {
		tracks = append(tracks, track)
	}
	return tracks
}

// GetSimulcastLayers returns the simulcast layers for a track.
func (p *Publisher) GetSimulcastLayers(trackID TrackID) ([]SimulcastLayer, error) {
	track, err := p.GetTrack(trackID)
	if err != nil {
		return nil, err
	}
	return track.GetLayers(), nil
}

// SetMetadata sets publisher metadata.
func (p *Publisher) SetMetadata(key, value string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.metadata[key] = value
	p.updatedAt = time.Now()
}

// GetMetadata gets publisher metadata.
func (p *Publisher) GetMetadata(key string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	value, ok := p.metadata[key]
	return value, ok
}

// GetAllMetadata returns a copy of all metadata.
func (p *Publisher) GetAllMetadata() map[string]string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]string, len(p.metadata))
	for k, v := range p.metadata {
		result[k] = v
	}
	return result
}

// TrackCount returns the number of tracks.
func (p *Publisher) TrackCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.tracks)
}

// SetOnTrackAdded sets the callback for track added events.
func (p *Publisher) SetOnTrackAdded(cb func(track *LocalTrack)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onTrackAdded = cb
}

// SetOnTrackRemoved sets the callback for track removed events.
func (p *Publisher) SetOnTrackRemoved(cb func(trackID TrackID)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onTrackRemoved = cb
}

// GetCreatedAt returns when the publisher was created.
func (p *Publisher) GetCreatedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.createdAt
}

// GetUpdatedAt returns when the publisher was last updated.
func (p *Publisher) GetUpdatedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.updatedAt
}

// PublisherManager manages multiple publishers.
type PublisherManager struct {
	mu         sync.RWMutex
	publishers map[string]*Publisher
}

// NewPublisherManager creates a new PublisherManager.
func NewPublisherManager() *PublisherManager {
	return &PublisherManager{
		publishers: make(map[string]*Publisher),
	}
}

// GetOrCreatePublisher gets an existing publisher or creates a new one.
func (m *PublisherManager) GetOrCreatePublisher(publisherID string) *Publisher {
	m.mu.Lock()
	defer m.mu.Unlock()

	if publisher, ok := m.publishers[publisherID]; ok {
		return publisher
	}

	publisher := NewPublisher(publisherID)
	m.publishers[publisherID] = publisher
	return publisher
}

// GetPublisher gets a publisher by ID.
func (m *PublisherManager) GetPublisher(publisherID string) (*Publisher, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	publisher, ok := m.publishers[publisherID]
	return publisher, ok
}

// RemovePublisher removes a publisher.
func (m *PublisherManager) RemovePublisher(publisherID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.publishers, publisherID)
}

// ListPublishers returns all publisher IDs.
func (m *PublisherManager) ListPublishers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.publishers))
	for id := range m.publishers {
		ids = append(ids, id)
	}
	return ids
}

// PublisherCount returns the number of publishers.
func (m *PublisherManager) PublisherCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.publishers)
}
