package webrtc

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTURNCredentials_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "just expired",
			expiresAt: time.Now().Add(-1 * time.Millisecond),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &TURNCredentials{
				ExpiresAt: tt.expiresAt,
			}
			if got := c.IsExpired(); got != tt.want {
				t.Errorf("TURNCredentials.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTURNCredentials_TimeToExpiry(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		wantZero  bool
	}{
		{
			name:      "future expiry",
			expiresAt: time.Now().Add(1 * time.Hour),
			wantZero:  false,
		},
		{
			name:      "past expiry",
			expiresAt: time.Now().Add(-1 * time.Hour),
			wantZero:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &TURNCredentials{
				ExpiresAt: tt.expiresAt,
			}
			got := c.TimeToExpiry()
			if tt.wantZero && got != 0 {
				t.Errorf("TURNCredentials.TimeToExpiry() = %v, want 0", got)
			}
			if !tt.wantZero && got <= 0 {
				t.Errorf("TURNCredentials.TimeToExpiry() = %v, want > 0", got)
			}
		})
	}
}

func TestTURNCredentials_NeedsRotation(t *testing.T) {
	rotationInterval := 12 * time.Hour

	tests := []struct {
		name      string
		createdAt time.Time
		want      bool
	}{
		{
			name:      "needs rotation - elapsed time exceeded interval",
			createdAt: time.Now().Add(-13 * time.Hour),
			want:      true,
		},
		{
			name:      "no rotation needed - elapsed time within interval",
			createdAt: time.Now().Add(-5 * time.Hour),
			want:      false,
		},
		{
			name:      "needs rotation - exactly at interval",
			createdAt: time.Now().Add(-12 * time.Hour),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &TURNCredentials{
				CreatedAt: tt.createdAt,
				ExpiresAt: tt.createdAt.Add(24 * time.Hour),
			}
			if got := c.NeedsRotation(rotationInterval); got != tt.want {
				t.Errorf("TURNCredentials.NeedsRotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTURNConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TURNConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: TURNConfig{
				URLs:             []string{"turn:turn.example.com:3478"},
				Secret:           "test-secret",
				CredentialTTL:    24 * time.Hour,
				RotationInterval: 12 * time.Hour,
			},
			wantErr: nil,
		},
		{
			name: "empty URLs",
			config: TURNConfig{
				Secret:           "test-secret",
				CredentialTTL:    24 * time.Hour,
				RotationInterval: 12 * time.Hour,
			},
			wantErr: ErrEmptyTURNURLs,
		},
		{
			name: "empty secret",
			config: TURNConfig{
				URLs:             []string{"turn:turn.example.com:3478"},
				Secret:           "",
				CredentialTTL:    24 * time.Hour,
				RotationInterval: 12 * time.Hour,
			},
			wantErr: ErrEmptySecret,
		},
		{
			name: "invalid TTL",
			config: TURNConfig{
				URLs:             []string{"turn:turn.example.com:3478"},
				Secret:           "test-secret",
				CredentialTTL:    0,
				RotationInterval: 12 * time.Hour,
			},
			wantErr: ErrInvalidTTL,
		},
		{
			name: "invalid rotation interval",
			config: TURNConfig{
				URLs:             []string{"turn:turn.example.com:3478"},
				Secret:           "test-secret",
				CredentialTTL:    24 * time.Hour,
				RotationInterval: 0,
			},
			wantErr: ErrInvalidTTL,
		},
		{
			name: "rotation interval >= TTL",
			config: TURNConfig{
				URLs:             []string{"turn:turn.example.com:3478"},
				Secret:           "test-secret",
				CredentialTTL:    24 * time.Hour,
				RotationInterval: 24 * time.Hour,
			},
			wantErr: ErrInvalidTTL, // Expect rotation interval error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("TURNConfig.Validate() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) && err.Error() != tt.wantErr.Error() {
					// Check if it's the rotation interval error
					if tt.name == "rotation interval >= TTL" {
						if !strings.Contains(err.Error(), "rotation interval must be less than credential TTL") {
							t.Errorf("TURNConfig.Validate() error = %v, wantErr contains 'rotation interval'", err)
						}
					} else {
						t.Errorf("TURNConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
					}
				}
			} else {
				if err != nil {
					t.Errorf("TURNConfig.Validate() error = %v, wantErr nil", err)
				}
			}
		})
	}
}

func TestDefaultTURNConfig(t *testing.T) {
	config := DefaultTURNConfig()

	if config.CredentialTTL != DefaultCredentialTTL {
		t.Errorf("DefaultTURNConfig().CredentialTTL = %v, want %v", config.CredentialTTL, DefaultCredentialTTL)
	}
	if config.RotationInterval != DefaultRotationInterval {
		t.Errorf("DefaultTURNConfig().RotationInterval = %v, want %v", config.RotationInterval, DefaultRotationInterval)
	}
}

