package pkgjwt

import (
	"context"
	"fmt"
	"time"

	"github.com/possibities/gin-boilerplate/pkg/cache"
	"github.com/redis/go-redis/v9"
)

type RedisTokenStore struct {
	client *redis.Client
	keys   *cache.Keyspace
}

func NewRedisTokenStore(keys *cache.Keyspace, client *redis.Client) *RedisTokenStore {
	return &RedisTokenStore{
		client: client,
		keys:   keys,
	}
}

func (s *RedisTokenStore) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	return s.client.Set(ctx, blacklistKey(jti), "1", ttl).Err()
}

func (s *RedisTokenStore) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	count, err := s.client.Exists(ctx, blacklistKey(jti)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *RedisTokenStore) StoreRefreshToken(ctx context.Context, userID uint, jti string, ttl time.Duration) error {
	if err := s.client.Set(ctx, s.keys.RefreshToken(userID, jti), "1", ttl).Err(); err != nil {
		return err
	}
	return s.client.Set(ctx, legacyRefreshTokenKey(userID, jti), "1", ttl).Err()
}

func (s *RedisTokenStore) HasRefreshToken(ctx context.Context, userID uint, jti string) (bool, error) {
	count, err := s.client.Exists(ctx, s.keys.RefreshToken(userID, jti)).Result()
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	count, err = s.client.Exists(ctx, legacyRefreshTokenKey(userID, jti)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *RedisTokenStore) DeleteRefreshToken(ctx context.Context, userID uint, jti string) error {
	return s.client.Del(ctx, s.keys.RefreshToken(userID, jti), legacyRefreshTokenKey(userID, jti)).Err()
}

func blacklistKey(jti string) string {
	return cache.JWTBlacklistKey(jti)
}

func legacyRefreshTokenKey(userID uint, jti string) string {
	return fmt.Sprintf("auth:refresh:%d:%s", userID, jti)
}
