package room

import (
	"sync"
	"time"
)

// EventType represents the type of room event.
type EventType string

const (
	// EventParticipantJoined is emitted when a participant joins the room.
	EventParticipantJoined EventType = "participantJoined"
	// EventParticipantLeft is emitted when a participant leaves the room.
	EventParticipantLeft EventType = "participantLeft"
	// EventParticipantReconnected is emitted when a participant reconnects to the room.
	EventParticipantReconnected EventType = "participantReconnected"
	// EventTrackPublished is emitted when a track is published.
	EventTrackPublished EventType = "trackPublished"
	// EventTrackUnpublished is emitted when a track is unpublished.
	EventTrackUnpublished EventType = "trackUnpublished"
	// EventRoomStateChanged is emitted when the room state changes.
	EventRoomStateChanged EventType = "roomStateChanged"
)

// Event represents a room event.
type Event struct {
	// Type is the type of event.
	Type EventType
	// RoomID is the ID of the room where the event occurred.
	RoomID string
	// ParticipantID is the ID of the participant (if applicable).
	ParticipantID string
	// TrackID is the ID of the track (if applicable).
	TrackID string
	// Metadata is additional event-specific metadata.
	Metadata map[string]interface{}
	// Timestamp is when the event occurred.
	Timestamp time.Time
}

// EventHandler is a function that handles room events.
type EventHandler func(event *Event)

// EventEmitter manages event listeners and emits events.
type EventEmitter struct {
	mu        sync.RWMutex
	handlers  map[EventType][]EventHandler
	allEvents []EventHandler
}

// NewEventEmitter creates a new event emitter.
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		handlers:  make(map[EventType][]EventHandler),
		allEvents: make([]EventHandler, 0),
	}
}

// On registers an event handler for a specific event type.
func (e *EventEmitter) On(eventType EventType, handler EventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.handlers[eventType] == nil {
		e.handlers[eventType] = make([]EventHandler, 0)
	}

	e.handlers[eventType] = append(e.handlers[eventType], handler)
}

// OnAll registers an event handler for all event types.
func (e *EventEmitter) OnAll(handler EventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.allEvents = append(e.allEvents, handler)
}

// Emit emits an event to all registered handlers.
func (e *EventEmitter) Emit(event *Event) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Call specific event handlers
	if handlers, exists := e.handlers[event.Type]; exists {
		for _, handler := range handlers {
			go handler(event)
		}
	}

	// Call all-event handlers
	for _, handler := range e.allEvents {
		go handler(event)
	}
}

// RemoveAllHandlers removes all event handlers.
func (e *EventEmitter) RemoveAllHandlers() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.handlers = make(map[EventType][]EventHandler)
	e.allEvents = make([]EventHandler, 0)
}

// CreateParticipantJoinedEvent creates a participant joined event.
func CreateParticipantJoinedEvent(roomID, participantID string, metadata map[string]interface{}) *Event {
	return &Event{
		Type:          EventParticipantJoined,
		RoomID:        roomID,
		ParticipantID: participantID,
		Metadata:      metadata,
		Timestamp:     time.Now(),
	}
}

// CreateParticipantLeftEvent creates a participant left event.
func CreateParticipantLeftEvent(roomID, participantID, reason string) *Event {
	metadata := map[string]interface{}{
		"reason": reason,
	}
	return &Event{
		Type:          EventParticipantLeft,
		RoomID:        roomID,
		ParticipantID: participantID,
		Metadata:      metadata,
		Timestamp:     time.Now(),
	}
}

// CreateParticipantReconnectedEvent creates a participant reconnected event.
func CreateParticipantReconnectedEvent(roomID, participantID string, metadata map[string]interface{}) *Event {
	return &Event{
		Type:          EventParticipantReconnected,
		RoomID:        roomID,
		ParticipantID: participantID,
		Metadata:      metadata,
		Timestamp:     time.Now(),
	}
}

// CreateTrackPublishedEvent creates a track published event.
func CreateTrackPublishedEvent(roomID, participantID, trackID, kind string, simulcast bool, metadata map[string]interface{}) *Event {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["kind"] = kind
	metadata["simulcast"] = simulcast

	return &Event{
		Type:          EventTrackPublished,
		RoomID:        roomID,
		ParticipantID: participantID,
		TrackID:       trackID,
		Metadata:      metadata,
		Timestamp:     time.Now(),
	}
}

// CreateTrackUnpublishedEvent creates a track unpublished event.
func CreateTrackUnpublishedEvent(roomID, participantID, trackID string) *Event {
	return &Event{
		Type:          EventTrackUnpublished,
		RoomID:        roomID,
		ParticipantID: participantID,
		TrackID:       trackID,
		Timestamp:     time.Now(),
	}
}

// CreateRoomStateChangedEvent creates a room state changed event.
func CreateRoomStateChangedEvent(roomID string, oldState, newState RoomState) *Event {
	metadata := map[string]interface{}{
		"oldState": string(oldState),
		"newState": string(newState),
	}
	return &Event{
		Type:      EventRoomStateChanged,
		RoomID:    roomID,
		Metadata:  metadata,
		Timestamp: time.Now(),
	}
}