func TestNewTURNCredentialService(t *testing.T) {
	tests := []struct {
		name    string
		config  TURNConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: TURNConfig{
				URLs:             []string{"turn:turn.example.com:3478"},
				Secret:           "test-secret",
				CredentialTTL:    24 * time.Hour,
				RotationInterval: 12 * time.Hour,
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: TURNConfig{
				URLs:             []string{},
				Secret:           "test-secret",
				CredentialTTL:    24 * time.Hour,
				RotationInterval: 12 * time.Hour,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTURNCredentialService(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTURNCredentialService() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Error("NewTURNCredentialService() = nil, want non-nil")
			}
		})
	}
}

func TestTURNCredentialService_GenerateCredentials(t *testing.T) {
	config := TURNConfig{
		URLs:             []string{"turn:turn.example.com:3478"},
		Secret:           "test-secret",
		CredentialTTL:    24 * time.Hour,
		RotationInterval: 12 * time.Hour,
	}
	service, err := NewTURNCredentialService(config)
	if err != nil {
		t.Fatalf("NewTURNCredentialService() error = %v", err)
	}

	tests := []struct {
		name          string
		participantID string
		wantErr       bool
	}{
		{
			name:          "valid participant ID",
			participantID: "user-123",
			wantErr:       false,
		},
		{
			name:          "empty participant ID",
			participantID: "",
			wantErr:       true,
		},
		{
			name:          "participant ID with colon",
			participantID: "user:123",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.GenerateCredentials(tt.participantID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Validate generated credentials
			if got == nil {
				t.Error("GenerateCredentials() = nil, want non-nil")
				return
			}
			if len(got.URLs) != len(config.URLs) {
				t.Errorf("GenerateCredentials() URLs count = %d, want %d", len(got.URLs), len(config.URLs))
			}
			if got.Username == "" {
				t.Error("GenerateCredentials() Username is empty")
			}
			if got.Credential == "" {
				t.Error("GenerateCredentials() Credential is empty")
			}
			if got.CreatedAt.IsZero() {
				t.Error("GenerateCredentials() CreatedAt is zero")
			}
			if got.ExpiresAt.Before(time.Now()) {
				t.Error("GenerateCredentials() ExpiresAt is in the past")
			}

			// Validate username format (should be "timestamp:participantID")
			parts := strings.Split(got.Username, ":")
			if len(parts) != 2 {
				t.Errorf("GenerateCredentials() Username format invalid: %s", got.Username)
			}
			if parts[1] != tt.participantID {
				t.Errorf("GenerateCredentials() Username participant part = %s, want %s", parts[1], tt.participantID)
			}
		})
	}
}

func TestTURNCredentialService_RefreshCredentials(t *testing.T) {
	config := TURNConfig{
		URLs:             []string{"turn:turn.example.com:3478"},
		Secret:           "test-secret",
		CredentialTTL:    24 * time.Hour,
		RotationInterval: 1 * time.Second, // Short interval for testing
	}
	service, err := NewTURNCredentialService(config)
	if err != nil {
		t.Fatalf("NewTURNCredentialService() error = %v", err)
	}

	participantID := "user-123"

	// Generate initial credentials
	creds1, err := service.GenerateCredentials(participantID)
	if err != nil {
		t.Fatalf("GenerateCredentials() error = %v", err)
	}

	// Refresh immediately (should return same credentials)
	creds2, err := service.RefreshCredentials(participantID)
	if err != nil {
		t.Fatalf("RefreshCredentials() error = %v", err)
	}
	if creds1.Username != creds2.Username {
		t.Error("RefreshCredentials() should return same credentials when not needed")
	}

	// Wait for rotation interval to pass
	time.Sleep(1100 * time.Millisecond)

	// Refresh again (should generate new credentials)
	creds3, err := service.RefreshCredentials(participantID)
	if err != nil {
		t.Fatalf("RefreshCredentials() error = %v", err)
	}
	if creds1.Username == creds3.Username {
		t.Error("RefreshCredentials() should generate new credentials after rotation interval")
	}
}

