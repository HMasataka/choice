package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryRoomStore_SaveRoom(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:           "room-1",
		ServerID:         "server-1",
		State:            RoomStateCreated,
		ParticipantCount: 0,
		MaxParticipants:  100,
		TrackCount:       0,
		Metadata:         map[string]interface{}{"name": "Test Room"},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	// Verify room was saved
	saved, err := store.GetRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, room.RoomID, saved.RoomID)
	assert.Equal(t, room.ServerID, saved.ServerID)
	assert.Equal(t, room.State, saved.State)
}

func TestMemoryRoomStore_SaveRoom_AlreadyExists(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:   "room-1",
		ServerID: "server-1",
		State:    RoomStateCreated,
	}

	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	// Try to save again
	err = store.SaveRoom(ctx, room)
	assert.ErrorIs(t, err, ErrRoomAlreadyExists)
}

func TestMemoryRoomStore_SaveRoom_NilRoom(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	err := store.SaveRoom(ctx, nil)
	assert.ErrorIs(t, err, ErrInvalidRoomInfo)
}

func TestMemoryRoomStore_GetRoom_NotFound(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	_, err := store.GetRoom(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrRoomNotFound)
}

func TestMemoryRoomStore_DeleteRoom(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:   "room-1",
		ServerID: "server-1",
		State:    RoomStateCreated,
	}

	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	err = store.DeleteRoom(ctx, "room-1")
	require.NoError(t, err)

	_, err = store.GetRoom(ctx, "room-1")
	assert.ErrorIs(t, err, ErrRoomNotFound)
}

func TestMemoryRoomStore_DeleteRoom_NotFound(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	err := store.DeleteRoom(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrRoomNotFound)
}

func TestMemoryRoomStore_UpdateRoom(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:           "room-1",
		ServerID:         "server-1",
		State:            RoomStateCreated,
		ParticipantCount: 0,
	}

	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	// Update room
	room.State = RoomStateActive
	room.ParticipantCount = 5
	err = store.UpdateRoom(ctx, room)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, RoomStateActive, updated.State)
	assert.Equal(t, 5, updated.ParticipantCount)
}

func TestMemoryRoomStore_UpdateRoom_ServerChange(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:   "room-1",
		ServerID: "server-1",
		State:    RoomStateCreated,
	}

	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	// Update server
	room.ServerID = "server-2"
	err = store.UpdateRoom(ctx, room)
	require.NoError(t, err)

	// Verify server index updated
	rooms1, err := store.ListRoomsByServer(ctx, "server-1")
	require.NoError(t, err)
	assert.Len(t, rooms1, 0)

	rooms2, err := store.ListRoomsByServer(ctx, "server-2")
	require.NoError(t, err)
	assert.Len(t, rooms2, 1)
}

func TestMemoryRoomStore_ListRooms(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		room := &RoomInfo{
			RoomID:   "room-" + string(rune('0'+i)),
			ServerID: "server-1",
			State:    RoomStateCreated,
		}
		err := store.SaveRoom(ctx, room)
		require.NoError(t, err)
	}

	rooms, err := store.ListRooms(ctx)
	require.NoError(t, err)
	assert.Len(t, rooms, 3)
}

func TestMemoryRoomStore_ListRoomsByServer(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	// Create rooms on different servers
	for i := 1; i <= 3; i++ {
		room := &RoomInfo{
			RoomID:   "room-" + string(rune('0'+i)),
			ServerID: "server-1",
			State:    RoomStateCreated,
		}
		err := store.SaveRoom(ctx, room)
		require.NoError(t, err)
	}

	for i := 4; i <= 5; i++ {
		room := &RoomInfo{
			RoomID:   "room-" + string(rune('0'+i)),
			ServerID: "server-2",
			State:    RoomStateCreated,
		}
		err := store.SaveRoom(ctx, room)
		require.NoError(t, err)
	}

	rooms1, err := store.ListRoomsByServer(ctx, "server-1")
	require.NoError(t, err)
	assert.Len(t, rooms1, 3)

	rooms2, err := store.ListRoomsByServer(ctx, "server-2")
	require.NoError(t, err)
	assert.Len(t, rooms2, 2)
}

func TestMemoryRoomStore_RoomExists(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	exists, err := store.RoomExists(ctx, "room-1")
	require.NoError(t, err)
	assert.False(t, exists)

	room := &RoomInfo{
		RoomID:   "room-1",
		ServerID: "server-1",
		State:    RoomStateCreated,
	}
	err = store.SaveRoom(ctx, room)
	require.NoError(t, err)

	exists, err = store.RoomExists(ctx, "room-1")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestMemoryRoomStore_UpdateParticipantCount(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:           "room-1",
		ServerID:         "server-1",
		State:            RoomStateCreated,
		ParticipantCount: 0,
	}
	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	// Increment
	err = store.UpdateParticipantCount(ctx, "room-1", 5)
	require.NoError(t, err)

	updated, err := store.GetRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, 5, updated.ParticipantCount)

	// Decrement
	err = store.UpdateParticipantCount(ctx, "room-1", -2)
	require.NoError(t, err)

	updated, err = store.GetRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, 3, updated.ParticipantCount)
}

func TestMemoryRoomStore_UpdateTrackCount(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:     "room-1",
		ServerID:   "server-1",
		State:      RoomStateCreated,
		TrackCount: 0,
	}
	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	err = store.UpdateTrackCount(ctx, "room-1", 10)
	require.NoError(t, err)

	updated, err := store.GetRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, 10, updated.TrackCount)
}

func TestMemoryRoomStore_UpdateRoomState(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:   "room-1",
		ServerID: "server-1",
		State:    RoomStateCreated,
	}
	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	err = store.UpdateRoomState(ctx, "room-1", RoomStateActive)
	require.NoError(t, err)

	updated, err := store.GetRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, RoomStateActive, updated.State)
}

func TestMemoryRoomStore_Close(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:   "room-1",
		ServerID: "server-1",
		State:    RoomStateCreated,
	}
	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	err = store.Close()
	require.NoError(t, err)

	rooms, err := store.ListRooms(ctx)
	require.NoError(t, err)
	assert.Len(t, rooms, 0)
}

func TestMemoryRoomStore_ConcurrentAccess(t *testing.T) {
	store := NewMemoryRoomStore()
	ctx := context.Background()

	room := &RoomInfo{
		RoomID:           "room-1",
		ServerID:         "server-1",
		State:            RoomStateCreated,
		ParticipantCount: 0,
	}
	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	// Concurrent updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_ = store.UpdateParticipantCount(ctx, "room-1", 1)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	updated, err := store.GetRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, 10, updated.ParticipantCount)
}
