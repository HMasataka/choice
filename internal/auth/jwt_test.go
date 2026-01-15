package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}
	return privateKey, &privateKey.PublicKey
}

func TestJWTValidator_Validate(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	t.Run("valid token", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			RoomID: "room-456",
			Role:   "publisher",
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		result, err := validator.Validate(context.Background(), tokenString)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Subject != "user-123" {
			t.Errorf("expected subject 'user-123', got %q", result.Subject)
		}
		if result.RoomID != "room-456" {
			t.Errorf("expected room_id 'room-456', got %q", result.RoomID)
		}
		if result.Role != "publisher" {
			t.Errorf("expected role 'publisher', got %q", result.Role)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			},
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrTokenExpired) {
			t.Errorf("expected ErrTokenExpired, got %v", err)
		}
	})

	t.Run("token not yet valid", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(time.Hour)),
				NotBefore: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(2 * time.Hour)),
			},
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrTokenNotYetValid) {
			t.Errorf("expected ErrTokenNotYetValid, got %v", err)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		// Use a different key for validation
		_, differentPublicKey := generateTestKeyPair(t)
		keySource := NewStaticKeySource(differentPublicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("expected ErrInvalidSignature, got %v", err)
		}
	})

	t.Run("malformed token", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		_, err := validator.Validate(context.Background(), "not.a.valid.token")
		if !errors.Is(err, ErrInvalidToken) {
			t.Errorf("expected ErrInvalidToken, got %v", err)
		}
	})

	t.Run("missing subject claim", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrMissingClaim) {
			t.Errorf("expected ErrMissingClaim, got %v", err)
		}
	})

	t.Run("missing iat claim", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrMissingClaim) {
			t.Errorf("expected ErrMissingClaim for iat, got %v", err)
		}
	})

	t.Run("missing exp claim", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:  "user-123",
				IssuedAt: jwt.NewNumericDate(time.Now()),
			},
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrMissingClaim) {
			t.Errorf("expected ErrMissingClaim for exp, got %v", err)
		}
	})

	t.Run("rejects RS384 algorithm", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		// Generate token with RS384 instead of RS256
		token := jwt.NewWithClaims(jwt.SigningMethodRS384, claims)
		tokenString, err := token.SignedString(privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("expected ErrInvalidSignature for RS384, got %v", err)
		}
	})

	t.Run("rejects HS256 algorithm", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		// Generate token with HS256 (symmetric algorithm)
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := token.SignedString([]byte("secret"))
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("expected ErrInvalidSignature for HS256, got %v", err)
		}
	})

	t.Run("validates issuer", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		cfg := JWTConfig{
			Issuer: "expected-issuer",
		}
		validator := NewJWTValidator(cfg, keySource)

		// Token with wrong issuer
		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				Issuer:    "wrong-issuer",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrInvalidIssuer) {
			t.Errorf("expected ErrInvalidIssuer, got %v", err)
		}

		// Token with correct issuer
		claims.Issuer = "expected-issuer"
		tokenString, err = GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("validates audience", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		cfg := JWTConfig{
			Audience: "expected-audience",
		}
		validator := NewJWTValidator(cfg, keySource)

		// Token with wrong audience
		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				Audience:  jwt.ClaimStrings{"wrong-audience"},
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if !errors.Is(err, ErrInvalidAudience) {
			t.Errorf("expected ErrInvalidAudience, got %v", err)
		}

		// Token with correct audience
		claims.Audience = jwt.ClaimStrings{"expected-audience"}
		tokenString, err = GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("clock skew tolerance", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		cfg := JWTConfig{
			ClockSkew: time.Minute,
		}
		validator := NewJWTValidator(cfg, keySource)

		// Token that expired 30 seconds ago (within clock skew)
		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-30 * time.Second)),
			},
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.Validate(context.Background(), tokenString)
		if err != nil {
			t.Errorf("expected no error with clock skew, got %v", err)
		}
	})
}

func TestJWTValidator_ValidateForRoom(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	t.Run("valid token for room", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			RoomID: "room-456",
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		result, err := validator.ValidateForRoom(context.Background(), tokenString, "room-456")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.RoomID != "room-456" {
			t.Errorf("expected room_id 'room-456', got %q", result.RoomID)
		}
	})

	t.Run("token with wrong room_id", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			RoomID: "room-456",
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.ValidateForRoom(context.Background(), tokenString, "room-789")
		if !errors.Is(err, ErrInvalidRoomID) {
			t.Errorf("expected ErrInvalidRoomID, got %v", err)
		}
	})

	t.Run("token without room_id allows any room", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			// No RoomID set
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.ValidateForRoom(context.Background(), tokenString, "any-room")
		if err != nil {
			t.Errorf("expected no error for token without room_id, got %v", err)
		}
	})
}

func TestJWTValidator_ValidateForRoomStrict(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	t.Run("valid token with matching room_id", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			RoomID: "room-456",
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		result, err := validator.ValidateForRoomStrict(context.Background(), tokenString, "room-456")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.RoomID != "room-456" {
			t.Errorf("expected room_id 'room-456', got %q", result.RoomID)
		}
	})

	t.Run("rejects token without room_id", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			// No RoomID set
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.ValidateForRoomStrict(context.Background(), tokenString, "any-room")
		if !errors.Is(err, ErrMissingClaim) {
			t.Errorf("expected ErrMissingClaim for missing room_id, got %v", err)
		}
	})

	t.Run("rejects token with wrong room_id", func(t *testing.T) {
		keySource := NewStaticKeySource(publicKey)
		validator := NewJWTValidator(DefaultJWTConfig(), keySource)

		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "user-123",
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			RoomID: "room-456",
		}

		tokenString, err := GenerateToken(claims, privateKey)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = validator.ValidateForRoomStrict(context.Background(), tokenString, "room-789")
		if !errors.Is(err, ErrInvalidRoomID) {
			t.Errorf("expected ErrInvalidRoomID, got %v", err)
		}
	})
}

func TestGenerateTokenWithKID(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	keySource := NewStaticKeySource(publicKey)
	validator := NewJWTValidator(DefaultJWTConfig(), keySource)

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	tokenString, err := GenerateTokenWithKID(claims, privateKey, "my-key-id")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// Parse to check the kid header
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &Claims{})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}

	kid, ok := token.Header["kid"].(string)
	if !ok || kid != "my-key-id" {
		t.Errorf("expected kid 'my-key-id', got %q", kid)
	}

	// Should still validate
	_, err = validator.Validate(context.Background(), tokenString)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStaticKeySource(t *testing.T) {
	t.Run("returns configured key", func(t *testing.T) {
		_, publicKey := generateTestKeyPair(t)
		keySource := NewStaticKeySource(publicKey)

		key, err := keySource.GetKey(context.Background(), "any-kid")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if key != publicKey {
			t.Error("expected same key to be returned")
		}
	})

	t.Run("returns error for nil key", func(t *testing.T) {
		keySource := NewStaticKeySource(nil)

		_, err := keySource.GetKey(context.Background(), "any-kid")
		if err == nil {
			t.Error("expected error for nil key")
		}
	})
}

func TestDefaultJWTConfig(t *testing.T) {
	cfg := DefaultJWTConfig()

	if cfg.Issuer != "" {
		t.Errorf("expected empty issuer, got %q", cfg.Issuer)
	}
	if cfg.Audience != "" {
		t.Errorf("expected empty audience, got %q", cfg.Audience)
	}
	if cfg.ClockSkew != 30*time.Second {
		t.Errorf("expected 30s clock skew, got %v", cfg.ClockSkew)
	}
}
