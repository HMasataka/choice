package audiochannel

import (
	"context"
	"log"
	"sync"

	"github.com/HMasataka/choice/internal/room"
	"github.com/pion/webrtc/v4/pkg/media"
)

// SFUAudioManager manages Selective Forwarding Unit audio distribution
type SFUAudioManager struct {
	ctx         context.Context
	roomManager *room.RoomManager

	// Track subscribers management
	// roomID -> clientID -> []subscribedClientIDs
	trackSubscriptions map[string]map[string][]string

	// Audio sample forwarding channels
	// key: "roomID:senderID" -> channel for audio samples
	audioForwarders map[string]chan *AudioSampleWithSender

	mu sync.RWMutex
}

// AudioSampleWithSender wraps audio sample with sender information
type AudioSampleWithSender struct {
	Sample     *media.Sample
	SenderID   string
	TrackLabel string
	Timestamp  int64
}

// NewSFUAudioManager creates a new SFU audio manager
func NewSFUAudioManager(ctx context.Context, roomManager *room.RoomManager) *SFUAudioManager {
	return &SFUAudioManager{
		ctx:                ctx,
		roomManager:        roomManager,
		trackSubscriptions: make(map[string]map[string][]string),
		audioForwarders:    make(map[string]chan *AudioSampleWithSender),
	}
}

// SetupClientAudio sets up SFU audio handling for a client
func (sfu *SFUAudioManager) SetupClientAudio(client *room.Client) error {
	roomID := client.RoomID
	clientID := client.ID

	if roomID == "" {
		log.Printf("Client %s has no room ID", clientID)
		return nil
	}

	// Initialize room subscriptions if not exists
	sfu.mu.Lock()
	if _, exists := sfu.trackSubscriptions[roomID]; !exists {
		sfu.trackSubscriptions[roomID] = make(map[string][]string)
	}
	sfu.trackSubscriptions[roomID][clientID] = []string{}
	sfu.mu.Unlock()

	// Get existing clients in the room to subscribe to
	roomObj, exists := sfu.roomManager.GetRoom(roomID)
	if !exists {
		log.Printf("Room %s not found", roomID)
		return nil
	}

	existingClients := roomObj.GetClients()

	// Subscribe to all existing clients' audio tracks
	for existingClientID := range existingClients {
		if existingClientID != clientID {
			sfu.subscribeToClient(roomID, clientID, existingClientID)
		}
	}

	// Make all existing clients subscribe to this new client
	for existingClientID := range existingClients {
		if existingClientID != clientID {
			sfu.subscribeToClient(roomID, existingClientID, clientID)
		}
	}

	// Set up audio sample forwarding for this client's outgoing audio
	audioTracks := client.GetAudioTracks()
	for trackLabel, audioTrack := range audioTracks {
		log.Printf("Setting up SFU audio forwarding for client %s, track %s", clientID, trackLabel)

		// Create forwarder channel if not exists
		forwarderKey := roomID + ":" + clientID
		sfu.mu.Lock()
		if _, exists := sfu.audioForwarders[forwarderKey]; !exists {
			sfu.audioForwarders[forwarderKey] = make(chan *AudioSampleWithSender, 100)
			go sfu.startAudioForwarder(forwarderKey, roomID, clientID)
		}
		sfu.mu.Unlock()

		// Set handler to forward audio samples to subscribers
		audioTrack.SetOnSample(func(sample *media.Sample) {
			sampleWithSender := &AudioSampleWithSender{
				Sample:     sample,
				SenderID:   clientID,
				TrackLabel: trackLabel,
				Timestamp:  sample.Duration.Nanoseconds(),
			}

			// Send to forwarder (non-blocking)
			select {
			case sfu.audioForwarders[forwarderKey] <- sampleWithSender:
			default:
				log.Printf("Audio forwarder channel full for %s", forwarderKey)
			}
		})
	}

	log.Printf("SFU audio setup completed for client %s in room %s", clientID, roomID)
	return nil
}

