package signaling

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenBucket_Allow(t *testing.T) {
	tests := []struct {
		name     string
		capacity float64
		rate     float64
		requests int
		sleep    time.Duration
		want     int // expected successful requests
	}{
		{
			name:     "allow within capacity",
			capacity: 5,
			rate:     10,
			requests: 5,
			sleep:    0,
			want:     5,
		},
		{
			name:     "exceed capacity",
			capacity: 5,
			rate:     10,
			requests: 10,
			sleep:    0,
			want:     5,
		},
		{
			name:     "refill over time",
			capacity: 10,
			rate:     10, // 10 tokens per second
			requests: 15,
			sleep:    100 * time.Millisecond, // Allow refill
			want:     11,                      // 10 initial + ~1 from refill
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := newTokenBucket(tt.capacity, tt.rate)
			allowed := 0

			for i := 0; i < tt.requests; i++ {
				if tt.sleep > 0 && i == 10 {
					time.Sleep(tt.sleep)
				}
				if tb.Allow() {
					allowed++
				}
			}

			// Allow some tolerance for timing-based tests
			if tt.sleep > 0 {
				if allowed < tt.want-1 || allowed > tt.want+1 {
					t.Errorf("Allow() = %d, want ~%d", allowed, tt.want)
				}
			} else {
				if allowed != tt.want {
					t.Errorf("Allow() = %d, want %d", allowed, tt.want)
				}
			}
		})
	}
}

func TestTokenBucket_Concurrent(t *testing.T) {
	tb := newTokenBucket(100, 50)
	var wg sync.WaitGroup
	allowed := int32(0)

	// Spawn multiple goroutines trying to acquire tokens
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if tb.Allow() {
					atomic.AddInt32(&allowed, 1)
				}
			}
		}()
	}

	wg.Wait()

	// Should allow up to capacity
	if got := atomic.LoadInt32(&allowed); got > 100 {
		t.Errorf("Concurrent Allow() = %d, want <= 100", got)
	}
}

func TestBandwidthTracker_Allow(t *testing.T) {
	tests := []struct {
		name       string
		limit      int64
		windowSize time.Duration
		transfers  []int64
		want       []bool
	}{
		{
			name:       "within limit",
			limit:      1000,
			windowSize: time.Second,
			transfers:  []int64{100, 200, 300},
			want:       []bool{true, true, true},
		},
		{
			name:       "exceed limit",
			limit:      1000,
			windowSize: time.Second,
			transfers:  []int64{500, 600},
			want:       []bool{true, false},
		},
		{
			name:       "unlimited",
			limit:      0,
			windowSize: time.Second,
			transfers:  []int64{10000, 20000, 30000},
			want:       []bool{true, true, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := newBandwidthTracker(tt.limit, tt.windowSize)

			for i, bytes := range tt.transfers {
				got := bt.Allow(bytes)
				if got != tt.want[i] {
					t.Errorf("Allow(%d) = %v, want %v", bytes, got, tt.want[i])
				}
			}
		})
	}
}

func TestWebSocketRateLimiter_AllowConnection(t *testing.T) {
	cfg := WebSocketRateLimitConfig{
		ConnectionsPerSecondPerIP:      2,
		MessagesPerSecondPerConnection: 100,
		BurstSize:                      5,
	}
	rl := NewWebSocketRateLimiter(cfg)
	defer rl.Close()

	ip := "192.168.1.1"

	// Should allow initial burst
	for i := 0; i < 5; i++ {
		if !rl.AllowConnection(ip) {
			t.Errorf("AllowConnection(%s) #%d = false, want true", ip, i)
		}
	}

	// Should rate limit after burst
	if rl.AllowConnection(ip) {
		t.Errorf("AllowConnection(%s) after burst = true, want false", ip)
	}

	// Wait for refill
	time.Sleep(600 * time.Millisecond)

	// Should allow after refill (at least 1 token)
	if !rl.AllowConnection(ip) {
		t.Errorf("AllowConnection(%s) after refill = false, want true", ip)
	}
}

func TestWebSocketRateLimiter_AllowMessage(t *testing.T) {
	cfg := WebSocketRateLimitConfig{
		ConnectionsPerSecondPerIP:      10,
		MessagesPerSecondPerConnection: 5,
		MaxMessageSize:                 1000,
		BurstSize:                      10,
	}
	rl := NewWebSocketRateLimiter(cfg)
	defer rl.Close()

	connID := "conn-123"

	// Message too large
	if rl.AllowMessage(connID, 2000) {
		t.Errorf("AllowMessage() with large message = true, want false")
	}

	// Should allow initial burst
	for i := 0; i < 10; i++ {
		if !rl.AllowMessage(connID, 100) {
			t.Errorf("AllowMessage() #%d = false, want true", i)
		}
	}

	// Should rate limit after burst
	if rl.AllowMessage(connID, 100) {
		t.Errorf("AllowMessage() after burst = true, want false")
	}
}

