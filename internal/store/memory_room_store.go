package store

import (
	"context"
	"sync"
	"time"
)

// MemoryRoomStore is an in-memory implementation of RoomStore.
// Useful for testing and single-instance deployments.
type MemoryRoomStore struct {
	rooms       map[string]*RoomInfo
	serverIndex map[string]map[string]struct{} // serverID -> set of roomIDs
	mu          sync.RWMutex
}

// NewMemoryRoomStore creates a new in-memory room store.
func NewMemoryRoomStore() *MemoryRoomStore {
	return &MemoryRoomStore{
		rooms:       make(map[string]*RoomInfo),
		serverIndex: make(map[string]map[string]struct{}),
	}
}

// SaveRoom saves room information to memory.
func (s *MemoryRoomStore) SaveRoom(_ context.Context, room *RoomInfo) error {
	if room == nil || room.RoomID == "" {
		return ErrInvalidRoomInfo
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rooms[room.RoomID]; exists {
		return ErrRoomAlreadyExists
	}

	// Deep copy room info
	roomCopy := s.copyRoomInfo(room)
	s.rooms[room.RoomID] = roomCopy

	// Update server index
	if room.ServerID != "" {
		if s.serverIndex[room.ServerID] == nil {
			s.serverIndex[room.ServerID] = make(map[string]struct{})
		}
		s.serverIndex[room.ServerID][room.RoomID] = struct{}{}
	}

	return nil
}

// GetRoom retrieves room information by room ID.
func (s *MemoryRoomStore) GetRoom(_ context.Context, roomID string) (*RoomInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.rooms[roomID]
	if !exists {
		return nil, ErrRoomNotFound
	}

	return s.copyRoomInfo(room), nil
}

// DeleteRoom deletes room information from memory.
func (s *MemoryRoomStore) DeleteRoom(_ context.Context, roomID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[roomID]
	if !exists {
		return ErrRoomNotFound
	}

	// Remove from server index
	if room.ServerID != "" {
		if serverRooms, ok := s.serverIndex[room.ServerID]; ok {
			delete(serverRooms, roomID)
			if len(serverRooms) == 0 {
				delete(s.serverIndex, room.ServerID)
			}
		}
	}

	delete(s.rooms, roomID)
	return nil
}

// UpdateRoom updates existing room information in memory.
func (s *MemoryRoomStore) UpdateRoom(_ context.Context, room *RoomInfo) error {
	if room == nil || room.RoomID == "" {
		return ErrInvalidRoomInfo
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.rooms[room.RoomID]
	if !exists {
		return ErrRoomNotFound
	}

	// Update server index if server changed
	if existing.ServerID != room.ServerID {
		// Remove from old server index
		if existing.ServerID != "" {
			if serverRooms, ok := s.serverIndex[existing.ServerID]; ok {
				delete(serverRooms, room.RoomID)
				if len(serverRooms) == 0 {
					delete(s.serverIndex, existing.ServerID)
				}
			}
		}

		// Add to new server index
		if room.ServerID != "" {
			if s.serverIndex[room.ServerID] == nil {
				s.serverIndex[room.ServerID] = make(map[string]struct{})
			}
			s.serverIndex[room.ServerID][room.RoomID] = struct{}{}
		}
	}

	// Deep copy and update
	roomCopy := s.copyRoomInfo(room)
	roomCopy.UpdatedAt = time.Now()
	s.rooms[room.RoomID] = roomCopy

	return nil
}

// ListRooms returns all rooms in the store.
func (s *MemoryRoomStore) ListRooms(_ context.Context) ([]*RoomInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rooms := make([]*RoomInfo, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, s.copyRoomInfo(room))
	}

	return rooms, nil
}

// ListRoomsByServer returns all rooms assigned to a specific server.
func (s *MemoryRoomStore) ListRoomsByServer(_ context.Context, serverID string) ([]*RoomInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	roomIDs, ok := s.serverIndex[serverID]
	if !ok {
		return []*RoomInfo{}, nil
	}

	rooms := make([]*RoomInfo, 0, len(roomIDs))
	for roomID := range roomIDs {
		if room, exists := s.rooms[roomID]; exists {
			rooms = append(rooms, s.copyRoomInfo(room))
		}
	}

	return rooms, nil
}

// RoomExists checks if a room exists in the store.
func (s *MemoryRoomStore) RoomExists(_ context.Context, roomID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.rooms[roomID]
	return exists, nil
}

// UpdateParticipantCount atomically updates the participant count for a room.
func (s *MemoryRoomStore) UpdateParticipantCount(_ context.Context, roomID string, delta int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[roomID]
	if !exists {
		return ErrRoomNotFound
	}

	room.ParticipantCount += delta
	room.UpdatedAt = time.Now()

	return nil
}

// UpdateTrackCount atomically updates the track count for a room.
func (s *MemoryRoomStore) UpdateTrackCount(_ context.Context, roomID string, delta int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[roomID]
	if !exists {
		return ErrRoomNotFound
	}

	room.TrackCount += delta
	room.UpdatedAt = time.Now()

	return nil
}

// UpdateRoomState updates the state of a room.
func (s *MemoryRoomStore) UpdateRoomState(_ context.Context, roomID string, state RoomState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[roomID]
	if !exists {
		return ErrRoomNotFound
	}

	room.State = state
	room.UpdatedAt = time.Now()

	return nil
}

// Close cleans up resources.
func (s *MemoryRoomStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rooms = make(map[string]*RoomInfo)
	s.serverIndex = make(map[string]map[string]struct{})

	return nil
}

// copyRoomInfo creates a deep copy of RoomInfo.
func (s *MemoryRoomStore) copyRoomInfo(room *RoomInfo) *RoomInfo {
	if room == nil {
		return nil
	}

	roomCopy := &RoomInfo{
		RoomID:           room.RoomID,
		ServerID:         room.ServerID,
		State:            room.State,
		ParticipantCount: room.ParticipantCount,
		MaxParticipants:  room.MaxParticipants,
		TrackCount:       room.TrackCount,
		CreatedAt:        room.CreatedAt,
		UpdatedAt:        room.UpdatedAt,
	}

	if room.Metadata != nil {
		roomCopy.Metadata = make(map[string]interface{}, len(room.Metadata))
		for k, v := range room.Metadata {
			roomCopy.Metadata[k] = v
		}
	}

	return roomCopy
}
