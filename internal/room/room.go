package room

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// RoomState represents the state of a room.
type RoomState string

const (
	// RoomStateCreated indicates the room has been created but has no participants.
	RoomStateCreated RoomState = "created"
	// RoomStateActive indicates the room has one or more participants.
	RoomStateActive RoomState = "active"
	// RoomStateLocked indicates the room is locked and new participants cannot join.
	RoomStateLocked RoomState = "locked"
	// RoomStateClosing indicates the room is in the process of closing.
	RoomStateClosing RoomState = "closing"
	// RoomStateClosed indicates the room has been closed.
	RoomStateClosed RoomState = "closed"
)

// Room represents a room where participants can join and communicate.
type Room struct {
	mu sync.RWMutex

	// ID is the unique identifier for the room.
	ID string
	// State is the current state of the room.
	State RoomState
	// MaxParticipants is the maximum number of participants allowed in the room.
	MaxParticipants int
	// MaxTracksPerRoom is the maximum number of tracks allowed in the room.
	MaxTracksPerRoom int
	// EmptyTimeout is the duration after which an empty room will be closed.
	EmptyTimeout time.Duration
	// Metadata is arbitrary metadata associated with the room.
	Metadata map[string]interface{}
	// CreatedAt is the time when the room was created.
	CreatedAt time.Time
	// UpdatedAt is the time when the room was last updated.
	UpdatedAt time.Time

	// participants is a map of participant IDs to participants.
	participants map[string]*Participant
	// trackCount is the current number of tracks in the room.
	trackCount int
	// emptyTimer is the timer for closing an empty room.
	emptyTimer *time.Timer
}

// NewRoom creates a new room with the given ID and options.
func NewRoom(id string, opts ...RoomOption) *Room {
	if id == "" {
		id = uuid.New().String()
	}

	r := &Room{
		ID:               id,
		State:            RoomStateCreated,
		MaxParticipants:  100,  // Default maximum participants
		MaxTracksPerRoom: 500,  // Default maximum tracks per room
		EmptyTimeout:     5 * time.Minute, // Default empty timeout
		Metadata:         make(map[string]interface{}),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		participants:     make(map[string]*Participant),
		trackCount:       0,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// RoomOption is a functional option for configuring a room.
type RoomOption func(*Room)

// WithMaxParticipants sets the maximum number of participants allowed in the room.
func WithMaxParticipants(max int) RoomOption {
	return func(r *Room) {
		r.MaxParticipants = max
	}
}

// WithMaxTracksPerRoom sets the maximum number of tracks allowed in the room.
func WithMaxTracksPerRoom(max int) RoomOption {
	return func(r *Room) {
		r.MaxTracksPerRoom = max
	}
}

// WithEmptyTimeout sets the duration after which an empty room will be closed.
func WithEmptyTimeout(timeout time.Duration) RoomOption {
	return func(r *Room) {
		r.EmptyTimeout = timeout
	}
}

// WithMetadata sets the metadata for the room.
func WithMetadata(metadata map[string]interface{}) RoomOption {
	return func(r *Room) {
		r.Metadata = metadata
	}
}

// AddParticipant adds a participant to the room.
func (r *Room) AddParticipant(p *Participant) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.State == RoomStateLocked {
		return ErrRoomLocked
	}

	if r.State == RoomStateClosing || r.State == RoomStateClosed {
		return ErrRoomClosed
	}

	if len(r.participants) >= r.MaxParticipants {
		return ErrRoomFull
	}

	if _, exists := r.participants[p.ID]; exists {
		return ErrParticipantAlreadyJoined
	}

	r.participants[p.ID] = p
	r.UpdatedAt = time.Now()

	// Update room state
	if r.State == RoomStateCreated {
		r.State = RoomStateActive
	}

	// Cancel empty timer if it was set
	if r.emptyTimer != nil {
		r.emptyTimer.Stop()
		r.emptyTimer = nil
	}

	return nil
}

// RemoveParticipant removes a participant from the room.
func (r *Room) RemoveParticipant(participantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.participants[participantID]; !exists {
		return ErrParticipantNotFound
	}

	delete(r.participants, participantID)
	r.UpdatedAt = time.Now()

	// Update room state
	if len(r.participants) == 0 {
		r.State = RoomStateCreated
		// Start empty timer
		if r.EmptyTimeout > 0 {
			r.emptyTimer = time.AfterFunc(r.EmptyTimeout, func() {
				r.Close()
			})
		}
	}

	return nil
}

// GetParticipant returns a participant by ID.
func (r *Room) GetParticipant(participantID string) (*Participant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.participants[participantID]
	if !exists {
		return nil, ErrParticipantNotFound
	}

	return p, nil
}

// GetParticipants returns all participants in the room.
func (r *Room) GetParticipants() []*Participant {
	r.mu.RLock()
	defer r.mu.RUnlock()

	participants := make([]*Participant, 0, len(r.participants))
	for _, p := range r.participants {
		participants = append(participants, p)
	}

	return participants
}

// ParticipantCount returns the number of participants in the room.
func (r *Room) ParticipantCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.participants)
}

// Lock locks the room, preventing new participants from joining.
func (r *Room) Lock() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.State == RoomStateClosing || r.State == RoomStateClosed {
		return ErrRoomClosed
	}

	r.State = RoomStateLocked
	r.UpdatedAt = time.Now()

	return nil
}

// Unlock unlocks the room, allowing new participants to join.
func (r *Room) Unlock() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.State == RoomStateClosing || r.State == RoomStateClosed {
		return ErrRoomClosed
	}

	if r.State != RoomStateLocked {
		return ErrRoomNotLocked
	}

	if len(r.participants) == 0 {
		r.State = RoomStateCreated
	} else {
		r.State = RoomStateActive
	}
	r.UpdatedAt = time.Now()

	return nil
}

// IsLocked returns true if the room is locked.
func (r *Room) IsLocked() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.State == RoomStateLocked
}

// IncrementTrackCount increments the track count for the room.
func (r *Room) IncrementTrackCount() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.trackCount >= r.MaxTracksPerRoom {
		return ErrMaxTracksReached
	}

	r.trackCount++
	return nil
}

// DecrementTrackCount decrements the track count for the room.
func (r *Room) DecrementTrackCount() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.trackCount > 0 {
		r.trackCount--
	}
}

// TrackCount returns the current number of tracks in the room.
func (r *Room) TrackCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.trackCount
}

// Close closes the room and cleans up resources.
func (r *Room) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.State == RoomStateClosed {
		return nil
	}

	r.State = RoomStateClosing

	// Stop empty timer if it's running
	if r.emptyTimer != nil {
		r.emptyTimer.Stop()
		r.emptyTimer = nil
	}

	// Clear participants
	r.participants = make(map[string]*Participant)
	r.trackCount = 0

	r.State = RoomStateClosed
	r.UpdatedAt = time.Now()

	return nil
}

// IsClosed returns true if the room is closed.
func (r *Room) IsClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.State == RoomStateClosed
}

// GetState returns the current state of the room.
func (r *Room) GetState() RoomState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.State
}

// GetMetadata returns the metadata for the room.
func (r *Room) GetMetadata() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent concurrent access issues
	metadata := make(map[string]interface{}, len(r.Metadata))
	for k, v := range r.Metadata {
		metadata[k] = v
	}

	return metadata
}

// SetMetadata sets the metadata for the room.
func (r *Room) SetMetadata(metadata map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Metadata = metadata
	r.UpdatedAt = time.Now()
}
