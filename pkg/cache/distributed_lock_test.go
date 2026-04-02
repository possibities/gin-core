package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/possibities/gin-boilerplate/pkg/config"
	"github.com/redis/go-redis/v9"
)

func TestDistributedLockerAcquireAndRelease(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	locker := NewDistributedLocker(client, NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-boilerplate"},
	}))

	lock, err := locker.Acquire(context.Background(), "scheduler:outbox-dispatch", 10*time.Second)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	if _, err := locker.Acquire(context.Background(), "scheduler:outbox-dispatch", 10*time.Second); !errors.Is(err, ErrLockNotAcquired) {
		t.Fatalf("expected ErrLockNotAcquired, got %v", err)
	}

	if err := lock.Release(context.Background()); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	if _, err := locker.Acquire(context.Background(), "scheduler:outbox-dispatch", 10*time.Second); err != nil {
		t.Fatalf("expected lock reacquire after release, got %v", err)
	}
}

func TestDistributedLockerReleaseDoesNotDeleteForeignToken(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	locker := NewDistributedLocker(client, NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-boilerplate"},
	}))

	lock, err := locker.Acquire(context.Background(), "scheduler:outbox-dispatch", 10*time.Second)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	key := locker.keys.Lock("scheduler:outbox-dispatch")
	if err := client.Set(context.Background(), key, "other-token", 10*time.Second).Err(); err != nil {
		t.Fatalf("set foreign token: %v", err)
	}

	if err := lock.Release(context.Background()); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	value, err := client.Get(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("get lock key: %v", err)
	}
	if value != "other-token" {
		t.Fatalf("expected foreign token to remain, got %q", value)
	}
}

func TestDistributedLockerWithLockReleasesOnPanic(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	locker := NewDistributedLocker(client, NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-boilerplate"},
	}))

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}

		if _, err := locker.Acquire(context.Background(), "scheduler:outbox-dispatch", 10*time.Second); err != nil {
			t.Fatalf("expected lock to be released after panic, got %v", err)
		}
	}()

	_, _ = locker.WithLock(context.Background(), "scheduler:outbox-dispatch", 10*time.Second, func(context.Context) error {
		panic("boom")
	})
}