func TestTURNCredentialService_CleanupExpiredCredentials(t *testing.T) {
	config := TURNConfig{
		URLs:             []string{"turn:turn.example.com:3478"},
		Secret:           "test-secret",
		CredentialTTL:    500 * time.Millisecond,
		RotationInterval: 250 * time.Millisecond,
	}
	service, err := NewTURNCredentialService(config)
	if err != nil {
		t.Fatalf("NewTURNCredentialService() error = %v", err)
	}

	impl := service.(*turnCredentialService)

	// Generate credentials for multiple participants
	_, _ = service.GenerateCredentials("user-1")
	_, _ = service.GenerateCredentials("user-2")
	_, _ = service.GenerateCredentials("user-3")

	// Check initial count
	impl.mu.RLock()
	initialCount := len(impl.credentials)
	impl.mu.RUnlock()

	if initialCount != 3 {
		t.Errorf("Initial credential count = %d, want 3", initialCount)
	}

	// Wait for expiration
	time.Sleep(600 * time.Millisecond)

	// Cleanup
	impl.CleanupExpiredCredentials()

	// Check count after cleanup
	impl.mu.RLock()
	finalCount := len(impl.credentials)
	impl.mu.RUnlock()

	if finalCount != 0 {
		t.Errorf("Final credential count after cleanup = %d, want 0", finalCount)
	}
}

func TestBuildTURNURLs(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		port      int
		protocols []TURNProtocol
		want      []string
	}{
		{
			name:      "single UDP",
			host:      "turn.example.com",
			port:      3478,
			protocols: []TURNProtocol{TURNProtocolUDP},
			want:      []string{"turn:turn.example.com:3478?transport=udp"},
		},
		{
			name:      "UDP and TCP",
			host:      "turn.example.com",
			port:      3478,
			protocols: []TURNProtocol{TURNProtocolUDP, TURNProtocolTCP},
			want: []string{
				"turn:turn.example.com:3478?transport=udp",
				"turn:turn.example.com:3478?transport=tcp",
			},
		},
		{
			name:      "TLS",
			host:      "turn.example.com",
			port:      443,
			protocols: []TURNProtocol{TURNProtocolTLS},
			want:      []string{"turns:turn.example.com:443?transport=tcp"},
		},
		{
			name:      "all protocols",
			host:      "turn.example.com",
			port:      3478,
			protocols: []TURNProtocol{TURNProtocolUDP, TURNProtocolTCP, TURNProtocolTLS},
			want: []string{
				"turn:turn.example.com:3478?transport=udp",
				"turn:turn.example.com:3478?transport=tcp",
				"turns:turn.example.com:3478?transport=tcp",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildTURNURLs(tt.host, tt.port, tt.protocols...)
			if len(got) != len(tt.want) {
				t.Errorf("BuildTURNURLs() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i, url := range got {
				if url != tt.want[i] {
					t.Errorf("BuildTURNURLs()[%d] = %s, want %s", i, url, tt.want[i])
				}
			}
		})
	}
}

func TestMergeTURNCredentials(t *testing.T) {
	stunServers := []ICEServerConfig{
		{URLs: []string{"stun:stun.example.com:3478"}},
	}

	now := time.Now()
	turnCreds := &TURNCredentials{
		URLs:       []string{"turn:turn.example.com:3478"},
		Username:   "1234567890:user-123",
		Credential: "test-credential",
		CreatedAt:  now,
		ExpiresAt:  now.Add(24 * time.Hour),
	}

	tests := []struct {
		name        string
		stunServers []ICEServerConfig
		turnCreds   *TURNCredentials
		wantCount   int
	}{
		{
			name:        "merge TURN with STUN",
			stunServers: stunServers,
			turnCreds:   turnCreds,
			wantCount:   2,
		},
		{
			name:        "nil TURN credentials",
			stunServers: stunServers,
			turnCreds:   nil,
			wantCount:   1,
		},
		{
			name:        "empty STUN servers",
			stunServers: []ICEServerConfig{},
			turnCreds:   turnCreds,
			wantCount:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeTURNCredentials(tt.stunServers, tt.turnCreds)
			if len(got) != tt.wantCount {
				t.Errorf("MergeTURNCredentials() length = %d, want %d", len(got), tt.wantCount)
				return
			}

			// Verify TURN credentials are added correctly if provided
			if tt.turnCreds != nil {
				turnServer := got[len(got)-1]
				if len(turnServer.URLs) != len(tt.turnCreds.URLs) {
					t.Errorf("TURN server URLs count = %d, want %d", len(turnServer.URLs), len(tt.turnCreds.URLs))
				}
				if turnServer.Username != tt.turnCreds.Username {
					t.Errorf("TURN server Username = %s, want %s", turnServer.Username, tt.turnCreds.Username)
				}
				if turnServer.Credential != tt.turnCreds.Credential {
					t.Errorf("TURN server Credential = %s, want %s", turnServer.Credential, tt.turnCreds.Credential)
				}
			}
		})
	}
}

