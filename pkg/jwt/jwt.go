package pkgjwt

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/possibities/gin-boilerplate/pkg/config"
	pkgerrors "github.com/possibities/gin-boilerplate/pkg/errors"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type Subject struct {
	UserID   uint
	Role     string
	TenantID string
}

type Claims struct {
	UserID    uint   `json:"user_id"`
	Role      string `json:"role"`
	TenantID  string `json:"tenant_id"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type BlacklistStore interface {
	BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error
	IsBlacklisted(ctx context.Context, jti string) (bool, error)
}

type RefreshTokenStore interface {
	StoreRefreshToken(ctx context.Context, userID uint, jti string, ttl time.Duration) error
	HasRefreshToken(ctx context.Context, userID uint, jti string) (bool, error)
	DeleteRefreshToken(ctx context.Context, userID uint, jti string) error
}

type Manager struct {
	cfg           *config.Config
	blacklist     BlacklistStore
	refreshTokens RefreshTokenStore
	now           func() time.Time
}

func NewManager(cfg *config.Config, blacklist BlacklistStore, refreshTokens RefreshTokenStore) *Manager {
	return &Manager{
		cfg:           cfg,
		blacklist:     blacklist,
		refreshTokens: refreshTokens,
		now:           time.Now,
	}
}

func (m *Manager) GenerateTokenPair(ctx context.Context, subject Subject) (*TokenPair, error) {
	now := m.now().UTC()
	accessExpiry := now.Add(time.Duration(m.cfg.JWT.AccessTTLSec) * time.Second)
	refreshExpiry := now.Add(time.Duration(m.cfg.JWT.RefreshTTLSec) * time.Second)

	accessClaims := m.newClaims(subject, TokenTypeAccess, uuid.NewString(), now, accessExpiry)
	refreshClaims := m.newClaims(subject, TokenTypeRefresh, uuid.NewString(), now, refreshExpiry)

	accessKeys := m.accessSecrets()
	refreshKeys := m.refreshSecrets()
	if len(accessKeys) == 0 || len(refreshKeys) == 0 {
		return nil, fmt.Errorf("no signing keys configured")
	}

	accessToken, err := m.sign(accessClaims, accessKeys[0])
	if err != nil {
		return nil, err
	}
	refreshToken, err := m.sign(refreshClaims, refreshKeys[0])
	if err != nil {
		return nil, err
	}

	if err := m.refreshTokens.StoreRefreshToken(ctx, subject.UserID, refreshClaims.ID, time.Until(refreshExpiry)); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		AccessExpiresAt:  accessExpiry,
		RefreshExpiresAt: refreshExpiry,
	}, nil
}

func (m *Manager) AuthenticateAccessToken(ctx context.Context, token string) (*Claims, error) {
	claims, err := m.parse(token, m.accessSecrets(), TokenTypeAccess)
	if err != nil {
		return nil, err
	}
	blacklisted, err := m.blacklist.IsBlacklisted(ctx, claims.ID)
	if err != nil {
		return nil, fmt.Errorf("check token blacklist: %w", err)
	}
	if blacklisted {
		return nil, pkgerrors.ErrTokenBlacklisted
	}
	return claims, nil
}

func (m *Manager) ValidateRefreshToken(ctx context.Context, token string) (*Claims, error) {
	claims, err := m.parse(token, m.refreshSecrets(), TokenTypeRefresh)
	if err != nil {
		return nil, err
	}
	exists, err := m.refreshTokens.HasRefreshToken(ctx, claims.UserID, claims.ID)
	if err != nil {
		return nil, fmt.Errorf("check refresh token: %w", err)
	}
	if !exists {
		return nil, pkgerrors.ErrTokenInvalid
	}
	return claims, nil
}

func (m *Manager) BlacklistAccessToken(ctx context.Context, token string) error {
	claims, err := m.parse(token, m.accessSecrets(), TokenTypeAccess)
	if err != nil {
		return err
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		return pkgerrors.ErrTokenInvalid
	}
	return m.blacklist.BlacklistToken(ctx, claims.ID, ttl)
}

func (m *Manager) RevokeRefreshToken(ctx context.Context, token string) error {
	claims, err := m.parse(token, m.refreshSecrets(), TokenTypeRefresh)
	if err != nil {
		return err
	}
	return m.refreshTokens.DeleteRefreshToken(ctx, claims.UserID, claims.ID)
}

func (m *Manager) newClaims(subject Subject, tokenType, jti string, issuedAt, expiresAt time.Time) *Claims {
	return &Claims{
		UserID:    subject.UserID,
		Role:      subject.Role,
		TenantID:  subject.TenantID,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.cfg.JWT.Issuer,
			Subject:   strconv.FormatUint(uint64(subject.UserID), 10),
			Audience:  []string{m.cfg.App.Name},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ID:        jti,
		},
	}
}

type signingKey struct {
	KID    string
	Secret string
}

func (m *Manager) sign(claims *Claims, key signingKey) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = key.KID
	return token.SignedString([]byte(key.Secret))
}

func (m *Manager) accessSecrets() []signingKey {
	return buildKeys(m.cfg.JWT.AccessSecret, m.cfg.JWT.PreviousAccessSecret)
}

func (m *Manager) refreshSecrets() []signingKey {
	return buildKeys(m.cfg.JWT.RefreshSecret, m.cfg.JWT.PreviousRefreshSecret)
}

func buildKeys(primary, previous string) []signingKey {
	var keys []signingKey
	if primary != "" {
		keys = append(keys, signingKey{KID: keyID(primary), Secret: primary})
	}
	if previous != "" && previous != primary {
		keys = append(keys, signingKey{KID: keyID(previous), Secret: previous})
	}
	return keys
}

func keyID(secret string) string {
	if len(secret) <= 8 {
		return secret
	}
	return secret[:8]
}

func (m *Manager) parse(rawToken string, keys []signingKey, tokenType string) (*Claims, error) {
	if rawToken == "" {
		return nil, pkgerrors.ErrUnauthorized
	}

	if len(keys) == 0 {
		return nil, pkgerrors.ErrTokenInvalid
	}

	keyMap := make(map[string]string, len(keys))
	for _, k := range keys {
		keyMap[k.KID] = k.Secret
	}

	var token *jwt.Token
	var err error

	token, err = jwt.ParseWithClaims(rawToken, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, pkgerrors.ErrTokenInvalid
		}
		kid, _ := token.Header["kid"].(string)
		if kid != "" {
			if secret, ok := keyMap[kid]; ok {
				return []byte(secret), nil
			}
		}
		// Fallback: try all keys for tokens without kid (backward compatibility)
		return nil, fmt.Errorf("no matching key")
	})

	if err != nil {
		// Fallback: try each key sequentially for legacy tokens without kid
		for _, key := range keys {
			token, err = jwt.ParseWithClaims(rawToken, &Claims{}, func(token *jwt.Token) (any, error) {
				if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
					return nil, pkgerrors.ErrTokenInvalid
				}
				return []byte(key.Secret), nil
			})
			if err == nil {
				break
			}
			if errors.Is(err, jwt.ErrTokenExpired) {
				return nil, pkgerrors.ErrTokenInvalid
			}
		}
	}
	if err != nil {
		return nil, pkgerrors.ErrTokenInvalid
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, pkgerrors.ErrTokenInvalid
	}
	if claims.TokenType != tokenType || claims.ID == "" || claims.Issuer != m.cfg.JWT.Issuer {
		return nil, pkgerrors.ErrTokenInvalid
	}
	if !containsAudience(claims.Audience, m.cfg.App.Name) {
		return nil, pkgerrors.ErrTokenInvalid
	}
	if claims.UserID == 0 {
		return nil, pkgerrors.ErrTokenInvalid
	}

	return claims, nil
}

func containsAudience(audiences jwt.ClaimStrings, expected string) bool {
	for _, audience := range audiences {
		if audience == expected {
			return true
		}
	}
	return false
}
