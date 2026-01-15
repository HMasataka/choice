package auth

import (
	"crypto/rsa"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Common errors for token generation.
var (
	ErrPrivateKeyNotConfigured = errors.New("private key not configured")
	ErrInvalidExpiresIn        = errors.New("invalid expires_in value")
)

// TokenGeneratorConfig contains configuration for token generation.
type TokenGeneratorConfig struct {
	// Issuer is the issuer claim for generated tokens.
	Issuer string
	// Audience is the audience claim for generated tokens.
	Audience string
	// KeyID is the key ID to include in the token header.
	KeyID string
}

// TokenGenerator generates JWT tokens for room access.
type TokenGenerator struct {
	config     TokenGeneratorConfig
	privateKey *rsa.PrivateKey
	checker    *PermissionChecker
}

// NewTokenGenerator creates a new token generator.
func NewTokenGenerator(cfg TokenGeneratorConfig, privateKey *rsa.PrivateKey) *TokenGenerator {
	return &TokenGenerator{
		config:     cfg,
		privateKey: privateKey,
		checker:    NewPermissionChecker(),
	}
}

// GenerateToken generates a JWT token for the given room and participant.
func (g *TokenGenerator) GenerateToken(roomID, participantID, role string, expiresInSeconds int) (string, error) {
	if g.privateKey == nil {
		return "", ErrPrivateKeyNotConfigured
	}

	if expiresInSeconds <= 0 {
		return "", ErrInvalidExpiresIn
	}

	// Validate role
	parsedRole, err := ParseRole(role)
	if err != nil {
		return "", err
	}

	// Get permissions for the role
	permissions := g.checker.GetRolePermissions(parsedRole)
	permStrings := make([]string, len(permissions))
	for i, p := range permissions {
		permStrings[i] = string(p)
	}

	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   participantID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expiresInSeconds) * time.Second)),
			NotBefore: jwt.NewNumericDate(now),
		},
		RoomID:      roomID,
		Role:        string(parsedRole), // Use normalized (lowercased) role
		Permissions: permStrings,
	}

	// Set issuer if configured
	if g.config.Issuer != "" {
		claims.Issuer = g.config.Issuer
	}

	// Set audience if configured
	if g.config.Audience != "" {
		claims.Audience = jwt.ClaimStrings{g.config.Audience}
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	// Set key ID if configured
	if g.config.KeyID != "" {
		token.Header["kid"] = g.config.KeyID
	}

	// Sign and return
	return token.SignedString(g.privateKey)
}

// GenerateAdminToken generates a JWT token without room_id restriction.
// Admin tokens can access any room.
func (g *TokenGenerator) GenerateAdminToken(participantID string, expiresInSeconds int) (string, error) {
	if g.privateKey == nil {
		return "", ErrPrivateKeyNotConfigured
	}

	if expiresInSeconds <= 0 {
		return "", ErrInvalidExpiresIn
	}

	// Get admin permissions
	permissions := g.checker.GetRolePermissions(RoleAdmin)
	permStrings := make([]string, len(permissions))
	for i, p := range permissions {
		permStrings[i] = string(p)
	}

	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   participantID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expiresInSeconds) * time.Second)),
			NotBefore: jwt.NewNumericDate(now),
		},
		// No RoomID - allows access to any room
		Role:        string(RoleAdmin),
		Permissions: permStrings,
	}

	// Set issuer if configured
	if g.config.Issuer != "" {
		claims.Issuer = g.config.Issuer
	}

	// Set audience if configured
	if g.config.Audience != "" {
		claims.Audience = jwt.ClaimStrings{g.config.Audience}
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	// Set key ID if configured
	if g.config.KeyID != "" {
		token.Header["kid"] = g.config.KeyID
	}

	// Sign and return
	return token.SignedString(g.privateKey)
}
