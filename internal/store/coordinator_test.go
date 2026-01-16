package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashRingCoordinator_AddServer(t *testing.T) {
	coordinator := NewHashRingCoordinator()

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)

	servers := coordinator.GetServers()
	assert.Len(t, servers, 1)
	assert.Contains(t, servers, "server-1")
}

func TestHashRingCoordinator_AddServer_AlreadyExists(t *testing.T) {
	coordinator := NewHashRingCoordinator()

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)

	err = coordinator.AddServer("server-1")
	assert.ErrorIs(t, err, ErrServerExists)
}

func TestHashRingCoordinator_RemoveServer(t *testing.T) {
	coordinator := NewHashRingCoordinator()

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)

	err = coordinator.RemoveServer("server-1")
	require.NoError(t, err)

	servers := coordinator.GetServers()
	assert.Len(t, servers, 0)
}

func TestHashRingCoordinator_RemoveServer_NotFound(t *testing.T) {
	coordinator := NewHashRingCoordinator()

	err := coordinator.RemoveServer("nonexistent")
	assert.ErrorIs(t, err, ErrServerNotFound)
}

func TestHashRingCoordinator_AssignRoom_NoServers(t *testing.T) {
	coordinator := NewHashRingCoordinator()
	ctx := context.Background()

	_, err := coordinator.AssignRoom(ctx, "room-1")
	assert.ErrorIs(t, err, ErrNoServers)
}

func TestHashRingCoordinator_AssignRoom(t *testing.T) {
	coordinator := NewHashRingCoordinator()
	ctx := context.Background()

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)
	err = coordinator.AddServer("server-2")
	require.NoError(t, err)

	serverID, err := coordinator.AssignRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.NotEmpty(t, serverID)

	// Same room should always get the same server
	serverID2, err := coordinator.AssignRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, serverID, serverID2)
}

func TestHashRingCoordinator_GetServerForRoom(t *testing.T) {
	coordinator := NewHashRingCoordinator()
	ctx := context.Background()

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)

	serverID, err := coordinator.GetServerForRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, "server-1", serverID)
}

func TestHashRingCoordinator_ConsistentHashing(t *testing.T) {
	coordinator := NewHashRingCoordinator()
	ctx := context.Background()

	// Add initial servers
	for i := 1; i <= 3; i++ {
		err := coordinator.AddServer(fmt.Sprintf("server-%d", i))
		require.NoError(t, err)
	}

	// Record initial assignments
	initialAssignments := make(map[string]string)
	for i := 1; i <= 100; i++ {
		roomID := fmt.Sprintf("room-%d", i)
		serverID, err := coordinator.GetServerForRoom(ctx, roomID)
		require.NoError(t, err)
		initialAssignments[roomID] = serverID
	}

	// Add a new server
	err := coordinator.AddServer("server-4")
	require.NoError(t, err)

	// Count how many rooms changed assignment
	changed := 0
	for roomID, originalServer := range initialAssignments {
		newServer, err := coordinator.GetServerForRoom(ctx, roomID)
		require.NoError(t, err)
		if newServer != originalServer {
			changed++
		}
	}

	// With consistent hashing, only ~25% of rooms should move (1/4 of keys)
	// Allow some tolerance
	assert.Less(t, changed, 50, "Too many rooms changed assignment")
}

func TestHashRingCoordinator_Distribution(t *testing.T) {
	coordinator := NewHashRingCoordinator(WithVirtualNodes(150))
	ctx := context.Background()

	// Add servers
	servers := []string{"server-1", "server-2", "server-3"}
	for _, s := range servers {
		err := coordinator.AddServer(s)
		require.NoError(t, err)
	}

	// Assign many rooms and count distribution
	distribution := make(map[string]int)
	numRooms := 1000

	for i := 0; i < numRooms; i++ {
		roomID := fmt.Sprintf("room-%d", i)
		serverID, err := coordinator.GetServerForRoom(ctx, roomID)
		require.NoError(t, err)
		distribution[serverID]++
	}

	// Each server should get roughly 1/3 of rooms
	// Allow ±15% tolerance
	expectedPerServer := numRooms / len(servers)
	tolerance := int(float64(expectedPerServer) * 0.15)

	for _, server := range servers {
		count := distribution[server]
		assert.Greater(t, count, expectedPerServer-tolerance,
			"Server %s got too few rooms: %d", server, count)
		assert.Less(t, count, expectedPerServer+tolerance,
			"Server %s got too many rooms: %d", server, count)
	}
}

