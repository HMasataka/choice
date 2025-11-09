package room

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateClientID generates a unique client ID
func GenerateClientID() string {
	timestamp := time.Now().Unix()
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)

	return fmt.Sprintf("client_%d_%s", timestamp, hex.EncodeToString(randomBytes)[:8])
}

// GenerateRoomID generates a unique room ID
func GenerateRoomID() string {
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)

	return fmt.Sprintf("room_%s", hex.EncodeToString(randomBytes))
}

// SanitizeRoomID sanitizes a room ID to make it safe
func SanitizeRoomID(roomID string) string {
	if roomID == "" {
		return "default"
	}

	// Replace unsafe characters with underscores
	result := make([]rune, 0, len(roomID))
	for _, char := range roomID {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			result = append(result, char)
		} else {
			result = append(result, '_')
		}
	}

	sanitized := string(result)

	// Limit length
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}

	// Ensure it's not empty after sanitization
	if sanitized == "" {
		sanitized = "default"
	}

	return sanitized
}

// GetDefaultRoomName returns a default room name based on ID
func GetDefaultRoomName(roomID string) string {
	return fmt.Sprintf("Room %s", roomID)
}

// ClientStats represents client statistics
type ClientStats struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	RoomID       string    `json:"room_id"`
	Connected    time.Time `json:"connected"`
	LastPing     time.Time `json:"last_ping"`
	IsActive     bool      `json:"is_active"`
	AudioTracks  int       `json:"audio_tracks"`
	HasDataChannel bool    `json:"has_data_channel"`
}

// GetClientStats returns detailed client statistics
func (c *Client) GetStats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ClientStats{
		ID:           c.ID,
		Name:         c.Name,
		RoomID:       c.RoomID,
		Connected:    c.Connected,
		LastPing:     c.LastPing,
		IsActive:     c.IsActive(),
		AudioTracks:  len(c.AudioTracks),
		HasDataChannel: c.DataChannel != nil,
	}
}

// RoomStats represents room statistics
type RoomStats struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	MaxClients   int           `json:"max_clients"`
	ClientCount  int           `json:"client_count"`
	Created      time.Time     `json:"created"`
	IsFull       bool          `json:"is_full"`
	Clients      []ClientStats `json:"clients"`
}

// GetStats returns detailed room statistics
func (r *Room) GetStats() RoomStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make([]ClientStats, 0, len(r.Clients))
	for _, client := range r.Clients {
		clients = append(clients, client.GetStats())
	}

	return RoomStats{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		MaxClients:  r.MaxClients,
		ClientCount: len(r.Clients),
		Created:     r.Created,
		IsFull:      r.MaxClients > 0 && len(r.Clients) >= r.MaxClients,
		Clients:     clients,
	}
}

// ManagerStats represents room manager statistics
type ManagerStats struct {
	TotalRooms   int         `json:"total_rooms"`
	TotalClients int         `json:"total_clients"`
	Rooms        []RoomStats `json:"rooms"`
	Uptime       time.Time   `json:"uptime"`
}

// GetStats returns detailed manager statistics
func (rm *RoomManager) GetStats() ManagerStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rooms := make([]RoomStats, 0, len(rm.Rooms))
	for _, room := range rm.Rooms {
		rooms = append(rooms, room.GetStats())
	}

	return ManagerStats{
		TotalRooms:   len(rm.Rooms),
		TotalClients: len(rm.Clients),
		Rooms:        rooms,
		Uptime:       time.Now(), // This would be better with actual start time
	}
}