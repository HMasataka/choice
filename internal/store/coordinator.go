package store

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"sort"
	"sync"
)

// Coordinator errors.
var (
	ErrNoServers       = errors.New("no servers available")
	ErrServerNotFound  = errors.New("server not found")
	ErrServerExists    = errors.New("server already exists")
	ErrRoomNotAssigned = errors.New("room not assigned to any server")
)

// DefaultVirtualNodes is the default number of virtual nodes per server
// for consistent hashing. Higher values provide better distribution.
const DefaultVirtualNodes = 150

// HashRingCoordinator implements RoomCoordinator using consistent hashing.
// It distributes rooms across SFU servers with minimal remapping when
// servers are added or removed.
type HashRingCoordinator struct {
	roomStore    RoomStore         // for persisting room assignments
	hashToServer map[uint32]string // hash -> serverID
	servers      map[string]bool   // serverID -> exists
	ring         []uint32          // sorted hash values
	mu           sync.RWMutex
	virtualNodes int // number of virtual nodes per server
}

// HashRingOption is a functional option for configuring HashRingCoordinator.
type HashRingOption func(*HashRingCoordinator)

// WithVirtualNodes sets the number of virtual nodes per server.
func WithVirtualNodes(n int) HashRingOption {
	return func(c *HashRingCoordinator) {
		if n > 0 {
			c.virtualNodes = n
		}
	}
}

// WithRoomStore sets the room store for persisting room assignments.
func WithRoomStore(store RoomStore) HashRingOption {
	return func(c *HashRingCoordinator) {
		c.roomStore = store
	}
}

// NewHashRingCoordinator creates a new consistent hash ring coordinator.
func NewHashRingCoordinator(opts ...HashRingOption) *HashRingCoordinator {
	c := &HashRingCoordinator{
		ring:         make([]uint32, 0),
		hashToServer: make(map[uint32]string),
		servers:      make(map[string]bool),
		virtualNodes: DefaultVirtualNodes,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// AssignRoom assigns a room to an SFU server using consistent hashing.
// If the room is already assigned, it returns the existing assignment.
func (c *HashRingCoordinator) AssignRoom(ctx context.Context, roomID string) (string, error) {
	// Check if room is already assigned
	if c.roomStore != nil {
		room, err := c.roomStore.GetRoom(ctx, roomID)
		if err == nil && room.ServerID != "" {
			return room.ServerID, nil
		}
		// Return error if it's not a "not found" error (e.g., Redis/network failure)
		if err != nil && !errors.Is(err, ErrRoomNotFound) {
			return "", fmt.Errorf("failed to check existing room assignment: %w", err)
		}
	}

	// Calculate server using consistent hashing
	serverID, err := c.getServerForKey(roomID)
	if err != nil {
		return "", err
	}

	// Persist the assignment if room store is available
	if c.roomStore != nil {
		room, err := c.roomStore.GetRoom(ctx, roomID)
		if err == nil {
			// Update existing room
			room.ServerID = serverID
			if err := c.roomStore.UpdateRoom(ctx, room); err != nil {
				return "", fmt.Errorf("failed to update room assignment: %w", err)
			}
		} else if !errors.Is(err, ErrRoomNotFound) {
			// Return error if it's not a "not found" error
			return "", fmt.Errorf("failed to get room for assignment update: %w", err)
		}
		// If room doesn't exist (ErrRoomNotFound), the caller is responsible for creating it
	}

	return serverID, nil
}

// GetServerForRoom returns the assigned server for a room.
// It first checks the room store, then falls back to consistent hashing.
func (c *HashRingCoordinator) GetServerForRoom(ctx context.Context, roomID string) (string, error) {
	// Check room store first for explicit assignment
	if c.roomStore != nil {
		room, err := c.roomStore.GetRoom(ctx, roomID)
		if err == nil && room.ServerID != "" {
			return room.ServerID, nil
		}
		if err != nil && !errors.Is(err, ErrRoomNotFound) {
			return "", err
		}
	}

	// Fall back to consistent hashing
	return c.getServerForKey(roomID)
}

// RebalanceRoom moves a room to a different server for failover.
func (c *HashRingCoordinator) RebalanceRoom(ctx context.Context, roomID, newServerID string) error {
	c.mu.RLock()
	if !c.servers[newServerID] {
		c.mu.RUnlock()
		return ErrServerNotFound
	}
	c.mu.RUnlock()

	if c.roomStore == nil {
		return fmt.Errorf("room store not configured")
	}

	room, err := c.roomStore.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}

	room.ServerID = newServerID
	return c.roomStore.UpdateRoom(ctx, room)
}

// AddServer adds a server to the consistent hash ring.
func (c *HashRingCoordinator) AddServer(serverID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.servers[serverID] {
		return ErrServerExists
	}

	c.servers[serverID] = true

	// Add virtual nodes for this server
	for i := range c.virtualNodes {
		hash := c.hash(fmt.Sprintf("%s#%d", serverID, i))
		c.ring = append(c.ring, hash)
		c.hashToServer[hash] = serverID
	}

	// Sort the ring
	sort.Slice(c.ring, func(i, j int) bool {
		return c.ring[i] < c.ring[j]
	})

	return nil
}

// RemoveServer removes a server from the consistent hash ring.
func (c *HashRingCoordinator) RemoveServer(serverID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.servers[serverID] {
		return ErrServerNotFound
	}

	delete(c.servers, serverID)

	// Remove virtual nodes for this server
	newRing := make([]uint32, 0, len(c.ring)-c.virtualNodes)
	for _, hash := range c.ring {
		if c.hashToServer[hash] != serverID {
			newRing = append(newRing, hash)
		} else {
			delete(c.hashToServer, hash)
		}
	}
	c.ring = newRing

	return nil
}

// GetServers returns all servers in the ring.
func (c *HashRingCoordinator) GetServers() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	servers := make([]string, 0, len(c.servers))
	for serverID := range c.servers {
		servers = append(servers, serverID)
	}

	return servers
}

// ServerCount returns the number of servers in the ring.
func (c *HashRingCoordinator) ServerCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.servers)
}

// getServerForKey returns the server for a given key using consistent hashing.
func (c *HashRingCoordinator) getServerForKey(key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.ring) == 0 {
		return "", ErrNoServers
	}

	hash := c.hash(key)

	// Binary search for the first node with hash >= key hash
	idx := sort.Search(len(c.ring), func(i int) bool {
		return c.ring[i] >= hash
	})

	// Wrap around if we're past the end
	if idx >= len(c.ring) {
		idx = 0
	}

	return c.hashToServer[c.ring[idx]], nil
}

// hash computes the hash value for a key.
func (c *HashRingCoordinator) hash(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

// GetDistribution returns the distribution of virtual nodes per server.
// Useful for debugging and monitoring.
func (c *HashRingCoordinator) GetDistribution() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	distribution := make(map[string]int)
	for _, serverID := range c.hashToServer {
		distribution[serverID]++
	}

	return distribution
}
