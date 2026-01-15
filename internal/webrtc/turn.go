package webrtc

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrEmptySecret is returned when the TURN secret is empty.
	ErrEmptySecret = errors.New("TURN secret cannot be empty")
	// ErrInvalidTTL is returned when the TTL is invalid.
	ErrInvalidTTL = errors.New("TURN credential TTL must be positive")
	// ErrEmptyUsername is returned when the username is empty.
	ErrEmptyUsername = errors.New("username cannot be empty")
	// ErrEmptyTURNURLs is returned when no TURN URLs are provided.
	ErrEmptyTURNURLs = errors.New("TURN URLs cannot be empty")
)

// Default TURN credential parameters per requirements.md (section 2.7.2) and design.md (section 6.6).
const (
	// DefaultCredentialTTL is the default validity period for TURN credentials (24 hours).
	DefaultCredentialTTL = 24 * time.Hour
	// DefaultRotationInterval is the default interval for credential rotation (12 hours).
	DefaultRotationInterval = 12 * time.Hour
)

// TURNCredentials represents time-limited TURN credentials.
// Per requirements.md (section 2.7.2), credentials use TURN REST API method
// (RFC 5389 Long-term authentication mechanism with HMAC-SHA1 dynamic credential generation).
type TURNCredentials struct {
	// URLs are the TURN server URLs (e.g., "turn:turn.example.com:3478").
	URLs []string
	// Username is the time-limited username in the format "timestamp:username".
	// The timestamp is the Unix timestamp of expiration time.
	Username string
	// Credential is the HMAC-SHA1 signature of the username, base64-encoded.
	// Calculated as base64(HMAC-SHA1(secret, username)).
	Credential string
	// CreatedAt is the time when the credentials were created.
	CreatedAt time.Time
	// ExpiresAt is the expiration time of the credentials.
	ExpiresAt time.Time
}

