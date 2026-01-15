package auth

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Common errors for JWT validation.
var (
	ErrTokenExpired     = errors.New("token has expired")
	ErrTokenNotYetValid = errors.New("token is not yet valid")
	ErrInvalidToken     = errors.New("invalid token")
	ErrInvalidSignature = errors.New("invalid token signature")
	ErrInvalidClaims    = errors.New("invalid token claims")
	ErrMissingClaim     = errors.New("missing required claim")
	ErrInvalidIssuer    = errors.New("invalid token issuer")
	ErrInvalidAudience  = errors.New("invalid token audience")
	ErrInvalidRoomID    = errors.New("invalid room_id claim")
)

// Claims represents the JWT claims for SFU authentication.
type Claims struct {
	jwt.RegisteredClaims
	RoomID      string   `json:"room_id,omitempty"`
	Role        string   `json:"role,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// JWTConfig contains configuration for JWT validation.
type JWTConfig struct {
	// Issuer is the expected issuer of the token.
	Issuer string
	// Audience is the expected audience of the token.
	Audience string
	// ClockSkew is the allowed clock skew for token validation.
	ClockSkew time.Duration
}

// DefaultJWTConfig returns the default JWT configuration.
func DefaultJWTConfig() JWTConfig {
	return JWTConfig{
		Issuer:    "",
		Audience:  "",
		ClockSkew: 30 * time.Second,
	}
}

// JWTValidator validates JWT tokens.
type JWTValidator struct {
	config    JWTConfig
	keySource KeySource
}

// KeySource provides public keys for JWT validation.
type KeySource interface {
	// GetKey returns the public key for the given key ID.
	// If kid is empty, returns the default key.
	GetKey(ctx context.Context, kid string) (*rsa.PublicKey, error)
}

// NewJWTValidator creates a new JWT validator.
func NewJWTValidator(cfg JWTConfig, keySource KeySource) *JWTValidator {
	return &JWTValidator{
		config:    cfg,
		keySource: keySource,
	}
}

// Validate validates the given JWT token and returns the claims.
func (v *JWTValidator) Validate(ctx context.Context, tokenString string) (*Claims, error) {
	// Parse the token header to get the key ID
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method is strictly RS256 (not RS384/RS512)
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("%w: expected RS256, got %v", ErrInvalidSignature, token.Header["alg"])
		}

		// Get the key ID from the token header
		kid, _ := token.Header["kid"].(string) //nolint:errcheck // Empty kid is valid

		// Get the public key from the key source
		key, err := v.keySource.GetKey(ctx, kid)
		if err != nil {
			return nil, fmt.Errorf("failed to get key: %w", err)
		}

		return key, nil
	}, jwt.WithLeeway(v.config.ClockSkew))

	if err != nil {
		return nil, v.translateError(err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidClaims
	}

	// Validate issuer if configured
	if v.config.Issuer != "" {
		if claims.Issuer != v.config.Issuer {
			return nil, ErrInvalidIssuer
		}
	}

	// Validate audience if configured
	if v.config.Audience != "" {
		found := false
		for _, aud := range claims.Audience {
			if aud == v.config.Audience {
				found = true
				break
			}
		}
		if !found {
			return nil, ErrInvalidAudience
		}
	}

	// Validate required claims
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: sub", ErrMissingClaim)
	}
	if claims.IssuedAt == nil {
		return nil, fmt.Errorf("%w: iat", ErrMissingClaim)
	}
	if claims.ExpiresAt == nil {
		return nil, fmt.Errorf("%w: exp", ErrMissingClaim)
	}

	return claims, nil
}

// ValidateForRoom validates the token and ensures it's valid for the given room.
// If the token has a room_id claim, it must match the given roomID.
// Tokens without a room_id claim are considered "admin" tokens and can access any room.
// Use ValidateForRoomStrict if room_id must be present and match.
func (v *JWTValidator) ValidateForRoom(ctx context.Context, tokenString string, roomID string) (*Claims, error) {
	claims, err := v.Validate(ctx, tokenString)
	if err != nil {
		return nil, err
	}

	// Check room_id claim if present (tokens without room_id can access any room)
	if claims.RoomID != "" && claims.RoomID != roomID {
		return nil, ErrInvalidRoomID
	}

	return claims, nil
}

// ValidateForRoomStrict validates the token and requires a matching room_id claim.
// This should be used for regular participants who must have room-scoped tokens.
func (v *JWTValidator) ValidateForRoomStrict(ctx context.Context, tokenString string, roomID string) (*Claims, error) {
	claims, err := v.Validate(ctx, tokenString)
	if err != nil {
		return nil, err
	}

	// Require room_id claim to be present and match
	if claims.RoomID == "" {
		return nil, fmt.Errorf("%w: room_id", ErrMissingClaim)
	}
	if claims.RoomID != roomID {
		return nil, ErrInvalidRoomID
	}

	return claims, nil
}

// translateError converts JWT library errors to our error types.
func (v *JWTValidator) translateError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, jwt.ErrTokenExpired) {
		return ErrTokenExpired
	}
	if errors.Is(err, jwt.ErrTokenNotValidYet) {
		return ErrTokenNotYetValid
	}
	if errors.Is(err, jwt.ErrTokenMalformed) {
		return ErrInvalidToken
	}
	if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
		return ErrInvalidSignature
	}
	if errors.Is(err, jwt.ErrSignatureInvalid) {
		return ErrInvalidSignature
	}

	// Check if the error message contains signature-related text
	errMsg := err.Error()
	if contains(errMsg, "signature") || contains(errMsg, "signing") {
		return fmt.Errorf("%w: %w", ErrInvalidSignature, err)
	}

	return fmt.Errorf("%w: %w", ErrInvalidToken, err)
}

// contains checks if s contains substr (simple implementation to avoid strings import).
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// StaticKeySource provides a static public key for testing.
type StaticKeySource struct {
	key *rsa.PublicKey
}

// NewStaticKeySource creates a new static key source.
func NewStaticKeySource(key *rsa.PublicKey) *StaticKeySource {
	return &StaticKeySource{key: key}
}

// GetKey returns the static public key.
func (s *StaticKeySource) GetKey(_ context.Context, _ string) (*rsa.PublicKey, error) {
	if s.key == nil {
		return nil, errors.New("no key configured")
	}
	return s.key, nil
}

// GenerateToken generates a JWT token for testing purposes.
// This should only be used in tests.
func GenerateToken(claims *Claims, privateKey *rsa.PrivateKey) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(privateKey)
}

// GenerateTokenWithKID generates a JWT token with a key ID for testing purposes.
func GenerateTokenWithKID(claims *Claims, privateKey *rsa.PrivateKey, kid string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	return token.SignedString(privateKey)
}
