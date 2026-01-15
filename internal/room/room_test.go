package room

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRoom(t *testing.T) {
	t.Run("creates room with default values", func(t *testing.T) {
		room := NewRoom("test-room")

		assert.Equal(t, "test-room", room.ID)
		assert.Equal(t, RoomStateCreated, room.State)
		assert.Equal(t, 100, room.MaxParticipants)
		assert.Equal(t, 500, room.MaxTracksPerRoom)
		assert.Equal(t, 5*time.Minute, room.EmptyTimeout)
		assert.NotNil(t, room.Metadata)
		assert.NotNil(t, room.participants)
	})

	t.Run("creates room with custom options", func(t *testing.T) {
		metadata := map[string]interface{}{"key": "value"}
		room := NewRoom("test-room",
			WithMaxParticipants(50),
			WithMaxTracksPerRoom(100),
			WithEmptyTimeout(10*time.Minute),
			WithMetadata(metadata),
		)

		assert.Equal(t, 50, room.MaxParticipants)
		assert.Equal(t, 100, room.MaxTracksPerRoom)
		assert.Equal(t, 10*time.Minute, room.EmptyTimeout)
		assert.Equal(t, metadata, room.Metadata)
	})
}

func TestRoom_AddParticipant(t *testing.T) {
	t.Run("adds participant successfully", func(t *testing.T) {
		room := NewRoom("test-room")
		participant := NewParticipant("participant-1", "test-room")

		err := room.AddParticipant(participant)
		require.NoError(t, err)

		assert.Equal(t, 1, room.ParticipantCount())
		assert.Equal(t, RoomStateActive, room.State)
	})

	t.Run("returns error when room is locked", func(t *testing.T) {
		room := NewRoom("test-room")
		err := room.Lock()
		require.NoError(t, err)

		participant := NewParticipant("participant-1", "test-room")
		err = room.AddParticipant(participant)
		assert.Equal(t, ErrRoomLocked, err)
	})

	t.Run("returns error when room is full", func(t *testing.T) {
		room := NewRoom("test-room", WithMaxParticipants(1))
		participant1 := NewParticipant("participant-1", "test-room")
		participant2 := NewParticipant("participant-2", "test-room")

		err := room.AddParticipant(participant1)
		require.NoError(t, err)

		err = room.AddParticipant(participant2)
		assert.Equal(t, ErrRoomFull, err)
	})

	t.Run("returns error when participant already joined", func(t *testing.T) {
		room := NewRoom("test-room")
		participant := NewParticipant("participant-1", "test-room")

		err := room.AddParticipant(participant)
		require.NoError(t, err)

		err = room.AddParticipant(participant)
		assert.Equal(t, ErrParticipantAlreadyJoined, err)
	})
}

func TestRoom_RemoveParticipant(t *testing.T) {
	t.Run("removes participant successfully", func(t *testing.T) {
		room := NewRoom("test-room")
		participant := NewParticipant("participant-1", "test-room")

		err := room.AddParticipant(participant)
		require.NoError(t, err)

		err = room.RemoveParticipant("participant-1")
		require.NoError(t, err)

		assert.Equal(t, 0, room.ParticipantCount())
		assert.Equal(t, RoomStateCreated, room.State)
	})

	t.Run("returns error when participant not found", func(t *testing.T) {
		room := NewRoom("test-room")

		err := room.RemoveParticipant("nonexistent")
		assert.Equal(t, ErrParticipantNotFound, err)
	})
}

func TestRoom_LockUnlock(t *testing.T) {
	t.Run("locks and unlocks room successfully", func(t *testing.T) {
		room := NewRoom("test-room")

		err := room.Lock()
		require.NoError(t, err)
		assert.Equal(t, RoomStateLocked, room.State)
		assert.True(t, room.IsLocked())

		err = room.Unlock()
		require.NoError(t, err)
		assert.Equal(t, RoomStateCreated, room.State)
		assert.False(t, room.IsLocked())
	})

	t.Run("returns error when unlocking non-locked room", func(t *testing.T) {
		room := NewRoom("test-room")

		err := room.Unlock()
		assert.Equal(t, ErrRoomNotLocked, err)
	})
}

func TestRoom_TrackCount(t *testing.T) {
	t.Run("increments and decrements track count", func(t *testing.T) {
		room := NewRoom("test-room")

		err := room.IncrementTrackCount()
		require.NoError(t, err)
		assert.Equal(t, 1, room.TrackCount())

		err = room.IncrementTrackCount()
		require.NoError(t, err)
		assert.Equal(t, 2, room.TrackCount())

		room.DecrementTrackCount()
		assert.Equal(t, 1, room.TrackCount())
	})

	t.Run("returns error when max tracks reached", func(t *testing.T) {
		room := NewRoom("test-room", WithMaxTracksPerRoom(2))

		err := room.IncrementTrackCount()
		require.NoError(t, err)

		err = room.IncrementTrackCount()
		require.NoError(t, err)

		err = room.IncrementTrackCount()
		assert.Equal(t, ErrMaxTracksReached, err)
	})
}

func TestRoom_Close(t *testing.T) {
	t.Run("closes room successfully", func(t *testing.T) {
		room := NewRoom("test-room")
		participant := NewParticipant("participant-1", "test-room")

		err := room.AddParticipant(participant)
		require.NoError(t, err)

		err = room.Close()
		require.NoError(t, err)

		assert.Equal(t, RoomStateClosed, room.State)
		assert.Equal(t, 0, room.ParticipantCount())
		assert.True(t, room.IsClosed())
	})
}
