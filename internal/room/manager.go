package room

import (
	"sync"

	"github.com/HMasataka/choice/pkg/logger"
)

// Manager manages rooms.
type Manager struct {
	mu     sync.RWMutex
	rooms  map[string]*Room
	logger *logger.Logger
}

// NewManager creates a new room manager.
func NewManager(log *logger.Logger) *Manager {
	return &Manager{
		rooms:  make(map[string]*Room),
		logger: log,
	}
}

// CreateRoom creates a new room with the given ID and options.
func (m *Manager) CreateRoom(id string, opts ...RoomOption) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.rooms[id]; exists {
		return nil, ErrRoomAlreadyExists
	}

	room := NewRoom(id, opts...)
	m.rooms[id] = room

	m.logger.Info("room created", "room_id", id)

	return room, nil
}

// GetRoom returns a room by ID.
func (m *Manager) GetRoom(id string) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	room, exists := m.rooms[id]
	if !exists {
		return nil, ErrRoomNotFound
	}

	return room, nil
}

// DeleteRoom deletes a room by ID.
func (m *Manager) DeleteRoom(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[id]
	if !exists {
		return ErrRoomNotFound
	}

	// Close the room first
	if err := room.Close(); err != nil {
		m.logger.Error("failed to close room", "room_id", id, "error", err)
	}

	delete(m.rooms, id)

	m.logger.Info("room deleted", "room_id", id)

	return nil
}

// GetAllRooms returns all rooms.
func (m *Manager) GetAllRooms() []*Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}

	return rooms
}

// RoomCount returns the number of rooms.
func (m *Manager) RoomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.rooms)
}

// Close closes all rooms and cleans up resources.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, room := range m.rooms {
		if err := room.Close(); err != nil {
			m.logger.Error("failed to close room", "room_id", id, "error", err)
		}
	}

	m.rooms = make(map[string]*Room)

	return nil
}

var (
	// ErrRoomAlreadyExists is returned when attempting to create a room with an ID that already exists.
	ErrRoomAlreadyExists = ErrRoomNotFound // Placeholder, will be properly defined
)

func init() {
	ErrRoomAlreadyExists = &roomError{msg: "room already exists"}
}

type roomError struct {
	msg string
}

func (e *roomError) Error() string {
	return e.msg
}
