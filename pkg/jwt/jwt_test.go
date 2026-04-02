package pkgjwt

import (
	"context"
	"testing"
	"time"

	"github.com/possibities/gin-core/pkg/cache"
	"github.com/possibities/gin-core/pkg/config"
	pkgerrors "github.com/possibities/gin-core/pkg/errors"
)

type memoryBlacklistStore struct {
	entries map[string]time.Time
}

func (s *memoryBlacklistStore) BlacklistToken(_ context.Context, jti string, ttl time.Duration) error {
	if s.entries == nil {
		s.entries = make(map[string]time.Time)
	}
	s.entries[jti] = time.Now().Add(ttl)
	return nil
}

func (s *memoryBlacklistStore) IsBlacklisted(_ context.Context, jti string) (bool, error) {
	expiresAt, ok := s.entries[jti]
	return ok && expiresAt.After(time.Now()), nil
}

type memoryRefreshStore struct {
	entries map[string]struct{}
	keys    *cache.Keyspace
}

func (s *memoryRefreshStore) StoreRefreshToken(_ context.Context, userID uint, jti string, _ time.Duration) error {
	if s.entries == nil {
		s.entries = make(map[string]struct{})
	}
	s.entries[s.keys.RefreshToken(userID, jti)] = struct{}{}
	return nil
}

func (s *memoryRefreshStore) HasRefreshToken(_ context.Context, userID uint, jti string) (bool, error) {
	_, ok := s.entries[s.keys.RefreshToken(userID, jti)]
	return ok, nil
}

func (s *memoryRefreshStore) DeleteRefreshToken(_ context.Context, userID uint, jti string) error {
	delete(s.entries, s.keys.RefreshToken(userID, jti))
	return nil
}

