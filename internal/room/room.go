package room

import (
	"log"
	"sync"
	"time"
)

// Room represents a room containing multiple clients
type Room struct {
	ID          string
	Name        string
	Description string
	MaxClients  int // 0 means unlimited
	Clients     map[string]*Client
	Created     time.Time
	mu          sync.RWMutex
}

// NewRoom creates a new room
func NewRoom(id, name, description string, maxClients int) *Room {
	return &Room{
		ID:          id,
		Name:        name,
		Description: description,
		MaxClients:  maxClients,
		Clients:     make(map[string]*Client),
		Created:     time.Now(),
	}
}

// AddClient adds a client to the room
func (r *Room) AddClient(client *Client) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if room is full
	if r.MaxClients > 0 && len(r.Clients) >= r.MaxClients {
		return ErrRoomIsFull
	}

	// Check if client already exists
	if _, exists := r.Clients[client.ID]; exists {
		return ErrClientAlreadyExists
	}

	r.Clients[client.ID] = client
	client.RoomID = r.ID

	log.Printf("Client %s (%s) joined room %s. Total clients: %d",
		client.ID, client.Name, r.ID, len(r.Clients))

	return nil
}

// RemoveClient removes a client from the room
func (r *Room) RemoveClient(clientID string) (*Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, exists := r.Clients[clientID]
	if !exists {
		return nil, ErrClientNotFound
	}

	delete(r.Clients, clientID)

	log.Printf("Client %s (%s) left room %s. Remaining clients: %d",
		client.ID, client.Name, r.ID, len(r.Clients))

	return client, nil
}

// GetClient gets a client by ID
func (r *Room) GetClient(clientID string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, exists := r.Clients[clientID]
	return client, exists
}

// GetClients returns a copy of all clients in the room
func (r *Room) GetClients() map[string]*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make(map[string]*Client)
	for k, v := range r.Clients {
		clients[k] = v
	}
	return clients
}

// GetClientCount returns the number of clients in the room
func (r *Room) GetClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Clients)
}

// IsEmpty checks if the room is empty
func (r *Room) IsEmpty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Clients) == 0
}

// IsFull checks if the room is full
func (r *Room) IsFull() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.MaxClients > 0 && len(r.Clients) >= r.MaxClients
}

// BroadcastMessage sends a message to all clients in the room via WebSocket
func (r *Room) BroadcastMessage(data []byte) error {
	r.mu.RLock()
	clients := make([]*Client, 0, len(r.Clients))
	for _, client := range r.Clients {
		clients = append(clients, client)
	}
	r.mu.RUnlock()

	var lastErr error
	for _, client := range clients {
		if err := client.SendMessage(data); err != nil {
			log.Printf("Failed to send message to client %s: %v", client.ID, err)
			lastErr = err
		}
	}

	return lastErr
}

// BroadcastToOthers sends a message to all clients except the sender
func (r *Room) BroadcastToOthers(senderID string, data []byte) error {
	r.mu.RLock()
	clients := make([]*Client, 0, len(r.Clients))
	for _, client := range r.Clients {
		if client.ID != senderID {
			clients = append(clients, client)
		}
	}
	r.mu.RUnlock()

	var lastErr error
	for _, client := range clients {
		if err := client.SendMessage(data); err != nil {
			log.Printf("Failed to send message to client %s: %v", client.ID, err)
			lastErr = err
		}
	}

	return lastErr
}

// BroadcastDataChannelMessage sends a message to all clients via DataChannel
func (r *Room) BroadcastDataChannelMessage(data []byte) error {
	r.mu.RLock()
	clients := make([]*Client, 0, len(r.Clients))
	for _, client := range r.Clients {
		clients = append(clients, client)
	}
	r.mu.RUnlock()

	var lastErr error
	for _, client := range clients {
		if err := client.SendDataChannelMessage(data); err != nil {
			log.Printf("Failed to send data channel message to client %s: %v", client.ID, err)
			lastErr = err
		}
	}

	return lastErr
}

// BroadcastDataChannelToOthers sends a DataChannel message to all clients except the sender
func (r *Room) BroadcastDataChannelToOthers(senderID string, data []byte) error {
	r.mu.RLock()
	clients := make([]*Client, 0, len(r.Clients))
	for _, client := range r.Clients {
		if client.ID != senderID {
			clients = append(clients, client)
		}
	}
	r.mu.RUnlock()

	var lastErr error
	for _, client := range clients {
		if err := client.SendDataChannelMessage(data); err != nil {
			log.Printf("Failed to send data channel message to client %s: %v", client.ID, err)
			lastErr = err
		}
	}

	return lastErr
}

// GetClientInfoList returns information about all clients in the room
func (r *Room) GetClientInfoList() []ClientInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]ClientInfo, 0, len(r.Clients))
	for _, client := range r.Clients {
		infos = append(infos, client.GetInfo())
	}

	return infos
}

// RemoveInactiveClients removes clients that haven't pinged recently
func (r *Room) RemoveInactiveClients() []*Client {
	r.mu.Lock()
	defer r.mu.Unlock()

	inactiveClients := make([]*Client, 0)
	for clientID, client := range r.Clients {
		if !client.IsActive() {
			inactiveClients = append(inactiveClients, client)
			delete(r.Clients, clientID)
			log.Printf("Removed inactive client %s from room %s", clientID, r.ID)
		}
	}

	return inactiveClients
}

// RoomInfo represents basic room information
type RoomInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MaxClients  int       `json:"max_clients"`
	ClientCount int       `json:"client_count"`
	Created     time.Time `json:"created"`
	IsFull      bool      `json:"is_full"`
}

// GetInfo returns room information
func (r *Room) GetInfo() RoomInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return RoomInfo{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		MaxClients:  r.MaxClients,
		ClientCount: len(r.Clients),
		Created:     r.Created,
		IsFull:      r.MaxClients > 0 && len(r.Clients) >= r.MaxClients,
	}
}