func TestTURNCredentialService_GetCredentials(t *testing.T) {
	config := TURNConfig{
		URLs:             []string{"turn:turn.example.com:3478"},
		Secret:           "test-secret",
		CredentialTTL:    1 * time.Second,
		RotationInterval: 500 * time.Millisecond,
	}
	service, err := NewTURNCredentialService(config)
	if err != nil {
		t.Fatalf("NewTURNCredentialService() error = %v", err)
	}

	impl := service.(*turnCredentialService)
	participantID := "user-123"

	// Test getting credentials before generation
	creds := impl.GetCredentials(participantID)
	if creds != nil {
		t.Error("GetCredentials() should return nil for non-existent credentials")
	}

	// Generate credentials
	_, err = service.GenerateCredentials(participantID)
	if err != nil {
		t.Fatalf("GenerateCredentials() error = %v", err)
	}

	// Test getting valid credentials
	creds = impl.GetCredentials(participantID)
	if creds == nil {
		t.Error("GetCredentials() should return non-nil for existing credentials")
	}

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	// Test getting expired credentials
	creds = impl.GetCredentials(participantID)
	if creds != nil {
		t.Error("GetCredentials() should return nil for expired credentials")
	}
}

func TestTURNCredentialService_ReturnedCredentialsAreCopies(t *testing.T) {
	config := TURNConfig{
		URLs:             []string{"turn:turn.example.com:3478"},
		Secret:           "test-secret",
		CredentialTTL:    24 * time.Hour,
		RotationInterval: 12 * time.Hour,
	}
	service, err := NewTURNCredentialService(config)
	if err != nil {
		t.Fatalf("NewTURNCredentialService() error = %v", err)
	}

	impl := service.(*turnCredentialService)
	participantID := "user-123"

	// Generate credentials
	creds1, err := service.GenerateCredentials(participantID)
	if err != nil {
		t.Fatalf("GenerateCredentials() error = %v", err)
	}

	// Mutate the returned credentials
	originalUsername := creds1.Username
	creds1.Username = "modified-username"
	creds1.URLs[0] = "turn:modified.example.com:3478"

	// Get the cached credentials and verify they are unchanged
	impl.mu.RLock()
	cached := impl.credentials[participantID]
	impl.mu.RUnlock()

	if cached.Username != originalUsername {
		t.Error("Mutating returned credentials should not affect cached credentials")
	}
	if cached.URLs[0] != "turn:turn.example.com:3478" {
		t.Error("Mutating returned credentials URLs should not affect cached credentials")
	}

	// Test GetCredentials also returns a copy
	creds2 := impl.GetCredentials(participantID)
	if creds2 == nil {
		t.Fatal("GetCredentials() returned nil")
	}

	// Mutate the credentials from GetCredentials
	creds2.Username = "another-modified-username"
	creds2.URLs[0] = "turn:another-modified.example.com:3478"

	// Verify cached credentials are still unchanged
	impl.mu.RLock()
	cached2 := impl.credentials[participantID]
	impl.mu.RUnlock()

	if cached2.Username != originalUsername {
		t.Error("Mutating GetCredentials result should not affect cached credentials")
	}
	if cached2.URLs[0] != "turn:turn.example.com:3478" {
		t.Error("Mutating GetCredentials result URLs should not affect cached credentials")
	}

	// Test RefreshCredentials also returns a copy (no rotation case)
	creds3, err := service.RefreshCredentials(participantID)
	if err != nil {
		t.Fatalf("RefreshCredentials() error = %v", err)
	}

	// Mutate the credentials from RefreshCredentials
	creds3.Username = "refresh-modified-username"
	creds3.URLs[0] = "turn:refresh-modified.example.com:3478"

	// Verify cached credentials are still unchanged
	impl.mu.RLock()
	cached3 := impl.credentials[participantID]
	impl.mu.RUnlock()

	if cached3.Username != originalUsername {
		t.Error("Mutating RefreshCredentials result (no rotation) should not affect cached credentials")
	}
	if cached3.URLs[0] != "turn:turn.example.com:3478" {
		t.Error("Mutating RefreshCredentials result URLs (no rotation) should not affect cached credentials")
	}
}

// Benchmark tests
func BenchmarkGenerateCredentials(b *testing.B) {
	config := TURNConfig{
		URLs:             []string{"turn:turn.example.com:3478"},
		Secret:           "test-secret",
		CredentialTTL:    24 * time.Hour,
		RotationInterval: 12 * time.Hour,
	}
	service, _ := NewTURNCredentialService(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.GenerateCredentials("user-123")
	}
}

func BenchmarkRefreshCredentials(b *testing.B) {
	config := TURNConfig{
		URLs:             []string{"turn:turn.example.com:3478"},
		Secret:           "test-secret",
		CredentialTTL:    24 * time.Hour,
		RotationInterval: 12 * time.Hour,
	}
	service, _ := NewTURNCredentialService(config)
	_, _ = service.GenerateCredentials("user-123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.RefreshCredentials("user-123")
	}
}
