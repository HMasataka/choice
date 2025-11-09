package room

import (
	"log"
	"strings"
	"sync"
	"time"
)

// RoomManager manages all rooms and clients
type RoomManager struct {
	Rooms           map[string]*Room
	Clients         map[string]*Client
	DefaultMaxUsers int
	mu              sync.RWMutex
	cleanupTicker   *time.Ticker
	stopCleanup     chan struct{}
}

// NewRoomManager creates a new room manager
func NewRoomManager(defaultMaxUsers int) *RoomManager {
	rm := &RoomManager{
		Rooms:           make(map[string]*Room),
		Clients:         make(map[string]*Client),
		DefaultMaxUsers: defaultMaxUsers,
		cleanupTicker:   time.NewTicker(30 * time.Second), // Cleanup every 30 seconds
		stopCleanup:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go rm.cleanupRoutine()

	return rm
}

// CreateRoom creates a new room
func (rm *RoomManager) CreateRoom(roomID, name, description string, maxClients int) (*Room, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Validate room ID
	if err := rm.validateRoomID(roomID); err != nil {
		return nil, err
	}

	// Check if room already exists
	if _, exists := rm.Rooms[roomID]; exists {
		return nil, ErrRoomAlreadyExists
	}

	// Use default max clients if not specified
	if maxClients <= 0 {
		maxClients = rm.DefaultMaxUsers
	}

	room := NewRoom(roomID, name, description, maxClients)
	rm.Rooms[roomID] = room

	log.Printf("Created room %s (%s) with max clients: %d", roomID, name, maxClients)
	return room, nil
}

// GetOrCreateRoom gets an existing room or creates a new one
func (rm *RoomManager) GetOrCreateRoom(roomID string) (*Room, error) {
	rm.mu.RLock()
	room, exists := rm.Rooms[roomID]
	rm.mu.RUnlock()

	if exists {
		return room, nil
	}

	// Create new room with default settings
	return rm.CreateRoom(roomID, roomID, "Auto-created room", rm.DefaultMaxUsers)
}

// GetRoom gets a room by ID
func (rm *RoomManager) GetRoom(roomID string) (*Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	room, exists := rm.Rooms[roomID]
	return room, exists
}

// DeleteRoom deletes a room
func (rm *RoomManager) DeleteRoom(roomID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, exists := rm.Rooms[roomID]
	if !exists {
		return ErrRoomNotFound
	}

	// Remove all clients from the room first
	clients := room.GetClients()
	for _, client := range clients {
		delete(rm.Clients, client.ID)
		if err := client.Close(); err != nil {
			log.Printf("Error closing client %s: %v", client.ID, err)
		}
	}

	delete(rm.Rooms, roomID)
	log.Printf("Deleted room %s", roomID)
	return nil
}

// AddClient adds a client to a room
func (rm *RoomManager) AddClient(roomID string, client *Client) error {
	// Validate client ID
	if err := rm.validateClientID(client.ID); err != nil {
		return err
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if client already exists
	if _, exists := rm.Clients[client.ID]; exists {
		return ErrClientAlreadyExists
	}

	// Get or create room
	room, exists := rm.Rooms[roomID]
	if !exists {
		// Create room automatically
		rm.mu.Unlock()
		var err error
		room, err = rm.GetOrCreateRoom(roomID)
		if err != nil {
			return err
		}
		rm.mu.Lock()
	}

	// Add client to room
	if err := room.AddClient(client); err != nil {
		return err
	}

	// Add client to global client map
	rm.Clients[client.ID] = client

	log.Printf("Added client %s to room %s", client.ID, roomID)
	return nil
}

// RemoveClient removes a client from the system
func (rm *RoomManager) RemoveClient(clientID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	client, exists := rm.Clients[clientID]
	if !exists {
		return ErrClientNotFound
	}

	// Remove from room
	if room, roomExists := rm.Rooms[client.RoomID]; roomExists {
		if _, err := room.RemoveClient(clientID); err != nil {
			log.Printf("Error removing client %s from room %s: %v", clientID, client.RoomID, err)
		}

		// Delete empty room
		if room.IsEmpty() {
			delete(rm.Rooms, client.RoomID)
			log.Printf("Deleted empty room %s", client.RoomID)
		}
	}

	// Remove from global client map
	delete(rm.Clients, clientID)

	// Close client connections
	if err := client.Close(); err != nil {
		log.Printf("Error closing client %s: %v", clientID, err)
	}

	log.Printf("Removed client %s from system", clientID)
	return nil
}

// GetClient gets a client by ID
func (rm *RoomManager) GetClient(clientID string) (*Client, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	client, exists := rm.Clients[clientID]
	return client, exists
}

// BroadcastToRoom broadcasts a message to all clients in a room
func (rm *RoomManager) BroadcastToRoom(roomID string, data []byte) error {
	room, exists := rm.GetRoom(roomID)
	if !exists {
		return ErrRoomNotFound
	}

	return room.BroadcastMessage(data)
}

// BroadcastToOthers broadcasts a message to all clients in a room except the sender
func (rm *RoomManager) BroadcastToOthers(senderID string, data []byte) error {
	client, exists := rm.GetClient(senderID)
	if !exists {
		return ErrClientNotFound
	}

	room, exists := rm.GetRoom(client.RoomID)
	if !exists {
		return ErrRoomNotFound
	}

	return room.BroadcastToOthers(senderID, data)
}

// GetRoomList returns a list of all rooms
func (rm *RoomManager) GetRoomList() []RoomInfo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rooms := make([]RoomInfo, 0, len(rm.Rooms))
	for _, room := range rm.Rooms {
		rooms = append(rooms, room.GetInfo())
	}

	return rooms
}

// GetClientCount returns the total number of connected clients
func (rm *RoomManager) GetClientCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.Clients)
}