// subscribeToClient subscribes subscriberID to senderID's audio tracks
func (sfu *SFUAudioManager) subscribeToClient(roomID, subscriberID, senderID string) {
	sfu.mu.Lock()
	defer sfu.mu.Unlock()

	if roomSubscriptions, exists := sfu.trackSubscriptions[roomID]; exists {
		if subscriberList, exists := roomSubscriptions[subscriberID]; exists {
			// Check if already subscribed
			for _, existingSender := range subscriberList {
				if existingSender == senderID {
					return // Already subscribed
				}
			}
			// Add subscription
			sfu.trackSubscriptions[roomID][subscriberID] = append(subscriberList, senderID)
			log.Printf("Client %s subscribed to client %s's audio in room %s", subscriberID, senderID, roomID)
		}
	}
}

// unsubscribeFromClient unsubscribes subscriberID from senderID's audio tracks
func (sfu *SFUAudioManager) unsubscribeFromClient(roomID, subscriberID, senderID string) {
	sfu.mu.Lock()
	defer sfu.mu.Unlock()

	if roomSubscriptions, exists := sfu.trackSubscriptions[roomID]; exists {
		if subscriberList, exists := roomSubscriptions[subscriberID]; exists {
			// Remove subscription
			newList := []string{}
			for _, existingSender := range subscriberList {
				if existingSender != senderID {
					newList = append(newList, existingSender)
				}
			}
			sfu.trackSubscriptions[roomID][subscriberID] = newList
			log.Printf("Client %s unsubscribed from client %s's audio in room %s", subscriberID, senderID, roomID)
		}
	}
}

// startAudioForwarder starts the audio forwarding routine for a client
func (sfu *SFUAudioManager) startAudioForwarder(forwarderKey, roomID, senderID string) {
	log.Printf("Started audio forwarder for %s", forwarderKey)

	sfu.mu.RLock()
	forwarderChan := sfu.audioForwarders[forwarderKey]
	sfu.mu.RUnlock()

	for {
		select {
		case <-sfu.ctx.Done():
			log.Printf("Audio forwarder stopped for %s (context done)", forwarderKey)
			return

		case sampleWithSender, ok := <-forwarderChan:
			if !ok {
				log.Printf("Audio forwarder stopped for %s (channel closed)", forwarderKey)
				return
			}

			// Forward audio sample to all subscribers
			sfu.forwardAudioSample(roomID, sampleWithSender)
		}
	}
}

// forwardAudioSample forwards audio sample to all subscribers
func (sfu *SFUAudioManager) forwardAudioSample(roomID string, sampleWithSender *AudioSampleWithSender) {
	sfu.mu.RLock()
	roomSubscriptions, roomExists := sfu.trackSubscriptions[roomID]
	sfu.mu.RUnlock()

	if !roomExists {
		return
	}

	senderID := sampleWithSender.SenderID
	forwardedCount := 0

	// Get all subscribers of this sender
	for subscriberID, subscribedSenders := range roomSubscriptions {
		if subscriberID == senderID {
			continue // Don't send to sender
		}

		// Check if this subscriber is subscribed to the sender
		isSubscribed := false
		for _, subscribedSender := range subscribedSenders {
			if subscribedSender == senderID {
				isSubscribed = true
				break
			}
		}

		if isSubscribed {
			// Forward audio sample to subscriber
			if err := sfu.sendAudioToClient(subscriberID, sampleWithSender); err != nil {
				log.Printf("Failed to forward audio from %s to %s: %v", senderID, subscriberID, err)
			} else {
				forwardedCount++
			}
		}
	}

	if forwardedCount > 0 {
		log.Printf("Forwarded audio from %s to %d subscribers in room %s", senderID, forwardedCount, roomID)
	}
}