// IsExpired checks if the credentials have expired.
func (c *TURNCredentials) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// TimeToExpiry returns the duration until expiration.
// Returns 0 if already expired.
func (c *TURNCredentials) TimeToExpiry() time.Duration {
	remaining := time.Until(c.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// NeedsRotation checks if the credentials should be rotated.
// Per requirements.md (section 2.7.2), credentials should be rotated every 12 hours.
// This checks if the elapsed time since creation has exceeded the rotation interval.
func (c *TURNCredentials) NeedsRotation(rotationInterval time.Duration) bool {
	elapsed := time.Since(c.CreatedAt)
	return elapsed >= rotationInterval
}

// TURNCredentialService generates time-limited TURN credentials.
// Per requirements.md (section 2.7.2), this implements Long-term credentials (RFC 5389).
type TURNCredentialService interface {
	// GenerateCredentials creates new TURN credentials for a participant.
	// The credentials are valid for the configured TTL (default 24 hours).
	GenerateCredentials(participantID string) (*TURNCredentials, error)

	// RefreshCredentials generates new credentials before expiry.
	// This should be called periodically (default every 12 hours).
	RefreshCredentials(participantID string) (*TURNCredentials, error)
}

// TURNConfig contains TURN server configuration.
type TURNConfig struct {
	// URLs are the TURN server URLs.
	// Supports multiple URLs for fallback per requirements.md (section 2.7.1).
	URLs []string

	// Secret is the shared secret for HMAC-SHA1 credential generation.
	// This must be kept secure and synchronized with the TURN server.
	Secret string

	// CredentialTTL is the validity period for credentials.
	// Per requirements.md (section 2.7.2), default is 24 hours.
	CredentialTTL time.Duration

	// RotationInterval is the interval for credential rotation.
	// Per requirements.md (section 2.7.2), default is 12 hours.
	RotationInterval time.Duration
}

// Validate validates the TURN configuration.
func (c *TURNConfig) Validate() error {
	if len(c.URLs) == 0 {
		return ErrEmptyTURNURLs
	}
	if c.Secret == "" {
		return ErrEmptySecret
	}
	if c.CredentialTTL <= 0 {
		return ErrInvalidTTL
	}
	if c.RotationInterval <= 0 {
		return ErrInvalidTTL
	}
	if c.RotationInterval >= c.CredentialTTL {
		return errors.New("rotation interval must be less than credential TTL")
	}
	return nil
}

// DefaultTURNConfig returns the default TURN configuration.
func DefaultTURNConfig() TURNConfig {
	return TURNConfig{
		CredentialTTL:    DefaultCredentialTTL,
		RotationInterval: DefaultRotationInterval,
	}
}

// turnCredentialService is the concrete implementation of TURNCredentialService.
type turnCredentialService struct {
	config TURNConfig
	mu     sync.RWMutex
	// credentials stores the current credentials per participant.
	credentials map[string]*TURNCredentials
}

// NewTURNCredentialService creates a new TURN credential service.
// Per requirements.md (section 2.7.2), this implements Long-term credentials (RFC 5389).
func NewTURNCredentialService(config TURNConfig) (TURNCredentialService, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &turnCredentialService{
		config:      config,
		credentials: make(map[string]*TURNCredentials),
	}, nil
}

// GenerateCredentials creates new TURN credentials for a participant.
// The username format is "timestamp:username" and the credential
// is HMAC-SHA1(secret, username) base64-encoded (TURN REST API method).
func (s *turnCredentialService) GenerateCredentials(participantID string) (*TURNCredentials, error) {
	if participantID == "" {
		return nil, ErrEmptyUsername
	}

	// Validate participantID: must not contain colon as it conflicts with username format
	if containsColon(participantID) {
		return nil, errors.New("participantID must not contain colon character")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate timestamp-based username (TURN REST API format)
	now := time.Now()
	expiresAt := now.Add(s.config.CredentialTTL)
	timestamp := expiresAt.Unix()
	username := fmt.Sprintf("%d:%s", timestamp, participantID)

	// Calculate HMAC-SHA1 credential (TURN REST API method)
	credential := s.generateHMAC(username)

	creds := &TURNCredentials{
		URLs:       make([]string, len(s.config.URLs)),
		Username:   username,
		Credential: credential,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
	}
	copy(creds.URLs, s.config.URLs)

	// Store credentials for rotation tracking
	s.credentials[participantID] = creds

	// Return a copy to prevent concurrent modification
	credsCopy := *creds
	credsCopy.URLs = make([]string, len(creds.URLs))
	copy(credsCopy.URLs, creds.URLs)
	return &credsCopy, nil
}

// RefreshCredentials generates new credentials before expiry.
// Per requirements.md (section 2.7.2), credentials should be rotated every 12 hours.
func (s *turnCredentialService) RefreshCredentials(participantID string) (*TURNCredentials, error) {
	if participantID == "" {
		return nil, ErrEmptyUsername
	}

	s.mu.RLock()
	existing := s.credentials[participantID]
	s.mu.RUnlock()

	// Check if rotation is needed
	if existing != nil && !existing.NeedsRotation(s.config.RotationInterval) {
		// Return a copy to prevent concurrent modification
		credsCopy := *existing
		credsCopy.URLs = make([]string, len(existing.URLs))
		copy(credsCopy.URLs, existing.URLs)
		return &credsCopy, nil
	}

	// Generate new credentials
	return s.GenerateCredentials(participantID)
}

// generateHMAC calculates HMAC-SHA1 and returns base64-encoded result.
// TURN REST API method: credential = base64(HMAC-SHA1(secret, username)).
func (s *turnCredentialService) generateHMAC(username string) string {
	h := hmac.New(sha1.New, []byte(s.config.Secret))
	h.Write([]byte(username))
	signature := h.Sum(nil)
	return base64.StdEncoding.EncodeToString(signature)
}

// CleanupExpiredCredentials removes expired credentials from the cache.
// This should be called periodically to prevent memory leaks.
func (s *turnCredentialService) CleanupExpiredCredentials() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for participantID, creds := range s.credentials {
		if now.After(creds.ExpiresAt) {
			delete(s.credentials, participantID)
		}
	}
}

// GetCredentials retrieves cached credentials for a participant.
// Returns nil if no credentials exist or if they have expired.
// Returns a copy of the credentials to prevent concurrent modification issues.
func (s *turnCredentialService) GetCredentials(participantID string) *TURNCredentials {
	s.mu.RLock()
	defer s.mu.RUnlock()

	creds, exists := s.credentials[participantID]
	if !exists || creds.IsExpired() {
		return nil
	}

	// Return a copy to prevent concurrent modification
	credsCopy := *creds
	credsCopy.URLs = make([]string, len(creds.URLs))
	copy(credsCopy.URLs, creds.URLs)
	return &credsCopy
}

// StartAutoRotation starts a background goroutine that automatically rotates
// credentials for all participants at the specified interval.
// Returns a channel that can be closed to stop the rotation.
func (s *turnCredentialService) StartAutoRotation() chan<- struct{} {
	stopCh := make(chan struct{})
	ticker := time.NewTicker(s.config.RotationInterval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.rotateAllCredentials()
			case <-stopCh:
				return
			}
		}
	}()

	return stopCh
}

