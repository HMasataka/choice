package room

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// ParticipantState represents the state of a participant.
type ParticipantState string

const (
	// ParticipantStateJoining indicates the participant is joining the room.
	ParticipantStateJoining ParticipantState = "joining"
	// ParticipantStateJoined indicates the participant has joined the room.
	ParticipantStateJoined ParticipantState = "joined"
	// ParticipantStatePublishing indicates the participant is publishing media.
	ParticipantStatePublishing ParticipantState = "publishing"
	// ParticipantStateSubscribing indicates the participant is subscribing to media.
	ParticipantStateSubscribing ParticipantState = "subscribing"
	// ParticipantStateLeaving indicates the participant is leaving the room.
	ParticipantStateLeaving ParticipantState = "leaving"
	// ParticipantStateLeft indicates the participant has left the room.
	ParticipantStateLeft ParticipantState = "left"
)

// Participant represents a participant in a room.
type Participant struct {
	mu sync.RWMutex

	// ID is the unique identifier for the participant.
	ID string
	// RoomID is the ID of the room the participant is in.
	RoomID string
	// State is the current state of the participant.
	State ParticipantState
	// Role is the role of the participant (e.g., admin, moderator, publisher, subscriber).
	Role string
	// Metadata is arbitrary metadata associated with the participant.
	Metadata map[string]interface{}
	// JoinedAt is the time when the participant joined the room.
	JoinedAt time.Time
	// UpdatedAt is the time when the participant was last updated.
	UpdatedAt time.Time

	// publishedTracks is a list of track IDs published by this participant.
	publishedTracks []string
	// subscriptions is a list of subscription IDs for this participant.
	subscriptions []string
}

// NewParticipant creates a new participant with the given ID and room ID.
func NewParticipant(id, roomID string, opts ...ParticipantOption) *Participant {
	if id == "" {
		id = uuid.New().String()
	}

	p := &Participant{
		ID:              id,
		RoomID:          roomID,
		State:           ParticipantStateJoining,
		Role:            "publisher", // Default role
		Metadata:        make(map[string]interface{}),
		JoinedAt:        time.Now(),
		UpdatedAt:       time.Now(),
		publishedTracks: make([]string, 0),
		subscriptions:   make([]string, 0),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// ParticipantOption is a functional option for configuring a participant.
type ParticipantOption func(*Participant)

// WithRole sets the role of the participant.
func WithRole(role string) ParticipantOption {
	return func(p *Participant) {
		p.Role = role
	}
}

// WithParticipantMetadata sets the metadata for the participant.
func WithParticipantMetadata(metadata map[string]interface{}) ParticipantOption {
	return func(p *Participant) {
		p.Metadata = metadata
	}
}

// SetState sets the state of the participant.
func (p *Participant) SetState(state ParticipantState) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.State = state
	p.UpdatedAt = time.Now()
}

// GetState returns the current state of the participant.
func (p *Participant) GetState() ParticipantState {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.State
}

// AddPublishedTrack adds a track ID to the list of published tracks.
func (p *Participant) AddPublishedTrack(trackID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.publishedTracks = append(p.publishedTracks, trackID)
	p.UpdatedAt = time.Now()

	// Update state to publishing if not already
	if p.State == ParticipantStateJoined {
		p.State = ParticipantStatePublishing
	}
}

// RemovePublishedTrack removes a track ID from the list of published tracks.
func (p *Participant) RemovePublishedTrack(trackID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, tid := range p.publishedTracks {
		if tid == trackID {
			p.publishedTracks = append(p.publishedTracks[:i], p.publishedTracks[i+1:]...)
			break
		}
	}
	p.UpdatedAt = time.Now()
}

// GetPublishedTracks returns the list of published track IDs.
func (p *Participant) GetPublishedTracks() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to prevent concurrent access issues
	tracks := make([]string, len(p.publishedTracks))
	copy(tracks, p.publishedTracks)
	return tracks
}

// AddSubscription adds a subscription ID to the list of subscriptions.
func (p *Participant) AddSubscription(subscriptionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.subscriptions = append(p.subscriptions, subscriptionID)
	p.UpdatedAt = time.Now()

	// Update state to subscribing if not already publishing
	if p.State == ParticipantStateJoined {
		p.State = ParticipantStateSubscribing
	}
}

// RemoveSubscription removes a subscription ID from the list of subscriptions.
func (p *Participant) RemoveSubscription(subscriptionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, sid := range p.subscriptions {
		if sid == subscriptionID {
			p.subscriptions = append(p.subscriptions[:i], p.subscriptions[i+1:]...)
			break
		}
	}
	p.UpdatedAt = time.Now()
}

// GetSubscriptions returns the list of subscription IDs.
func (p *Participant) GetSubscriptions() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to prevent concurrent access issues
	subs := make([]string, len(p.subscriptions))
	copy(subs, p.subscriptions)
	return subs
}

// GetMetadata returns the metadata for the participant.
func (p *Participant) GetMetadata() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy to prevent concurrent access issues
	metadata := make(map[string]interface{}, len(p.Metadata))
	for k, v := range p.Metadata {
		metadata[k] = v
	}

	return metadata
}

// SetMetadata sets the metadata for the participant.
func (p *Participant) SetMetadata(metadata map[string]interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Metadata = metadata
	p.UpdatedAt = time.Now()
}

// GetRole returns the role of the participant.
func (p *Participant) GetRole() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.Role
}

// SetRole sets the role of the participant.
func (p *Participant) SetRole(role string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.Role = role
	p.UpdatedAt = time.Now()
}
