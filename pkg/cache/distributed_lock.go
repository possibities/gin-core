package cache

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrLockNotAcquired = errors.New("distributed lock not acquired")

var releaseLockScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`)

var renewLockScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("pexpire", KEYS[1], ARGV[2])
end
return 0
`)

type DistributedLocker struct {
	client redis.UniversalClient
	keys   *Keyspace
}

type Lock struct {
	client  redis.UniversalClient
	key     string
	token   string
	ttl     time.Duration
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func NewDistributedLocker(client *redis.Client, keys *Keyspace) *DistributedLocker {
	return &DistributedLocker{
		client: client,
		keys:   keys,
	}
}

func (l *DistributedLocker) Acquire(ctx context.Context, resource string, ttl time.Duration) (*Lock, error) {
	key := l.keys.Lock(resource)
	token := uuid.NewString()

	acquired, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrLockNotAcquired
	}

	return &Lock{
		client: l.client,
		key:    key,
		token:  token,
		ttl:    ttl,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}, nil
}

// StartWatchdog begins a background goroutine that renews the lock every ttl/3.
// If renewal fails, the provided cancel function is called to abort the critical section.
// Call StopWatchdog when the critical section completes.
func (l *Lock) StartWatchdog(cancel context.CancelFunc) {
	interval := l.ttl / 3
	if interval <= 0 {
		interval = time.Second
	}

	go func() {
		defer close(l.doneCh)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-l.stopCh:
				return
			case <-ticker.C:
				ctx, renewCancel := context.WithTimeout(context.Background(), time.Second)
				renewed, err := renewLockScript.Run(ctx, l.client, []string{l.key}, l.token, l.ttl.Milliseconds()).Int()
				renewCancel()
				if err != nil || renewed == 0 {
					cancel()
					return
				}
			}
		}
	}()
}

// StopWatchdog stops the renewal goroutine and waits for it to exit.
func (l *Lock) StopWatchdog() {
	select {
	case <-l.stopCh:
		// already stopped
	default:
		close(l.stopCh)
	}
	<-l.doneCh
}

func (l *DistributedLocker) WithLock(ctx context.Context, resource string, ttl time.Duration, fn func(context.Context) error) (acquired bool, err error) {
	lock, err := l.Acquire(ctx, resource, ttl)
	if err != nil {
		if errors.Is(err, ErrLockNotAcquired) {
			return false, nil
		}
		return false, err
	}
	acquired = true

	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		err = errors.Join(err, lock.Release(releaseCtx))
	}()

	err = fn(ctx)
	return acquired, err
}

// WithLockAndWatchdog is like WithLock but starts a watchdog that auto-renews the lock.
// If renewal fails, the context passed to fn is cancelled to prevent operating without
// lock protection.
func (l *DistributedLocker) WithLockAndWatchdog(ctx context.Context, resource string, ttl time.Duration, fn func(context.Context) error) (acquired bool, err error) {
	lock, err := l.Acquire(ctx, resource, ttl)
	if err != nil {
		if errors.Is(err, ErrLockNotAcquired) {
			return false, nil
		}
		return false, err
	}
	acquired = true

	watchCtx, watchCancel := context.WithCancel(ctx)
	lock.StartWatchdog(watchCancel)

	defer func() {
		lock.StopWatchdog()
		watchCancel()
		releaseCtx, releaseCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer releaseCancel()
		err = errors.Join(err, lock.Release(releaseCtx))
	}()

	err = fn(watchCtx)
	return acquired, err
}

func (l *Lock) Release(ctx context.Context) error {
	_, err := releaseLockScript.Run(ctx, l.client, []string{l.key}, l.token).Result()
	return err
}