// rotateAllCredentials rotates credentials for all participants that need rotation.
func (s *turnCredentialService) rotateAllCredentials() {
	s.mu.RLock()
	participantIDs := make([]string, 0, len(s.credentials))
	for id := range s.credentials {
		participantIDs = append(participantIDs, id)
	}
	s.mu.RUnlock()

	for _, id := range participantIDs {
		// RefreshCredentials will check if rotation is needed
		_, _ = s.RefreshCredentials(id)
	}

	// Clean up expired credentials
	s.CleanupExpiredCredentials()
}

// TURNProtocol represents the TURN transport protocol.
// Per requirements.md (section 2.7.2), supports UDP, TCP, and TLS.
type TURNProtocol string

const (
	// TURNProtocolUDP uses UDP transport (standard TURN port 3478).
	TURNProtocolUDP TURNProtocol = "udp"
	// TURNProtocolTCP uses TCP transport (standard TURN port 3478).
	TURNProtocolTCP TURNProtocol = "tcp"
	// TURNProtocolTLS uses TLS over TCP (standard port 443 for firewall traversal).
	TURNProtocolTLS TURNProtocol = "tls"
)

// BuildTURNURLs constructs TURN URLs with the specified protocols.
// Example: BuildTURNURLs("turn.example.com", 3478, TURNProtocolUDP, TURNProtocolTCP)
// Returns: ["turn:turn.example.com:3478?transport=udp", "turn:turn.example.com:3478?transport=tcp"]
func BuildTURNURLs(host string, port int, protocols ...TURNProtocol) []string {
	urls := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		var url string
		if protocol == TURNProtocolTLS {
			// TLS uses turns:// scheme
			url = fmt.Sprintf("turns:%s:%d?transport=tcp", host, port)
		} else {
			url = fmt.Sprintf("turn:%s:%d?transport=%s", host, port, protocol)
		}
		urls = append(urls, url)
	}
	return urls
}

// MergeTURNCredentials combines TURN credentials with existing ICE servers.
// This is useful when adding TURN credentials to a list of STUN servers.
func MergeTURNCredentials(stunServers []ICEServerConfig, turnCreds *TURNCredentials) []ICEServerConfig {
	if turnCreds == nil {
		return stunServers
	}

	servers := make([]ICEServerConfig, len(stunServers))
	copy(servers, stunServers)

	servers = append(servers, ICEServerConfig{
		URLs:       turnCreds.URLs,
		Username:   turnCreds.Username,
		Credential: turnCreds.Credential,
	})

	return servers
}

// containsColon checks if a string contains a colon character.
// This is used to validate participantID to prevent conflicts with
// the "timestamp:participantID" username format.
func containsColon(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return true
		}
	}
	return false
}
