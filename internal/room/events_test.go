package room

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewEventEmitter(t *testing.T) {
	emitter := NewEventEmitter()

	assert.NotNil(t, emitter)
	assert.NotNil(t, emitter.handlers)
	assert.NotNil(t, emitter.allEvents)
}

func TestEventEmitter_On(t *testing.T) {
	t.Run("registers handler for event type", func(t *testing.T) {
		emitter := NewEventEmitter()
		var called int32
		done := make(chan struct{})

		emitter.On(EventParticipantJoined, func(event *Event) {
			atomic.StoreInt32(&called, 1)
			done <- struct{}{}
		})

		event := &Event{Type: EventParticipantJoined}
		emitter.Emit(event)

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
		assert.Equal(t, int32(1), atomic.LoadInt32(&called))
	})

	t.Run("registers multiple handlers for same event type", func(t *testing.T) {
		emitter := NewEventEmitter()
		var count int32
		var wg sync.WaitGroup
		wg.Add(2)

		emitter.On(EventParticipantJoined, func(event *Event) {
			atomic.AddInt32(&count, 1)
			wg.Done()
		})
		emitter.On(EventParticipantJoined, func(event *Event) {
			atomic.AddInt32(&count, 1)
			wg.Done()
		})

		event := &Event{Type: EventParticipantJoined}
		emitter.Emit(event)

		wg.Wait()
		assert.Equal(t, int32(2), atomic.LoadInt32(&count))
	})
}

func TestEventEmitter_OnAll(t *testing.T) {
	t.Run("receives all event types", func(t *testing.T) {
		emitter := NewEventEmitter()
		var events []*Event
		var mu sync.Mutex

		emitter.OnAll(func(event *Event) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		})

		emitter.Emit(&Event{Type: EventParticipantJoined})
		emitter.Emit(&Event{Type: EventParticipantLeft})
		emitter.Emit(&Event{Type: EventTrackPublished})

		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		assert.Len(t, events, 3)
	})
}

func TestEventEmitter_Emit(t *testing.T) {
	t.Run("emits to specific handlers", func(t *testing.T) {
		emitter := NewEventEmitter()
		var joinedCalled int32
		var leftCalled int32
		done := make(chan struct{})

		emitter.On(EventParticipantJoined, func(event *Event) {
			atomic.StoreInt32(&joinedCalled, 1)
			done <- struct{}{}
		})
		emitter.On(EventParticipantLeft, func(event *Event) {
			atomic.StoreInt32(&leftCalled, 1)
		})

		emitter.Emit(&Event{Type: EventParticipantJoined})

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
		assert.Equal(t, int32(1), atomic.LoadInt32(&joinedCalled))
		assert.Equal(t, int32(0), atomic.LoadInt32(&leftCalled))
	})

	t.Run("emits to both specific and all handlers", func(t *testing.T) {
		emitter := NewEventEmitter()
		var specificCalled, allCalled int32
		var wg sync.WaitGroup
		wg.Add(2)

		emitter.On(EventParticipantJoined, func(event *Event) {
			atomic.StoreInt32(&specificCalled, 1)
			wg.Done()
		})
		emitter.OnAll(func(event *Event) {
			atomic.StoreInt32(&allCalled, 1)
			wg.Done()
		})

		emitter.Emit(&Event{Type: EventParticipantJoined})

		wg.Wait()
		assert.Equal(t, int32(1), atomic.LoadInt32(&specificCalled))
		assert.Equal(t, int32(1), atomic.LoadInt32(&allCalled))
	})

	t.Run("no handlers does not panic", func(t *testing.T) {
		emitter := NewEventEmitter()
		assert.NotPanics(t, func() {
			emitter.Emit(&Event{Type: EventParticipantJoined})
		})
	})
}

func TestEventEmitter_RemoveAllHandlers(t *testing.T) {
	emitter := NewEventEmitter()
	var count int32

	emitter.On(EventParticipantJoined, func(event *Event) {
		atomic.AddInt32(&count, 1)
	})
	emitter.OnAll(func(event *Event) {
		atomic.AddInt32(&count, 1)
	})

	emitter.RemoveAllHandlers()
	emitter.Emit(&Event{Type: EventParticipantJoined})

	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&count))
}

