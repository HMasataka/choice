package room

import (
	"sync"
	"testing"

	"github.com/HMasataka/choice/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) *logger.Logger {
	log, err := logger.New(logger.Config{Level: "error"})
	require.NoError(t, err)
	return log
}

func TestNewManager(t *testing.T) {
	log := newTestLogger(t)
	manager := NewManager(log)

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.rooms)
	assert.Equal(t, 0, manager.RoomCount())
}

func TestManager_CreateRoom(t *testing.T) {
	log := newTestLogger(t)

	t.Run("creates room successfully", func(t *testing.T) {
		manager := NewManager(log)
		room, err := manager.CreateRoom("room-1")

		require.NoError(t, err)
		assert.NotNil(t, room)
		assert.Equal(t, "room-1", room.ID)
		assert.Equal(t, 1, manager.RoomCount())
	})

	t.Run("creates room with options", func(t *testing.T) {
		manager := NewManager(log)
		room, err := manager.CreateRoom("room-1", WithMaxParticipants(50))

		require.NoError(t, err)
		assert.Equal(t, 50, room.MaxParticipants)
	})

	t.Run("returns error when room already exists", func(t *testing.T) {
		manager := NewManager(log)
		_, err := manager.CreateRoom("room-1")
		require.NoError(t, err)

		_, err = manager.CreateRoom("room-1")
		assert.Equal(t, ErrRoomAlreadyExists, err)
	})
}

func TestManager_GetRoom(t *testing.T) {
	log := newTestLogger(t)

	t.Run("returns room when exists", func(t *testing.T) {
		manager := NewManager(log)
		created, err := manager.CreateRoom("room-1")
		require.NoError(t, err)

		got, err := manager.GetRoom("room-1")
		require.NoError(t, err)
		assert.Equal(t, created, got)
	})

	t.Run("returns error when room not found", func(t *testing.T) {
		manager := NewManager(log)

		_, err := manager.GetRoom("nonexistent")
		assert.Equal(t, ErrRoomNotFound, err)
	})
}

func TestManager_DeleteRoom(t *testing.T) {
	log := newTestLogger(t)

	t.Run("deletes room successfully", func(t *testing.T) {
		manager := NewManager(log)
		_, err := manager.CreateRoom("room-1")
		require.NoError(t, err)

		err = manager.DeleteRoom("room-1")
		require.NoError(t, err)
		assert.Equal(t, 0, manager.RoomCount())
	})

	t.Run("returns error when room not found", func(t *testing.T) {
		manager := NewManager(log)

		err := manager.DeleteRoom("nonexistent")
		assert.Equal(t, ErrRoomNotFound, err)
	})

	t.Run("closes room before deleting", func(t *testing.T) {
		manager := NewManager(log)
		room, err := manager.CreateRoom("room-1")
		require.NoError(t, err)

		// Add a participant so room has state
		participant := NewParticipant("p1", "room-1")
		err = room.AddParticipant(participant)
		require.NoError(t, err)

		err = manager.DeleteRoom("room-1")
		require.NoError(t, err)
		assert.True(t, room.IsClosed())
	})
}

func TestManager_GetAllRooms(t *testing.T) {
	log := newTestLogger(t)
	manager := NewManager(log)

	_, err := manager.CreateRoom("room-1")
	require.NoError(t, err)
	_, err = manager.CreateRoom("room-2")
	require.NoError(t, err)
	_, err = manager.CreateRoom("room-3")
	require.NoError(t, err)

	rooms := manager.GetAllRooms()
	assert.Equal(t, 3, len(rooms))
}

func TestManager_RoomCount(t *testing.T) {
	log := newTestLogger(t)
	manager := NewManager(log)

	assert.Equal(t, 0, manager.RoomCount())

	_, err := manager.CreateRoom("room-1")
	require.NoError(t, err)
	assert.Equal(t, 1, manager.RoomCount())

	_, err = manager.CreateRoom("room-2")
	require.NoError(t, err)
	assert.Equal(t, 2, manager.RoomCount())

	err = manager.DeleteRoom("room-1")
	require.NoError(t, err)
	assert.Equal(t, 1, manager.RoomCount())
}

func TestManager_Close(t *testing.T) {
	log := newTestLogger(t)
	manager := NewManager(log)

	room1, err := manager.CreateRoom("room-1")
	require.NoError(t, err)
	room2, err := manager.CreateRoom("room-2")
	require.NoError(t, err)

	err = manager.Close()
	require.NoError(t, err)

	assert.Equal(t, 0, manager.RoomCount())
	assert.True(t, room1.IsClosed())
	assert.True(t, room2.IsClosed())
}

func TestManager_Concurrency(t *testing.T) {
	log := newTestLogger(t)
	manager := NewManager(log)

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()

			roomID := "room-concurrent"
			// Half create, half get
			if idx%2 == 0 {
				manager.CreateRoom(roomID)
			} else {
				manager.GetRoom(roomID)
			}
		}(i)
	}

	wg.Wait()
	// Should not panic or race
	assert.GreaterOrEqual(t, manager.RoomCount(), 0)
}
