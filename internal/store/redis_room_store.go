package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// RedisRoomStore is a Redis-based implementation of RoomStore.
// It uses Redis Hash for storing room information (ADR-0005).
// Unlike session store, rooms do not have TTL and must be explicitly deleted.
type RedisRoomStore struct {
	client RedisClient
	prefix string
}

// NewRedisRoomStore creates a new Redis room store.
func NewRedisRoomStore(client RedisClient, prefix string) *RedisRoomStore {
	if prefix == "" {
		prefix = "room:"
	}

	return &RedisRoomStore{
		client: client,
		prefix: prefix,
	}
}

// SaveRoom saves room information to Redis.
// Rooms do not have TTL and must be explicitly deleted (ADR-0005).
func (s *RedisRoomStore) SaveRoom(ctx context.Context, room *RoomInfo) error {
	if room == nil || room.RoomID == "" {
		return ErrInvalidRoomInfo
	}

	key := s.roomKey(room.RoomID)

	// Check if room already exists
	exists, err := s.client.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check room existence: %w", err)
	}
	if exists > 0 {
		return ErrRoomAlreadyExists
	}

	// Serialize metadata to JSON
	metadataJSON, err := json.Marshal(room.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Store room as hash
	err = s.client.HSet(ctx, key,
		"room_id", room.RoomID,
		"server_id", room.ServerID,
		"state", string(room.State),
		"participant_count", strconv.Itoa(room.ParticipantCount),
		"max_participants", strconv.Itoa(room.MaxParticipants),
		"track_count", strconv.Itoa(room.TrackCount),
		"metadata", string(metadataJSON),
		"created_at", room.CreatedAt.Format(time.RFC3339Nano),
		"updated_at", room.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("failed to save room: %w", err)
	}

	// Add to server index for efficient lookup (skip if ServerID is empty)
	if room.ServerID != "" {
		serverIndexKey := s.serverIndexKey(room.ServerID)
		if err := s.client.SAdd(ctx, serverIndexKey, room.RoomID); err != nil {
			return fmt.Errorf("failed to add to server index: %w", err)
		}
	}

	return nil
}

// GetRoom retrieves room information from Redis by room ID.
func (s *RedisRoomStore) GetRoom(ctx context.Context, roomID string) (*RoomInfo, error) {
	key := s.roomKey(roomID)

	data, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get room: %w", err)
	}

	if len(data) == 0 {
		return nil, ErrRoomNotFound
	}

	return s.parseRoomInfo(data)
}

// DeleteRoom deletes room information from Redis.
func (s *RedisRoomStore) DeleteRoom(ctx context.Context, roomID string) error {
	key := s.roomKey(roomID)

	// Get room to find server ID for index cleanup
	room, err := s.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}

	// Delete room hash
	if err := s.client.Del(ctx, key); err != nil {
		return fmt.Errorf("failed to delete room: %w", err)
	}

	// Remove from server index
	if room.ServerID != "" {
		serverIndexKey := s.serverIndexKey(room.ServerID)
		if err := s.client.SRem(ctx, serverIndexKey, roomID); err != nil {
			return fmt.Errorf("failed to remove from server index: %w", err)
		}
	}

	return nil
}

// UpdateRoom updates existing room information in Redis.
func (s *RedisRoomStore) UpdateRoom(ctx context.Context, room *RoomInfo) error {
	if room == nil || room.RoomID == "" {
		return ErrInvalidRoomInfo
	}

	key := s.roomKey(room.RoomID)

	// Check if room exists
	exists, err := s.client.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check room existence: %w", err)
	}
	if exists == 0 {
		return ErrRoomNotFound
	}

	// Get existing room for server ID comparison
	existingRoom, err := s.GetRoom(ctx, room.RoomID)
	if err != nil {
		return err
	}

	// Update timestamp
	room.UpdatedAt = time.Now()

	// Serialize metadata to JSON
	metadataJSON, err := json.Marshal(room.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Update room hash
	err = s.client.HSet(ctx, key,
		"server_id", room.ServerID,
		"state", string(room.State),
		"participant_count", strconv.Itoa(room.ParticipantCount),
		"max_participants", strconv.Itoa(room.MaxParticipants),
		"track_count", strconv.Itoa(room.TrackCount),
		"metadata", string(metadataJSON),
		"updated_at", room.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("failed to update room: %w", err)
	}

	// Update server index if server changed
	if existingRoom.ServerID != room.ServerID {
		// Remove from old server index
		if existingRoom.ServerID != "" {
			oldServerIndexKey := s.serverIndexKey(existingRoom.ServerID)
			if err := s.client.SRem(ctx, oldServerIndexKey, room.RoomID); err != nil {
				return fmt.Errorf("failed to remove from old server index: %w", err)
			}
		}

		// Add to new server index
		if room.ServerID != "" {
			newServerIndexKey := s.serverIndexKey(room.ServerID)
			if err := s.client.SAdd(ctx, newServerIndexKey, room.RoomID); err != nil {
				return fmt.Errorf("failed to add to new server index: %w", err)
			}
		}
	}

	return nil
}

