package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func TestTokenGenerator_GenerateToken(t *testing.T) {
	privateKey := generateTestKey(t)

	cfg := TokenGeneratorConfig{
		Issuer:   "test-issuer",
		Audience: "test-audience",
		KeyID:    "test-key-id",
	}

	generator := NewTokenGenerator(cfg, privateKey)

	tokenString, err := generator.GenerateToken("room-123", "user-456", "publisher", 3600)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenString)

	// Parse and verify token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return &privateKey.PublicKey, nil
	})
	require.NoError(t, err)
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(*Claims)
	require.True(t, ok)

	assert.Equal(t, "user-456", claims.Subject)
	assert.Equal(t, "room-123", claims.RoomID)
	assert.Equal(t, "publisher", claims.Role)
	assert.Equal(t, "test-issuer", claims.Issuer)
	assert.Contains(t, claims.Audience, "test-audience")
	assert.NotNil(t, claims.IssuedAt)
	assert.NotNil(t, claims.ExpiresAt)
	assert.NotNil(t, claims.NotBefore)

	// Check that permissions are included
	assert.NotEmpty(t, claims.Permissions)

	// Check key ID in header
	assert.Equal(t, "test-key-id", token.Header["kid"])
}

func TestTokenGenerator_GenerateToken_AllRoles(t *testing.T) {
	privateKey := generateTestKey(t)
	generator := NewTokenGenerator(TokenGeneratorConfig{}, privateKey)

	roles := []string{"admin", "moderator", "publisher", "subscriber"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			tokenString, err := generator.GenerateToken("room-123", "user-456", role, 3600)
			require.NoError(t, err)
			assert.NotEmpty(t, tokenString)

			// Parse and verify token
			token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				return &privateKey.PublicKey, nil
			})
			require.NoError(t, err)
			assert.True(t, token.Valid)

			claims := token.Claims.(*Claims)
			assert.Equal(t, role, claims.Role)
		})
	}
}

func TestTokenGenerator_GenerateToken_InvalidRole(t *testing.T) {
	privateKey := generateTestKey(t)
	generator := NewTokenGenerator(TokenGeneratorConfig{}, privateKey)

	_, err := generator.GenerateToken("room-123", "user-456", "invalid-role", 3600)
	assert.ErrorIs(t, err, ErrInvalidRole)
}

func TestTokenGenerator_GenerateToken_NoPrivateKey(t *testing.T) {
	generator := NewTokenGenerator(TokenGeneratorConfig{}, nil)

	_, err := generator.GenerateToken("room-123", "user-456", "publisher", 3600)
	assert.ErrorIs(t, err, ErrPrivateKeyNotConfigured)
}

func TestTokenGenerator_GenerateToken_InvalidExpiresIn(t *testing.T) {
	privateKey := generateTestKey(t)
	generator := NewTokenGenerator(TokenGeneratorConfig{}, privateKey)

	testCases := []struct {
		name      string
		expiresIn int
	}{
		{"zero", 0},
		{"negative", -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := generator.GenerateToken("room-123", "user-456", "publisher", tc.expiresIn)
			assert.ErrorIs(t, err, ErrInvalidExpiresIn)
		})
	}
}

func TestTokenGenerator_GenerateToken_Expiration(t *testing.T) {
	privateKey := generateTestKey(t)
	generator := NewTokenGenerator(TokenGeneratorConfig{}, privateKey)

	expiresIn := 7200 // 2 hours
	tokenString, err := generator.GenerateToken("room-123", "user-456", "publisher", expiresIn)
	require.NoError(t, err)

	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return &privateKey.PublicKey, nil
	})
	require.NoError(t, err)

	claims := token.Claims.(*Claims)

	// Check expiration is approximately 2 hours from now
	expectedExpiry := time.Now().Add(time.Duration(expiresIn) * time.Second)
	assert.WithinDuration(t, expectedExpiry, claims.ExpiresAt.Time, 5*time.Second)
}

func TestTokenGenerator_GenerateAdminToken(t *testing.T) {
	privateKey := generateTestKey(t)

	cfg := TokenGeneratorConfig{
		Issuer:   "test-issuer",
		Audience: "test-audience",
		KeyID:    "test-key-id",
	}

	generator := NewTokenGenerator(cfg, privateKey)

	tokenString, err := generator.GenerateAdminToken("admin-user", 3600)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenString)

	// Parse and verify token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return &privateKey.PublicKey, nil
	})
	require.NoError(t, err)
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(*Claims)
	require.True(t, ok)

	assert.Equal(t, "admin-user", claims.Subject)
	assert.Empty(t, claims.RoomID) // Admin tokens have no room_id
	assert.Equal(t, "admin", claims.Role)
	assert.Equal(t, "test-issuer", claims.Issuer)
	assert.Contains(t, claims.Audience, "test-audience")

	// Check that admin permissions are included
	assert.NotEmpty(t, claims.Permissions)
}

func TestTokenGenerator_GenerateAdminToken_NoPrivateKey(t *testing.T) {
	generator := NewTokenGenerator(TokenGeneratorConfig{}, nil)

	_, err := generator.GenerateAdminToken("admin-user", 3600)
	assert.ErrorIs(t, err, ErrPrivateKeyNotConfigured)
}

func TestTokenGenerator_GenerateAdminToken_InvalidExpiresIn(t *testing.T) {
	privateKey := generateTestKey(t)
	generator := NewTokenGenerator(TokenGeneratorConfig{}, privateKey)

	_, err := generator.GenerateAdminToken("admin-user", 0)
	assert.ErrorIs(t, err, ErrInvalidExpiresIn)
}

func TestTokenGenerator_GenerateToken_NoOptionalConfig(t *testing.T) {
	privateKey := generateTestKey(t)

	// No issuer, audience, or key ID
	generator := NewTokenGenerator(TokenGeneratorConfig{}, privateKey)

	tokenString, err := generator.GenerateToken("room-123", "user-456", "publisher", 3600)
	require.NoError(t, err)

	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return &privateKey.PublicKey, nil
	})
	require.NoError(t, err)

	claims := token.Claims.(*Claims)

	// Optional fields should be empty/nil
	assert.Empty(t, claims.Issuer)
	assert.Empty(t, claims.Audience)
	assert.Nil(t, token.Header["kid"])
}

func TestTokenGenerator_GenerateToken_RoleNormalization(t *testing.T) {
	privateKey := generateTestKey(t)
	generator := NewTokenGenerator(TokenGeneratorConfig{}, privateKey)

	testCases := []struct {
		inputRole    string
		expectedRole string
	}{
		{"Publisher", "publisher"},
		{"ADMIN", "admin"},
		{"Moderator", "moderator"},
		{"SUBSCRIBER", "subscriber"},
		{"pUbLiShEr", "publisher"},
	}

	for _, tc := range testCases {
		t.Run(tc.inputRole, func(t *testing.T) {
			tokenString, err := generator.GenerateToken("room-123", "user-456", tc.inputRole, 3600)
			require.NoError(t, err)

			// Parse token
			token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
				return &privateKey.PublicKey, nil
			})
			require.NoError(t, err)

			claims := token.Claims.(*Claims)
			assert.Equal(t, tc.expectedRole, claims.Role, "Role should be normalized to lowercase")
		})
	}
}
