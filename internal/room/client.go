package room

import (
	"sync"
	"time"

	pkgwebrtc "github.com/HMasataka/choice/pkg/webrtc"

	"github.com/gorilla/websocket"
)

// Client represents a WebRTC client connection
type Client struct {
	ID           string
	RoomID       string
	Name         string // Display name for the client
	WebSocket    *websocket.Conn
	PeerConn     *pkgwebrtc.PeerConnection
	DataChannel  *pkgwebrtc.DataChannel
	AudioTracks  map[string]*pkgwebrtc.AudioTrack
	Connected    time.Time
	LastPing     time.Time
	mu           sync.RWMutex
}

// NewClient creates a new client instance
func NewClient(id, roomID, name string, ws *websocket.Conn, pc *pkgwebrtc.PeerConnection) *Client {
	return &Client{
		ID:           id,
		RoomID:       roomID,
		Name:         name,
		WebSocket:    ws,
		PeerConn:     pc,
		AudioTracks:  make(map[string]*pkgwebrtc.AudioTrack),
		Connected:    time.Now(),
		LastPing:     time.Now(),
	}
}

// SetDataChannel sets the data channel for the client
func (c *Client) SetDataChannel(dc *pkgwebrtc.DataChannel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DataChannel = dc
}

// GetDataChannel returns the data channel (thread-safe)
func (c *Client) GetDataChannel() *pkgwebrtc.DataChannel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DataChannel
}

// AddAudioTrack adds an audio track to the client
func (c *Client) AddAudioTrack(trackID string, track *pkgwebrtc.AudioTrack) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AudioTracks[trackID] = track
}

// RemoveAudioTrack removes an audio track from the client
func (c *Client) RemoveAudioTrack(trackID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.AudioTracks, trackID)
}

// GetAudioTracks returns a copy of audio tracks map (thread-safe)
func (c *Client) GetAudioTracks() map[string]*pkgwebrtc.AudioTrack {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tracks := make(map[string]*pkgwebrtc.AudioTrack)
	for k, v := range c.AudioTracks {
		tracks[k] = v
	}
	return tracks
}

// UpdateLastPing updates the last ping time
func (c *Client) UpdateLastPing() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastPing = time.Now()
}

// IsActive checks if the client is still active (within the last 30 seconds)
func (c *Client) IsActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.LastPing) < 30*time.Second
}

// SendMessage sends a message through the WebSocket connection
func (c *Client) SendMessage(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.WebSocket == nil {
		return ErrClientDisconnected
	}

	return c.WebSocket.WriteMessage(websocket.TextMessage, data)
}

// SendDataChannelMessage sends a message through the DataChannel
func (c *Client) SendDataChannelMessage(data []byte) error {
	c.mu.RLock()
	dc := c.DataChannel
	c.mu.RUnlock()

	if dc == nil {
		return ErrDataChannelNotReady
	}

	return dc.Send(data)
}

// Close closes the client connection and cleans up resources
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error

	// Close data channel
	if c.DataChannel != nil {
		c.DataChannel.Close()
		c.DataChannel = nil
	}

	// Close peer connection
	if c.PeerConn != nil {
		if err := c.PeerConn.Close(); err != nil {
			lastErr = err
		}
		c.PeerConn = nil
	}

	// Close WebSocket
	if c.WebSocket != nil {
		if err := c.WebSocket.Close(); err != nil {
			lastErr = err
		}
		c.WebSocket = nil
	}

	// Close audio tracks
	for _, track := range c.AudioTracks {
		if err := track.Close(); err != nil {
			lastErr = err
		}
	}
	c.AudioTracks = make(map[string]*pkgwebrtc.AudioTrack)

	return lastErr
}

// ClientInfo returns basic information about the client for broadcasting
type ClientInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Connected time.Time `json:"connected"`
	Active    bool      `json:"active"`
}

// GetInfo returns client information
func (c *Client) GetInfo() ClientInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return ClientInfo{
		ID:        c.ID,
		Name:      c.Name,
		Connected: c.Connected,
		Active:    c.IsActive(),
	}
}