// ListRooms returns all rooms in the store.
func (s *RedisRoomStore) ListRooms(ctx context.Context) ([]*RoomInfo, error) {
	pattern := s.roomKey("*")
	keys, err := s.client.Keys(ctx, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list room keys: %w", err)
	}

	rooms := make([]*RoomInfo, 0, len(keys))
	for _, key := range keys {
		data, err := s.client.HGetAll(ctx, key)
		if err != nil {
			// Skip rooms that fail to fetch (best-effort listing)
			continue
		}
		if len(data) == 0 {
			continue
		}

		room, err := s.parseRoomInfo(data)
		if err != nil {
			// Skip rooms with corrupted data (best-effort listing)
			// In production, consider logging this error for monitoring
			continue
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

// ListRoomsByServer returns all rooms assigned to a specific server.
func (s *RedisRoomStore) ListRoomsByServer(ctx context.Context, serverID string) ([]*RoomInfo, error) {
	serverIndexKey := s.serverIndexKey(serverID)
	roomIDs, err := s.client.SMembers(ctx, serverIndexKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get server rooms: %w", err)
	}

	rooms := make([]*RoomInfo, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		room, err := s.GetRoom(ctx, roomID)
		if err != nil {
			if errors.Is(err, ErrRoomNotFound) {
				// Clean up stale entry from index (best-effort, ignore errors)
				_ = s.client.SRem(ctx, serverIndexKey, roomID)
				continue
			}
			return nil, err
		}
		rooms = append(rooms, room)
	}

	return rooms, nil
}

// RoomExists checks if a room exists in the store.
func (s *RedisRoomStore) RoomExists(ctx context.Context, roomID string) (bool, error) {
	key := s.roomKey(roomID)
	exists, err := s.client.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check room existence: %w", err)
	}
	return exists > 0, nil
}

// UpdateParticipantCount atomically updates the participant count for a room.
func (s *RedisRoomStore) UpdateParticipantCount(ctx context.Context, roomID string, delta int) error {
	key := s.roomKey(roomID)

	// Check if room exists
	exists, err := s.client.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check room existence: %w", err)
	}
	if exists == 0 {
		return ErrRoomNotFound
	}

	// Atomically increment participant count
	_, err = s.client.HIncrBy(ctx, key, "participant_count", int64(delta))
	if err != nil {
		return fmt.Errorf("failed to update participant count: %w", err)
	}

	// Update timestamp
	err = s.client.HSet(ctx, key, "updated_at", time.Now().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("failed to update timestamp: %w", err)
	}

	return nil
}

// UpdateTrackCount atomically updates the track count for a room.
func (s *RedisRoomStore) UpdateTrackCount(ctx context.Context, roomID string, delta int) error {
	key := s.roomKey(roomID)

	// Check if room exists
	exists, err := s.client.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check room existence: %w", err)
	}
	if exists == 0 {
		return ErrRoomNotFound
	}

	// Atomically increment track count
	_, err = s.client.HIncrBy(ctx, key, "track_count", int64(delta))
	if err != nil {
		return fmt.Errorf("failed to update track count: %w", err)
	}

	// Update timestamp
	err = s.client.HSet(ctx, key, "updated_at", time.Now().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("failed to update timestamp: %w", err)
	}

	return nil
}

// UpdateRoomState updates the state of a room.
func (s *RedisRoomStore) UpdateRoomState(ctx context.Context, roomID string, state RoomState) error {
	key := s.roomKey(roomID)

	// Check if room exists
	exists, err := s.client.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to check room existence: %w", err)
	}
	if exists == 0 {
		return ErrRoomNotFound
	}

	// Update state and timestamp
	err = s.client.HSet(ctx, key,
		"state", string(state),
		"updated_at", time.Now().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("failed to update room state: %w", err)
	}

	return nil
}

// Close closes the Redis client.
func (s *RedisRoomStore) Close() error {
	return s.client.Close()
}

// roomKey returns the Redis key for a room.
func (s *RedisRoomStore) roomKey(roomID string) string {
	return fmt.Sprintf("%s%s", s.prefix, roomID)
}

// serverIndexKey returns the Redis key for a server's room index.
func (s *RedisRoomStore) serverIndexKey(serverID string) string {
	return fmt.Sprintf("%sserver:%s", s.prefix, serverID)
}

// parseRoomInfo parses a Redis hash map into a RoomInfo struct.
func (s *RedisRoomStore) parseRoomInfo(data map[string]string) (*RoomInfo, error) {
	// Validate required fields
	roomID := data["room_id"]
	if roomID == "" {
		return nil, fmt.Errorf("missing required field: room_id")
	}

	room := &RoomInfo{
		RoomID:   roomID,
		ServerID: data["server_id"],
		State:    RoomState(data["state"]),
	}

	// Parse participant count
	if pcStr := data["participant_count"]; pcStr != "" {
		pc, err := strconv.Atoi(pcStr)
		if err != nil {
			return nil, fmt.Errorf("invalid participant_count: %w", err)
		}
		room.ParticipantCount = pc
	}

	// Parse max participants
	if mpStr := data["max_participants"]; mpStr != "" {
		mp, err := strconv.Atoi(mpStr)
		if err != nil {
			return nil, fmt.Errorf("invalid max_participants: %w", err)
		}
		room.MaxParticipants = mp
	}

	// Parse track count
	if tcStr := data["track_count"]; tcStr != "" {
		tc, err := strconv.Atoi(tcStr)
		if err != nil {
			return nil, fmt.Errorf("invalid track_count: %w", err)
		}
		room.TrackCount = tc
	}

	// Parse metadata
	if metadataStr := data["metadata"]; metadataStr != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			return nil, fmt.Errorf("invalid metadata JSON: %w", err)
		}
		room.Metadata = metadata
	}

	// Parse created_at
	if createdAtStr := data["created_at"]; createdAtStr != "" {
		t, err := time.Parse(time.RFC3339Nano, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("invalid created_at: %w", err)
		}
		room.CreatedAt = t
	}

	// Parse updated_at
	if updatedAtStr := data["updated_at"]; updatedAtStr != "" {
		t, err := time.Parse(time.RFC3339Nano, updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("invalid updated_at: %w", err)
		}
		room.UpdatedAt = t
	}

	return room, nil
}