// sendAudioToClient sends audio sample to a specific client
func (sfu *SFUAudioManager) sendAudioToClient(clientID string, sampleWithSender *AudioSampleWithSender) error {
	_, exists := sfu.roomManager.GetClient(clientID)
	if !exists {
		return nil // Client not found
	}

	// In a real implementation, this would send the audio through a dedicated
	// receiving audio track for this specific sender

	// For now, we'll use a placeholder implementation
	log.Printf("SFU: Audio from %s forwarded to %s (track: %s, size: %d bytes)",
		sampleWithSender.SenderID,
		clientID,
		sampleWithSender.TrackLabel,
		len(sampleWithSender.Sample.Data))

	// TODO: In a real implementation, you would:
	// 1. Get the client's receiving audio track for this specific sender
	// 2. Send the sample through that track
	// 3. Handle any necessary format conversion or buffering

	return nil
}

// CleanupClientAudio cleans up SFU audio resources for a client
func (sfu *SFUAudioManager) CleanupClientAudio(clientID string) {
	client, exists := sfu.roomManager.GetClient(clientID)
	if !exists {
		log.Printf("Client %s not found for SFU audio cleanup", clientID)
		return
	}

	roomID := client.RoomID
	if roomID == "" {
		return
	}

	// Remove client from all subscriptions
	sfu.mu.Lock()
	if roomSubscriptions, exists := sfu.trackSubscriptions[roomID]; exists {
		// Remove client's subscriptions
		delete(roomSubscriptions, clientID)

		// Remove client from other clients' subscriptions
		for subscriberID, subscribedSenders := range roomSubscriptions {
			newList := []string{}
			for _, senderID := range subscribedSenders {
				if senderID != clientID {
					newList = append(newList, senderID)
				}
			}
			roomSubscriptions[subscriberID] = newList
		}

		// Clean up empty room subscriptions
		if len(roomSubscriptions) == 0 {
			delete(sfu.trackSubscriptions, roomID)
		}
	}

	// Close audio forwarder
	forwarderKey := roomID + ":" + clientID
	if forwarderChan, exists := sfu.audioForwarders[forwarderKey]; exists {
		close(forwarderChan)
		delete(sfu.audioForwarders, forwarderKey)
	}
	sfu.mu.Unlock()

	// Close client's audio tracks
	audioTracks := client.GetAudioTracks()
	for trackLabel, audioTrack := range audioTracks {
		if audioTrack != nil {
			audioTrack.Close()
			log.Printf("Closed audio track %s for client %s", trackLabel, clientID)
		}
	}

	log.Printf("SFU audio cleanup completed for client %s", clientID)
}

// GetSFUAudioStats returns SFU audio statistics for a room
func (sfu *SFUAudioManager) GetSFUAudioStats(roomID string) map[string]interface{} {
	sfu.mu.RLock()
	defer sfu.mu.RUnlock()

	stats := make(map[string]interface{})

	// Get subscription stats
	if roomSubscriptions, exists := sfu.trackSubscriptions[roomID]; exists {
		subscriptionStats := make(map[string]interface{})

		for clientID, subscribedSenders := range roomSubscriptions {
			subscriptionStats[clientID] = map[string]interface{}{
				"subscribed_to":    subscribedSenders,
				"subscription_count": len(subscribedSenders),
			}
		}

		stats["subscriptions"] = subscriptionStats
		stats["total_clients"] = len(roomSubscriptions)

		// Calculate total forwarding channels
		forwarderCount := 0
		for key := range sfu.audioForwarders {
			if len(key) > len(roomID) && key[:len(roomID)] == roomID {
				forwarderCount++
			}
		}
		stats["active_forwarders"] = forwarderCount
	}

	return stats
}

// GetAllSFUStats returns SFU statistics for all rooms
func (sfu *SFUAudioManager) GetAllSFUStats() map[string]interface{} {
	sfu.mu.RLock()
	defer sfu.mu.RUnlock()

	allStats := make(map[string]interface{})

	for roomID := range sfu.trackSubscriptions {
		allStats[roomID] = sfu.GetSFUAudioStats(roomID)
	}

	allStats["global"] = map[string]interface{}{
		"total_rooms":      len(sfu.trackSubscriptions),
		"total_forwarders": len(sfu.audioForwarders),
	}

	return allStats
}