// GetRoomCount returns the total number of rooms
func (rm *RoomManager) GetRoomCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.Rooms)
}

// Stop stops the room manager and cleans up resources
func (rm *RoomManager) Stop() {
	close(rm.stopCleanup)
	rm.cleanupTicker.Stop()

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Close all clients
	for _, client := range rm.Clients {
		if err := client.Close(); err != nil {
			log.Printf("Error closing client %s: %v", client.ID, err)
		}
	}

	// Clear maps
	rm.Clients = make(map[string]*Client)
	rm.Rooms = make(map[string]*Room)

	log.Println("Room manager stopped")
}

// cleanupRoutine periodically cleans up inactive clients and empty rooms
func (rm *RoomManager) cleanupRoutine() {
	for {
		select {
		case <-rm.cleanupTicker.C:
			rm.cleanup()
		case <-rm.stopCleanup:
			return
		}
	}
}

// cleanup removes inactive clients and empty rooms
func (rm *RoomManager) cleanup() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Remove inactive clients from each room
	roomsToDelete := make([]string, 0)
	for roomID, room := range rm.Rooms {
		inactiveClients := room.RemoveInactiveClients()

		// Remove inactive clients from global map
		for _, client := range inactiveClients {
			delete(rm.Clients, client.ID)
			if err := client.Close(); err != nil {
				log.Printf("Error closing inactive client %s: %v", client.ID, err)
			}
		}

		// Mark empty rooms for deletion
		if room.IsEmpty() {
			roomsToDelete = append(roomsToDelete, roomID)
		}
	}

	// Delete empty rooms
	for _, roomID := range roomsToDelete {
		delete(rm.Rooms, roomID)
		log.Printf("Cleaned up empty room %s", roomID)
	}

	if len(roomsToDelete) > 0 || rm.hasInactiveClients() {
		log.Printf("Cleanup completed. Rooms: %d, Clients: %d", len(rm.Rooms), len(rm.Clients))
	}
}

// hasInactiveClients checks if there are any inactive clients (for logging)
func (rm *RoomManager) hasInactiveClients() bool {
	for _, client := range rm.Clients {
		if !client.IsActive() {
			return true
		}
	}
	return false
}

// validateRoomID validates room ID format
func (rm *RoomManager) validateRoomID(roomID string) error {
	if roomID == "" {
		return ErrInvalidRoomID
	}

	// Room ID should be alphanumeric with dashes and underscores
	for _, char := range roomID {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_') {
			return ErrInvalidRoomID
		}
	}

	// Length check
	if len(roomID) > 100 {
		return ErrInvalidRoomID
	}

	return nil
}

// validateClientID validates client ID format
func (rm *RoomManager) validateClientID(clientID string) error {
	if clientID == "" {
		return ErrInvalidClientID
	}

	// Client ID should not contain sensitive characters
	if strings.Contains(clientID, " ") || strings.Contains(clientID, "\n") || strings.Contains(clientID, "\t") {
		return ErrInvalidClientID
	}

	// Length check
	if len(clientID) > 100 {
		return ErrInvalidClientID
	}

	return nil
}