func TestWebSocketRateLimiter_AllowBandwidth(t *testing.T) {
	cfg := WebSocketRateLimitConfig{
		ConnectionsPerSecondPerIP:      10,
		MessagesPerSecondPerConnection: 100,
		BandwidthLimitPerConnection:    1000,
		BandwidthLimitPerRoom:          5000,
	}
	rl := NewWebSocketRateLimiter(cfg)
	defer rl.Close()

	connID := "conn-123"
	roomID := "room-456"

	// Within connection limit
	if !rl.AllowBandwidth(connID, roomID, 500) {
		t.Errorf("AllowBandwidth() = false, want true")
	}

	// Exceed connection limit
	if rl.AllowBandwidth(connID, roomID, 600) {
		t.Errorf("AllowBandwidth() = true, want false (connection limit)")
	}

	// Wait for window reset
	time.Sleep(1100 * time.Millisecond)

	// Use multiple connections to exceed room limit
	for i := 0; i < 5; i++ {
		connID := "conn-" + string(rune(i))
		if !rl.AllowBandwidth(connID, roomID, 1000) {
			t.Errorf("AllowBandwidth() conn %d = false, want true", i)
		}
	}

	// Room limit should be exceeded
	if rl.AllowBandwidth("conn-new", roomID, 100) {
		t.Errorf("AllowBandwidth() = true, want false (room limit)")
	}
}

func TestWebSocketRateLimiter_UnlimitedRates(t *testing.T) {
	cfg := WebSocketRateLimitConfig{
		ConnectionsPerSecondPerIP:      0,
		MessagesPerSecondPerConnection: 0,
		BandwidthLimitPerConnection:    0,
		BandwidthLimitPerRoom:          0,
		MaxMessageSize:                 1024,
	}
	rl := NewWebSocketRateLimiter(cfg)
	defer rl.Close()

	for i := 0; i < 100; i++ {
		if !rl.AllowConnection("192.168.1.1") {
			t.Fatalf("AllowConnection() = false at %d, want true", i)
		}
	}

	for i := 0; i < 100; i++ {
		if !rl.AllowMessage("conn-1", 100) {
			t.Fatalf("AllowMessage() = false at %d, want true", i)
		}
	}

	for i := 0; i < 100; i++ {
		if !rl.AllowBandwidth("conn-1", "room-1", 10) {
			t.Fatalf("AllowBandwidth() = false at %d, want true", i)
		}
	}
}

func TestWebSocketRateLimiter_RemoveConnection(t *testing.T) {
	cfg := DefaultWebSocketRateLimitConfig()
	rl := NewWebSocketRateLimiter(cfg)
	defer rl.Close()

	connID := "conn-123"

	// Add some rate limit entries
	rl.AllowMessage(connID, 100)
	rl.AllowBandwidth(connID, "room-1", 100)

	// Verify entries exist
	if rl.GetConnectionMessageRate(connID) == 0 {
		t.Errorf("Message rate not set")
	}

	// Remove connection
	rl.RemoveConnection(connID)

	// Verify entries are removed
	if rl.GetConnectionMessageRate(connID) != 0 {
		t.Errorf("Message rate still exists after removal")
	}
}

func TestWebSocketRateLimiter_IsValidMessage(t *testing.T) {
	cfg := WebSocketRateLimitConfig{
		MaxMessageSize: 100,
	}
	rl := NewWebSocketRateLimiter(cfg)
	defer rl.Close()

	tests := []struct {
		name    string
		message []byte
		want    bool
	}{
		{
			name:    "valid JSON object",
			message: []byte(`{"method":"test"}`),
			want:    true,
		},
		{
			name:    "valid JSON with leading whitespace",
			message: []byte(" \n\t{\"method\":\"test\"}"),
			want:    true,
		},
		{
			name:    "valid JSON array",
			message: []byte(`[1,2,3]`),
			want:    true,
		},
		{
			name:    "empty message",
			message: []byte{},
			want:    false,
		},
		{
			name:    "invalid format",
			message: []byte(`invalid`),
			want:    false,
		},
		{
			name:    "invalid JSON",
			message: []byte(`{"method":}`),
			want:    false,
		},
		{
			name:    "message too large",
			message: make([]byte, 101),
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize message for "too large" test
			if tt.name == "message too large" {
				tt.message[0] = '{'
			}

			got := rl.IsValidMessage(tt.message)
			if got != tt.want {
				t.Errorf("IsValidMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebSocketRateLimiter_Cleanup(t *testing.T) {
	cfg := WebSocketRateLimitConfig{
		ConnectionsPerSecondPerIP:      10,
		MessagesPerSecondPerConnection: 100,
	}
	rl := NewWebSocketRateLimiter(cfg)

	// Add some entries
	rl.AllowConnection("192.168.1.1")
	rl.AllowMessage("conn-1", 100)

	// Trigger cleanup manually
	rl.cleanup()

	// Cleanup shouldn't remove recent entries, but this tests the cleanup mechanism
	// In a real scenario, we'd need to wait for entries to become stale

	rl.Close()
}
