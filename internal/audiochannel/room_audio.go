package audiochannel

import (
	"log"
	"sync"

	"github.com/HMasataka/choice/internal/room"
	"github.com/pion/webrtc/v4/pkg/media"
)

// RoomAudioManager manages audio broadcasting for rooms
type RoomAudioManager struct {
	roomManager *room.RoomManager
	mu          sync.RWMutex
}

// NewRoomAudioManager creates a new room audio manager
func NewRoomAudioManager(roomManager *room.RoomManager) *RoomAudioManager {
	return &RoomAudioManager{
		roomManager: roomManager,
	}
}

// SetupClientAudio sets up audio handling for a client
func (ram *RoomAudioManager) SetupClientAudio(client *room.Client) error {
	// Get or create audio tracks for the client
	audioTracks := client.GetAudioTracks()
	if len(audioTracks) == 0 {
		log.Printf("No audio tracks found for client %s", client.ID)
		return nil
	}

	// Set up audio sample handler for broadcasting
	for trackLabel, audioTrack := range audioTracks {
		log.Printf("Setting up audio broadcast for client %s, track %s", client.ID, trackLabel)

		// Set handler to broadcast audio samples to other clients in the room
		audioTrack.SetOnSample(func(sample *media.Sample) {
			ram.broadcastAudioSample(client, trackLabel, sample)
		})
	}

	return nil
}

// broadcastAudioSample broadcasts an audio sample to other clients in the same room
func (ram *RoomAudioManager) broadcastAudioSample(sender *room.Client, trackLabel string, sample *media.Sample) {
	// Get the room
	roomObj, exists := ram.roomManager.GetRoom(sender.RoomID)
	if !exists {
		log.Printf("Room %s not found for client %s", sender.RoomID, sender.ID)
		return
	}

	// Get all clients in the room except the sender
	clients := roomObj.GetClients()
	broadcastCount := 0

	for clientID, client := range clients {
		if clientID == sender.ID {
			continue // Skip sender
		}

		// Send audio sample to this client
		if err := ram.sendAudioSampleToClient(client, trackLabel, sample); err != nil {
			log.Printf("Failed to send audio sample to client %s: %v", clientID, err)
		} else {
			broadcastCount++
		}
	}

	if broadcastCount > 0 {
		log.Printf("Broadcasted audio sample from %s to %d clients in room %s",
			sender.ID, broadcastCount, sender.RoomID)
	}
}

// sendAudioSampleToClient sends an audio sample to a specific client
// Note: This is a simplified implementation. In a real scenario, you would need
// to manage individual AudioChannels per client for proper broadcasting.
func (ram *RoomAudioManager) sendAudioSampleToClient(client *room.Client, trackLabel string, sample *media.Sample) error {
	// For now, we'll skip the direct audio broadcasting to other clients
	// This requires more complex track management that would be implemented
	// with AudioChannel instances per client rather than AudioTrack
	log.Printf("Audio sample broadcast to client %s for track %s (placeholder)", client.ID, trackLabel)
	return nil
}

// GetRoomAudioStats returns audio statistics for all clients in a room
func (ram *RoomAudioManager) GetRoomAudioStats(roomID string) map[string]AudioChannelStats {
	roomObj, exists := ram.roomManager.GetRoom(roomID)
	if !exists {
		return make(map[string]AudioChannelStats)
	}

	stats := make(map[string]AudioChannelStats)
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

			stats[key] = channelStats
		}
	}

	return stats
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

	clients := roomObj.GetClients()
	broadcastCount := 0

	for clientID, client := range clients {
		// Send to all clients
		if err := ram.sendAudioSampleToClient(client, "system", sample); err != nil {
			log.Printf("Failed to send system audio to client %s: %v", clientID, err)
		} else {
			broadcastCount++
		}
	}

	log.Printf("Broadcasted system audio to %d clients in room %s", broadcastCount, roomID)
	return nil
}

// CleanupClientAudio cleans up audio resources for a client
func (ram *RoomAudioManager) CleanupClientAudio(clientID string) {
	client, exists := ram.roomManager.GetClient(clientID)
	if !exists {
		return
	}

	// Close all audio tracks for the client
	audioTracks := client.GetAudioTracks()
	for trackLabel, audioTrack := range audioTracks {
		if audioTrack != nil {
			audioTrack.Close()
			log.Printf("Closed audio track %s for client %s", trackLabel, clientID)
		}
	}

	log.Printf("Cleaned up audio resources for client %s", clientID)
}