func TestEventEmitter_Concurrency(t *testing.T) {
	emitter := NewEventEmitter()
	var count int32

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Register handlers concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			emitter.On(EventParticipantJoined, func(event *Event) {
				atomic.AddInt32(&count, 1)
			})
		}()

		// Emit events concurrently
		go func() {
			defer wg.Done()
			emitter.Emit(&Event{Type: EventParticipantJoined})
		}()
	}

	wg.Wait()
	// Should not panic or race
}

func TestCreateParticipantJoinedEvent(t *testing.T) {
	metadata := map[string]interface{}{"displayName": "Alice"}
	event := CreateParticipantJoinedEvent("room-1", "participant-1", metadata)

	assert.Equal(t, EventParticipantJoined, event.Type)
	assert.Equal(t, "room-1", event.RoomID)
	assert.Equal(t, "participant-1", event.ParticipantID)
	assert.Equal(t, "Alice", event.Metadata["displayName"])
	assert.False(t, event.Timestamp.IsZero())
}

func TestCreateParticipantLeftEvent(t *testing.T) {
	event := CreateParticipantLeftEvent("room-1", "participant-1", "disconnect")

	assert.Equal(t, EventParticipantLeft, event.Type)
	assert.Equal(t, "room-1", event.RoomID)
	assert.Equal(t, "participant-1", event.ParticipantID)
	assert.Equal(t, "disconnect", event.Metadata["reason"])
	assert.False(t, event.Timestamp.IsZero())
}

func TestCreateParticipantReconnectedEvent(t *testing.T) {
	metadata := map[string]interface{}{"previousSessionId": "session-old"}
	event := CreateParticipantReconnectedEvent("room-1", "participant-1", metadata)

	assert.Equal(t, EventParticipantReconnected, event.Type)
	assert.Equal(t, "room-1", event.RoomID)
	assert.Equal(t, "participant-1", event.ParticipantID)
	assert.Equal(t, "session-old", event.Metadata["previousSessionId"])
	assert.False(t, event.Timestamp.IsZero())
}

func TestCreateTrackPublishedEvent(t *testing.T) {
	t.Run("creates event with metadata", func(t *testing.T) {
		metadata := map[string]interface{}{"label": "camera"}
		event := CreateTrackPublishedEvent("room-1", "participant-1", "track-1", "video", true, metadata)

		assert.Equal(t, EventTrackPublished, event.Type)
		assert.Equal(t, "room-1", event.RoomID)
		assert.Equal(t, "participant-1", event.ParticipantID)
		assert.Equal(t, "track-1", event.TrackID)
		assert.Equal(t, "video", event.Metadata["kind"])
		assert.Equal(t, true, event.Metadata["simulcast"])
		assert.Equal(t, "camera", event.Metadata["label"])
		assert.False(t, event.Timestamp.IsZero())
	})

	t.Run("creates event without metadata", func(t *testing.T) {
		event := CreateTrackPublishedEvent("room-1", "participant-1", "track-1", "audio", false, nil)

		assert.Equal(t, EventTrackPublished, event.Type)
		assert.Equal(t, "audio", event.Metadata["kind"])
		assert.Equal(t, false, event.Metadata["simulcast"])
	})
}

func TestCreateTrackUnpublishedEvent(t *testing.T) {
	event := CreateTrackUnpublishedEvent("room-1", "participant-1", "track-1")

	assert.Equal(t, EventTrackUnpublished, event.Type)
	assert.Equal(t, "room-1", event.RoomID)
	assert.Equal(t, "participant-1", event.ParticipantID)
	assert.Equal(t, "track-1", event.TrackID)
	assert.False(t, event.Timestamp.IsZero())
}

func TestCreateRoomStateChangedEvent(t *testing.T) {
	event := CreateRoomStateChangedEvent("room-1", RoomStateCreated, RoomStateActive)

	assert.Equal(t, EventRoomStateChanged, event.Type)
	assert.Equal(t, "room-1", event.RoomID)
	assert.Equal(t, string(RoomStateCreated), event.Metadata["oldState"])
	assert.Equal(t, string(RoomStateActive), event.Metadata["newState"])
	assert.False(t, event.Timestamp.IsZero())
}
