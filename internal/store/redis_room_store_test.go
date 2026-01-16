package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisRoomStore_SaveRoom(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

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
	assert.Equal(t, room.MaxParticipants, saved.MaxParticipants)
}

func TestRedisRoomStore_SaveRoom_AlreadyExists(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()
	room := &RoomInfo{
		RoomID:    "room-1",
		ServerID:  "server-1",
		State:     RoomStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	// Try to save again
	err = store.SaveRoom(ctx, room)
	assert.ErrorIs(t, err, ErrRoomAlreadyExists)
}

func TestRedisRoomStore_SaveRoom_InvalidRoom(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	// Nil room
	err := store.SaveRoom(ctx, nil)
	assert.ErrorIs(t, err, ErrInvalidRoomInfo)

	// Empty room ID
	err = store.SaveRoom(ctx, &RoomInfo{})
	assert.ErrorIs(t, err, ErrInvalidRoomInfo)
}

func TestRedisRoomStore_GetRoom_NotFound(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	_, err := store.GetRoom(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrRoomNotFound)
}

func TestRedisRoomStore_DeleteRoom(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()
	room := &RoomInfo{
		RoomID:    "room-1",
		ServerID:  "server-1",
		State:     RoomStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	err = store.DeleteRoom(ctx, "room-1")
	require.NoError(t, err)

	// Verify room is deleted
	exists, err := store.RoomExists(ctx, "room-1")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRedisRoomStore_DeleteRoom_NotFound(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	err := store.DeleteRoom(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrRoomNotFound)
}

func TestRedisRoomStore_UpdateRoom(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()
	room := &RoomInfo{
		RoomID:           "room-1",
		ServerID:         "server-1",
		State:            RoomStateCreated,
		ParticipantCount: 0,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
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

func TestRedisRoomStore_UpdateRoom_InvalidRoom(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	err := store.UpdateRoom(ctx, nil)
	assert.ErrorIs(t, err, ErrInvalidRoomInfo)

	err = store.UpdateRoom(ctx, &RoomInfo{})
	assert.ErrorIs(t, err, ErrInvalidRoomInfo)
}

func TestRedisRoomStore_UpdateRoom_NotFound(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	room := &RoomInfo{
		RoomID:   "nonexistent",
		ServerID: "server-1",
		State:    RoomStateActive,
	}

	err := store.UpdateRoom(ctx, room)
	assert.ErrorIs(t, err, ErrRoomNotFound)
}

func TestRedisRoomStore_UpdateRoom_ServerChange(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()
	room := &RoomInfo{
		RoomID:    "room-1",
		ServerID:  "server-1",
		State:     RoomStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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

func TestRedisRoomStore_ListRooms(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		room := &RoomInfo{
			RoomID:    "room-" + string(rune('0'+i)),
			ServerID:  "server-1",
			State:     RoomStateCreated,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := store.SaveRoom(ctx, room)
		require.NoError(t, err)
	}

	rooms, err := store.ListRooms(ctx)
	require.NoError(t, err)
	assert.Len(t, rooms, 3)
}

func TestRedisRoomStore_ListRoomsByServer(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	// Create rooms on different servers
	for i := 1; i <= 3; i++ {
		room := &RoomInfo{
			RoomID:    "room-" + string(rune('0'+i)),
			ServerID:  "server-1",
			State:     RoomStateCreated,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := store.SaveRoom(ctx, room)
		require.NoError(t, err)
	}

	for i := 4; i <= 5; i++ {
		room := &RoomInfo{
			RoomID:    "room-" + string(rune('0'+i)),
			ServerID:  "server-2",
			State:     RoomStateCreated,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
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

func TestRedisRoomStore_RoomExists(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	exists, err := store.RoomExists(ctx, "room-1")
	require.NoError(t, err)
	assert.False(t, exists)

	room := &RoomInfo{
		RoomID:    "room-1",
		ServerID:  "server-1",
		State:     RoomStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = store.SaveRoom(ctx, room)
	require.NoError(t, err)

	exists, err = store.RoomExists(ctx, "room-1")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRedisRoomStore_UpdateParticipantCount(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()
	room := &RoomInfo{
		RoomID:           "room-1",
		ServerID:         "server-1",
		State:            RoomStateCreated,
		ParticipantCount: 0,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
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

func TestRedisRoomStore_UpdateParticipantCount_NotFound(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	err := store.UpdateParticipantCount(ctx, "nonexistent", 1)
	assert.ErrorIs(t, err, ErrRoomNotFound)
}

func TestRedisRoomStore_UpdateTrackCount(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()
	room := &RoomInfo{
		RoomID:     "room-1",
		ServerID:   "server-1",
		State:      RoomStateCreated,
		TrackCount: 0,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	err = store.UpdateTrackCount(ctx, "room-1", 10)
	require.NoError(t, err)

	updated, err := store.GetRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, 10, updated.TrackCount)
}

func TestRedisRoomStore_UpdateRoomState(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()
	room := &RoomInfo{
		RoomID:    "room-1",
		ServerID:  "server-1",
		State:     RoomStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	err = store.UpdateRoomState(ctx, "room-1", RoomStateActive)
	require.NoError(t, err)

	updated, err := store.GetRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, RoomStateActive, updated.State)
}

func TestRedisRoomStore_UpdateRoomState_NotFound(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	err := store.UpdateRoomState(ctx, "nonexistent", RoomStateActive)
	assert.ErrorIs(t, err, ErrRoomNotFound)
}

func TestRedisRoomStore_ParseRoomInfo_InvalidData(t *testing.T) {
	store := NewRedisRoomStore(nil, "test:room:")

	// Missing room_id
	_, err := store.parseRoomInfo(map[string]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "room_id")

	// Invalid participant_count
	_, err = store.parseRoomInfo(map[string]string{
		"room_id":           "room-1",
		"participant_count": "invalid",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "participant_count")

	// Invalid metadata JSON
	_, err = store.parseRoomInfo(map[string]string{
		"room_id":  "room-1",
		"metadata": "{invalid json}",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metadata")

	// Invalid created_at
	_, err = store.parseRoomInfo(map[string]string{
		"room_id":    "room-1",
		"created_at": "invalid-time",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "created_at")
}

func TestRedisRoomStore_ServerIndexMaintenance(t *testing.T) {
	client := newMockRedisClient()
	store := NewRedisRoomStore(client, "test:room:")

	ctx := context.Background()

	// Create a room
	room := &RoomInfo{
		RoomID:    "room-1",
		ServerID:  "server-1",
		State:     RoomStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := store.SaveRoom(ctx, room)
	require.NoError(t, err)

	// Verify server index
	rooms, err := store.ListRoomsByServer(ctx, "server-1")
	require.NoError(t, err)
	assert.Len(t, rooms, 1)

	// Delete room
	err = store.DeleteRoom(ctx, "room-1")
	require.NoError(t, err)

	// Verify server index is cleaned up
	rooms, err = store.ListRoomsByServer(ctx, "server-1")
	require.NoError(t, err)
	assert.Len(t, rooms, 0)
}
