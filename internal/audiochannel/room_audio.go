package audiochannel

import (
	"context"
	"log"
	"sync"

	"github.com/HMasataka/choice/internal/room"
	"github.com/pion/webrtc/v4/pkg/media"
)

// RoomAudioManager manages audio broadcasting for rooms using SFU architecture
type RoomAudioManager struct {
	roomManager *room.RoomManager
	sfuManager  *SFUAudioManager
	mu          sync.RWMutex
}

// NewRoomAudioManager creates a new room audio manager with SFU architecture
func NewRoomAudioManager(ctx context.Context, roomManager *room.RoomManager) *RoomAudioManager {
	return &RoomAudioManager{
		roomManager: roomManager,
		sfuManager:  NewSFUAudioManager(ctx, roomManager),
	}
}

// SetupClientAudio sets up SFU-based audio handling for a client
func (ram *RoomAudioManager) SetupClientAudio(client *room.Client) error {
	// Delegate to SFU manager
	return ram.sfuManager.SetupClientAudio(client)
}


// GetRoomAudioStats returns SFU audio statistics for a room
func (ram *RoomAudioManager) GetRoomAudioStats(roomID string) map[string]interface{} {
	// Get SFU-specific stats
	sfuStats := ram.sfuManager.GetSFUAudioStats(roomID)

	// Get traditional audio track stats
	roomObj, exists := ram.roomManager.GetRoom(roomID)
	if !exists {
		sfuStats["clients"] = make(map[string]AudioChannelStats)
		return sfuStats
	}

	clientStats := make(map[string]AudioChannelStats)
	clients := roomObj.GetClients()

	for clientID, client := range clients {
		audioTracks := client.GetAudioTracks()
		for trackLabel, audioTrack := range audioTracks {
			key := clientID + ":" + trackLabel

			// Create basic stats from audio channel
			channelStats := AudioChannelStats{
				Label:      trackLabel,
				Codec:      "audio/opus", // Assuming Opus
				SampleRate: 48000,        // Standard rate
				Channels:   1,            // Mono
				Closed:     false,
			}

			// Get additional stats from audio track if available
			if audioTrack != nil {
				trackStats := audioTrack.Stats()
				channelStats.PacketsSent = trackStats.PacketsSent
				channelStats.PacketsReceived = trackStats.PacketsReceived
				channelStats.BytesSent = trackStats.BytesSent
				channelStats.BytesReceived = trackStats.BytesReceived
			}

			clientStats[key] = channelStats
		}
	}

	sfuStats["clients"] = clientStats
	return sfuStats
}

// BroadcastSystemAudio broadcasts a system audio message to all clients in a room
func (ram *RoomAudioManager) BroadcastSystemAudio(roomID string, audioData []byte) error {
	roomObj, exists := ram.roomManager.GetRoom(roomID)
	if !exists {
		return room.ErrRoomNotFound
	}

	// Create media sample from audio data
	sample := &media.Sample{
		Data:     audioData,
		Duration: 20000000, // 20ms in nanoseconds (typical frame duration)
	}

	// In SFU architecture, system audio can be sent as a special "system" sender
	// that all clients are automatically subscribed to
	sampleWithSender := &AudioSampleWithSender{
		Sample:     sample,
		SenderID:   "system",
		TrackLabel: "system",
		Timestamp:  sample.Duration.Nanoseconds(),
	}

	// Send system audio to all clients
	clients := roomObj.GetClients()
	broadcastCount := 0

	for clientID := range clients {
		if err := ram.sfuManager.sendAudioToClient(clientID, sampleWithSender); err != nil {
			log.Printf("Failed to send system audio to client %s: %v", clientID, err)
		} else {
			broadcastCount++
		}
	}

	log.Printf("SFU: Broadcasted system audio to %d clients in room %s", broadcastCount, roomID)
	return nil
}

// CleanupClientAudio cleans up SFU audio resources for a client
func (ram *RoomAudioManager) CleanupClientAudio(clientID string) {
	// Delegate to SFU manager
	ram.sfuManager.CleanupClientAudio(clientID)
}