package dependency

import (
	"context"
	"fmt"
	"time"

	"github.com/possibities/gin-boilerplate/internal/handler"
	"github.com/possibities/gin-boilerplate/pkg/config"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type dependencyChecker struct {
	name  string
	check func() error
}

func (c *dependencyChecker) Name() string {
	return c.name
}

func (c *dependencyChecker) Check() error {
	return c.check()
}

func NewDependencyCheckers(cfg *config.Config, db *gorm.DB, redisClient *redis.Client) []handler.DependencyChecker {
	sqlDB, _ := db.DB()
	return []handler.DependencyChecker{
		&dependencyChecker{
			name: "postgres",
			check: func() error {
				if sqlDB == nil {
					return fmt.Errorf("sql db unavailable")
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.DB.DialTimeoutSec)*time.Second)
				defer cancel()
				return sqlDB.PingContext(ctx)
			},
		},
		&dependencyChecker{
			name: "redis",
			check: func() error {
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Redis.DialTimeoutSec)*time.Second)
				defer cancel()
				return redisClient.Ping(ctx).Err()
			},
		},
	}
}