func TestManagerGeneratesAndAuthenticatesAccessToken(t *testing.T) {
	manager := newTestManager()

	pair, err := manager.GenerateTokenPair(context.Background(), Subject{
		UserID:   42,
		Role:     "admin",
		TenantID: "tenant-a",
	})
	if err != nil {
		t.Fatalf("GenerateTokenPair() error = %v", err)
	}

	claims, err := manager.AuthenticateAccessToken(context.Background(), pair.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticateAccessToken() error = %v", err)
	}
	if claims.UserID != 42 || claims.Role != "admin" || claims.TenantID != "tenant-a" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestManagerRejectsBlacklistedAccessToken(t *testing.T) {
	manager := newTestManager()

	pair, err := manager.GenerateTokenPair(context.Background(), Subject{UserID: 1, Role: "member"})
	if err != nil {
		t.Fatalf("GenerateTokenPair() error = %v", err)
	}
	if err := manager.BlacklistAccessToken(context.Background(), pair.AccessToken); err != nil {
		t.Fatalf("BlacklistAccessToken() error = %v", err)
	}

	_, err = manager.AuthenticateAccessToken(context.Background(), pair.AccessToken)
	if err != pkgerrors.ErrTokenBlacklisted {
		t.Fatalf("expected ErrTokenBlacklisted, got %v", err)
	}
}

func TestManagerValidatesStoredRefreshToken(t *testing.T) {
	manager := newTestManager()

	pair, err := manager.GenerateTokenPair(context.Background(), Subject{UserID: 7, Role: "member"})
	if err != nil {
		t.Fatalf("GenerateTokenPair() error = %v", err)
	}

	if _, err := manager.ValidateRefreshToken(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("ValidateRefreshToken() error = %v", err)
	}

	if err := manager.RevokeRefreshToken(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("RevokeRefreshToken() error = %v", err)
	}

	if _, err := manager.ValidateRefreshToken(context.Background(), pair.RefreshToken); err != pkgerrors.ErrTokenInvalid {
		t.Fatalf("expected ErrTokenInvalid after revoke, got %v", err)
	}
}

func TestManagerRejectsTokenWithUnexpectedAudience(t *testing.T) {
	manager := newTestManager()
	otherAudienceManager := NewManager(&config.Config{
		App: config.AppConfig{Name: "another-service"},
		JWT: config.JWTConfig{
			Issuer:                "gin-core",
			AccessTTLSec:          300,
			RefreshTTLSec:         600,
			AccessSecret:          "access-secret",
			RefreshSecret:         "refresh-secret",
			PreviousAccessSecret:  "",
			PreviousRefreshSecret: "",
		},
	}, &memoryBlacklistStore{}, &memoryRefreshStore{
		keys: cache.NewKeyspace(&config.Config{App: config.AppConfig{Name: "another-service"}}),
	})

	pair, err := otherAudienceManager.GenerateTokenPair(context.Background(), Subject{
		UserID: 1,
		Role:   "admin",
	})
	if err != nil {
		t.Fatalf("GenerateTokenPair() error = %v", err)
	}

	if _, err := manager.AuthenticateAccessToken(context.Background(), pair.AccessToken); err != pkgerrors.ErrTokenInvalid {
		t.Fatalf("expected ErrTokenInvalid for audience mismatch, got %v", err)
	}
}

func TestManagerAcceptsPreviousSecretsDuringRotation(t *testing.T) {
	rotatedCfg := &config.Config{
		App: config.AppConfig{Name: "gin-core"},
		JWT: config.JWTConfig{
			Issuer:                "gin-core",
			AccessTTLSec:          300,
			RefreshTTLSec:         600,
			AccessSecret:          "new-access-secret",
			RefreshSecret:         "new-refresh-secret",
			PreviousAccessSecret:  "old-access-secret",
			PreviousRefreshSecret: "old-refresh-secret",
		},
	}
	legacyManager := NewManager(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
		JWT: config.JWTConfig{
			Issuer:        "gin-core",
			AccessTTLSec:  300,
			RefreshTTLSec: 600,
			AccessSecret:  "old-access-secret",
			RefreshSecret: "old-refresh-secret",
		},
	}, &memoryBlacklistStore{}, &memoryRefreshStore{keys: cache.NewKeyspace(rotatedCfg)})
	manager := NewManager(rotatedCfg, &memoryBlacklistStore{}, &memoryRefreshStore{keys: cache.NewKeyspace(rotatedCfg)})

	legacyPair, err := legacyManager.GenerateTokenPair(context.Background(), Subject{UserID: 9, Role: "member"})
	if err != nil {
		t.Fatalf("GenerateTokenPair() legacy error = %v", err)
	}
	if _, err := manager.AuthenticateAccessToken(context.Background(), legacyPair.AccessToken); err != nil {
		t.Fatalf("AuthenticateAccessToken() with previous secret error = %v", err)
	}

	refreshClaims, err := legacyManager.ValidateRefreshToken(context.Background(), legacyPair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken() legacy error = %v", err)
	}
	refreshStore := manager.refreshTokens.(*memoryRefreshStore)
	if err := refreshStore.StoreRefreshToken(context.Background(), refreshClaims.UserID, refreshClaims.ID, time.Minute); err != nil {
		t.Fatalf("StoreRefreshToken() error = %v", err)
	}
	if _, err := manager.ValidateRefreshToken(context.Background(), legacyPair.RefreshToken); err != nil {
		t.Fatalf("ValidateRefreshToken() with previous secret error = %v", err)
	}

	currentPair, err := manager.GenerateTokenPair(context.Background(), Subject{UserID: 10, Role: "admin"})
	if err != nil {
		t.Fatalf("GenerateTokenPair() rotated error = %v", err)
	}
	if _, err := legacyManager.AuthenticateAccessToken(context.Background(), currentPair.AccessToken); err != pkgerrors.ErrTokenInvalid {
		t.Fatalf("expected legacy manager to reject new access token, got %v", err)
	}
}

func newTestManager() *Manager {
	cfg := &config.Config{
		App: config.AppConfig{Name: "gin-core"},
		JWT: config.JWTConfig{
			Issuer:                "gin-core",
			AccessTTLSec:          300,
			RefreshTTLSec:         600,
			AccessSecret:          "access-secret",
			RefreshSecret:         "refresh-secret",
			PreviousAccessSecret:  "",
			PreviousRefreshSecret: "",
		},
	}
	return NewManager(cfg, &memoryBlacklistStore{}, &memoryRefreshStore{
		keys: cache.NewKeyspace(cfg),
	})
}