func TestHashRingCoordinator_WithRoomStore(t *testing.T) {
	roomStore := NewMemoryRoomStore()
	coordinator := NewHashRingCoordinator(WithRoomStore(roomStore))
	ctx := context.Background()

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)
	err = coordinator.AddServer("server-2")
	require.NoError(t, err)

	// Create a room in the store
	room := &RoomInfo{
		RoomID:    "room-1",
		ServerID:  "server-1",
		State:     RoomStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = roomStore.SaveRoom(ctx, room)
	require.NoError(t, err)

	// AssignRoom should return existing assignment
	serverID, err := coordinator.AssignRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, "server-1", serverID)

	// GetServerForRoom should also return existing assignment
	serverID, err = coordinator.GetServerForRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, "server-1", serverID)
}

func TestHashRingCoordinator_RebalanceRoom(t *testing.T) {
	roomStore := NewMemoryRoomStore()
	coordinator := NewHashRingCoordinator(WithRoomStore(roomStore))
	ctx := context.Background()

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)
	err = coordinator.AddServer("server-2")
	require.NoError(t, err)

	// Create a room
	room := &RoomInfo{
		RoomID:    "room-1",
		ServerID:  "server-1",
		State:     RoomStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = roomStore.SaveRoom(ctx, room)
	require.NoError(t, err)

	// Rebalance to server-2
	err = coordinator.RebalanceRoom(ctx, "room-1", "server-2")
	require.NoError(t, err)

	// Verify the room was moved
	serverID, err := coordinator.GetServerForRoom(ctx, "room-1")
	require.NoError(t, err)
	assert.Equal(t, "server-2", serverID)
}

func TestHashRingCoordinator_RebalanceRoom_ServerNotFound(t *testing.T) {
	roomStore := NewMemoryRoomStore()
	coordinator := NewHashRingCoordinator(WithRoomStore(roomStore))
	ctx := context.Background()

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)

	room := &RoomInfo{
		RoomID:    "room-1",
		ServerID:  "server-1",
		State:     RoomStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = roomStore.SaveRoom(ctx, room)
	require.NoError(t, err)

	err = coordinator.RebalanceRoom(ctx, "room-1", "nonexistent")
	assert.ErrorIs(t, err, ErrServerNotFound)
}

func TestHashRingCoordinator_GetDistribution(t *testing.T) {
	coordinator := NewHashRingCoordinator(WithVirtualNodes(10))

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)
	err = coordinator.AddServer("server-2")
	require.NoError(t, err)

	distribution := coordinator.GetDistribution()
	assert.Len(t, distribution, 2)
	assert.Equal(t, 10, distribution["server-1"])
	assert.Equal(t, 10, distribution["server-2"])
}

func TestHashRingCoordinator_ServerCount(t *testing.T) {
	coordinator := NewHashRingCoordinator()

	assert.Equal(t, 0, coordinator.ServerCount())

	err := coordinator.AddServer("server-1")
	require.NoError(t, err)
	assert.Equal(t, 1, coordinator.ServerCount())

	err = coordinator.AddServer("server-2")
	require.NoError(t, err)
	assert.Equal(t, 2, coordinator.ServerCount())

	err = coordinator.RemoveServer("server-1")
	require.NoError(t, err)
	assert.Equal(t, 1, coordinator.ServerCount())
}

func TestHashRingCoordinator_ConcurrentAccess(t *testing.T) {
	coordinator := NewHashRingCoordinator()
	ctx := context.Background()

	// Add initial server
	err := coordinator.AddServer("server-1")
	require.NoError(t, err)

	done := make(chan bool)

	// Concurrent server additions
	go func() {
		for i := 2; i <= 10; i++ {
			_ = coordinator.AddServer(fmt.Sprintf("server-%d", i))
		}
		done <- true
	}()

	// Concurrent room assignments
	go func() {
		for i := 0; i < 100; i++ {
			_, _ = coordinator.GetServerForRoom(ctx, fmt.Sprintf("room-%d", i))
		}
		done <- true
	}()

	<-done
	<-done

	// Verify state is consistent
	servers := coordinator.GetServers()
	assert.Greater(t, len(servers), 0)
}
