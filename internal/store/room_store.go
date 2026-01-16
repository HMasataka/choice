package store

import (
	"context"
	"errors"
	"time"
)

// RoomState represents the state of a room in the distributed store.
type RoomState string

const (
	// RoomStateCreated indicates the room has been created but has no participants.
	RoomStateCreated RoomState = "created"
	// RoomStateActive indicates the room has one or more participants.
	RoomStateActive RoomState = "active"
	// RoomStateLocked indicates the room is locked and new participants cannot join.
	RoomStateLocked RoomState = "locked"
	// RoomStateClosing indicates the room is in the process of closing.
	RoomStateClosing RoomState = "closing"
	// RoomStateClosed indicates the room has been closed.
	RoomStateClosed RoomState = "closed"
)

// RoomInfo represents room information stored in the distributed store.
// This is used for room-to-server mapping in a multi-instance SFU deployment.
type RoomInfo struct {
	// Metadata is arbitrary metadata associated with the room.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// CreatedAt is the time when the room was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the time when the room was last updated.
	UpdatedAt time.Time `json:"updated_at"`
	// RoomID is the unique identifier for the room.
	RoomID string `json:"room_id"`
	// ServerID is the ID of the SFU server handling this room.
	ServerID string `json:"server_id"`
	// State is the current state of the room.
	State RoomState `json:"state"`
	// ParticipantCount is the number of participants in the room.
	ParticipantCount int `json:"participant_count"`
	// MaxParticipants is the maximum number of participants allowed.
	MaxParticipants int `json:"max_participants"`
	// TrackCount is the current number of tracks in the room.
	TrackCount int `json:"track_count"`
}

// RoomStore errors.
var (
	ErrRoomNotFound      = errors.New("room not found")
	ErrRoomAlreadyExists = errors.New("room already exists")
	ErrInvalidRoomInfo   = errors.New("invalid room info")
)

// RoomStore is an interface for distributed room registry.
// It manages room information across multiple SFU instances.
type RoomStore interface {
	// SaveRoom saves room information to the store.
	// Unlike sessions, rooms do not have TTL and must be explicitly deleted (ADR-0005).
	SaveRoom(ctx context.Context, room *RoomInfo) error

	// GetRoom retrieves room information by room ID.
	GetRoom(ctx context.Context, roomID string) (*RoomInfo, error)

	// DeleteRoom deletes room information from the store.
	DeleteRoom(ctx context.Context, roomID string) error

	// UpdateRoom updates existing room information in the store.
	UpdateRoom(ctx context.Context, room *RoomInfo) error

	// ListRooms returns all rooms in the store.
	ListRooms(ctx context.Context) ([]*RoomInfo, error)

	// ListRoomsByServer returns all rooms assigned to a specific server.
	ListRoomsByServer(ctx context.Context, serverID string) ([]*RoomInfo, error)

	// RoomExists checks if a room exists in the store.
	RoomExists(ctx context.Context, roomID string) (bool, error)

	// UpdateParticipantCount atomically updates the participant count for a room.
	UpdateParticipantCount(ctx context.Context, roomID string, delta int) error

	// UpdateTrackCount atomically updates the track count for a room.
	UpdateTrackCount(ctx context.Context, roomID string, delta int) error

	// UpdateRoomState updates the state of a room.
	UpdateRoomState(ctx context.Context, roomID string, state RoomState) error

	// Close closes the store and cleans up resources.
	Close() error
}

// RoomCoordinator manages room-to-server assignments using consistent hashing.
type RoomCoordinator interface {
	// AssignRoom assigns a room to an SFU server using consistent hashing.
	AssignRoom(ctx context.Context, roomID string) (serverID string, err error)

	// GetServerForRoom returns the assigned server for a room.
	GetServerForRoom(ctx context.Context, roomID string) (serverID string, err error)

	// RebalanceRoom moves a room to a different server for failover.
	RebalanceRoom(ctx context.Context, roomID string, newServerID string) error

	// AddServer adds a server to the consistent hash ring.
	AddServer(serverID string) error

	// RemoveServer removes a server from the consistent hash ring.
	RemoveServer(serverID string) error

	// GetServers returns all servers in the ring.
	GetServers() []string
}
