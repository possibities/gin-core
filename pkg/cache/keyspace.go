package cache

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/possibities/gin-boilerplate/pkg/config"
)

type Keyspace struct {
	service string
}

func NewKeyspace(cfg *config.Config) *Keyspace {
	return &Keyspace{service: strings.TrimSpace(cfg.App.Name)}
}

func (k *Keyspace) Join(parts ...string) string {
	segments := make([]string, 0, len(parts)+1)
	if k.service != "" {
		segments = append(segments, k.service)
	}
	for _, part := range parts {
		if part != "" {
			segments = append(segments, part)
		}
	}
	return strings.Join(segments, ":")
}

func (k *Keyspace) Entity(entity string, id any) string {
	return k.Join(entity, toString(id))
}

func (k *Keyspace) List(entity, hash string) string {
	return k.Join(entity, "list", hash)
}

func (k *Keyspace) Lock(resource string) string {
	return k.Join("lock", resource)
}

func (k *Keyspace) RateLimitIP(ip string) string {
	return k.Join("ratelimit", "ip", ip)
}

func (k *Keyspace) RefreshToken(userID uint, jti string) string {
	return k.Join("auth", "refresh", strconv.FormatUint(uint64(userID), 10), jti)
}

func JWTBlacklistKey(jti string) string {
	return strings.Join([]string{"blacklist", "jwt", jti}, ":")
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprint(v)
	}
}
