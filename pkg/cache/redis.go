package cache

import (
	"time"

	"github.com/possibities/gin-boilerplate/pkg/config"
	pkgtracing "github.com/possibities/gin-boilerplate/pkg/tracing"
	"github.com/redis/go-redis/v9"
)

func NewRedisClient(cfg *config.Config, tracingProvider *pkgtracing.Provider) (*redis.Client, func(), error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		DialTimeout:  time.Duration(cfg.Redis.DialTimeoutSec) * time.Second,
		ReadTimeout:  time.Duration(cfg.Redis.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.Redis.WriteTimeoutSec) * time.Second,
	})
	client.AddHook(pkgtracing.NewRedisHook(tracingProvider))

	cleanup := func() {
		_ = client.Close()
	}

	return client, cleanup, nil
}
