package cache

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"time"

	"github.com/possibities/gin-boilerplate/pkg/config"
	"github.com/possibities/gin-boilerplate/pkg/metrics"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type LookupStatus int

const (
	LookupMiss LookupStatus = iota
	LookupHit
	LookupNull
	LookupLoaded
)

type ReadStore interface {
	GetJSON(ctx context.Context, key string, dest any) (LookupStatus, error)
	GetOrLoadJSON(ctx context.Context, key string, dest any, ttl time.Duration, loader func(context.Context) (any, error)) (LookupStatus, error)
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
	SetNull(ctx context.Context, key string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	DeleteWithDoubleDelete(ctx context.Context, key string, delay time.Duration) error
}

type redisKV interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

type ReadThroughStore struct {
	client    redisKV
	group     singleflight.Group
	nullTTL   time.Duration
	ttlJitter time.Duration
	metrics   *metrics.Registry
}

type cacheEnvelope struct {
	Null bool            `json:"null,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type loadResult struct {
	Status  LookupStatus
	Payload json.RawMessage
}

func NewReadThroughStore(cfg *config.Config, client *redis.Client, registry *metrics.Registry) *ReadThroughStore {
	store := newReadThroughStore(
		client,
		time.Duration(cfg.Cache.NullTTLSec)*time.Second,
		time.Duration(cfg.Cache.TTLJitterSec)*time.Second,
	)
	store.metrics = registry
	return store
}

func newReadThroughStore(client redisKV, nullTTL, ttlJitter time.Duration) *ReadThroughStore {
	return &ReadThroughStore{
		client:    client,
		nullTTL:   nullTTL,
		ttlJitter: ttlJitter,
	}
}

func (s *ReadThroughStore) GetJSON(ctx context.Context, key string, dest any) (LookupStatus, error) {
	return s.getJSON(ctx, key, dest, true)
}

func (s *ReadThroughStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.writeEnvelope(ctx, key, cacheEnvelope{Data: payload}, ttl, true)
}

func (s *ReadThroughStore) SetNull(ctx context.Context, key string, ttl time.Duration) error {
	return s.writeEnvelope(ctx, key, cacheEnvelope{Null: true}, ttl, false)
}

func (s *ReadThroughStore) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

// DeleteWithDoubleDelete performs an immediate delete followed by a delayed
// second delete after the specified delay. This mitigates the race condition
// where a concurrent read re-populates stale data between the DB write and
// the cache delete. The second delete runs in a background goroutine and
// uses a detached context so it is not cancelled by the parent request.
func (s *ReadThroughStore) DeleteWithDoubleDelete(ctx context.Context, key string, delay time.Duration) error { //nolint:contextcheck // second delete intentionally detached from request lifecycle
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return err
	}
	go func() {
		time.Sleep(delay)
		bgCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.client.Del(bgCtx, key).Err()
	}()
	return nil
}

func (s *ReadThroughStore) GetOrLoadJSON(
	ctx context.Context,
	key string,
	dest any,
	ttl time.Duration,
	loader func(context.Context) (any, error),
) (LookupStatus, error) {
	status, err := s.getJSON(ctx, key, dest, false)
	if err == nil && status != LookupMiss {
		s.observeLookup(status)
		return status, nil
	}

	result, loadErr, _ := s.group.Do(key, func() (any, error) {
		raw, cacheErr := s.client.Get(ctx, key).Bytes()
		if cacheErr == nil {
			cachedStatus, payload, decodeErr := decodeEnvelope(raw)
			if decodeErr == nil && cachedStatus != LookupMiss {
				return loadResult{Status: cachedStatus, Payload: payload}, nil
			}
		}

		value, err := loader(ctx)
		if err != nil {
			return loadResult{}, err
		}
		if value == nil {
			_ = s.SetNull(ctx, key, s.nullTTL)
			return loadResult{Status: LookupNull}, nil
		}

		payload, err := json.Marshal(value)
		if err != nil {
			return loadResult{}, err
		}
		_ = s.writeEnvelope(ctx, key, cacheEnvelope{Data: payload}, ttl, true)
		return loadResult{Status: LookupLoaded, Payload: payload}, nil
	})
	if loadErr != nil {
		return LookupMiss, loadErr
	}

	loaded, ok := result.(loadResult)
	if !ok {
		return LookupMiss, errors.New("unexpected cache load result")
	}
	if loaded.Status == LookupHit || loaded.Status == LookupLoaded {
		if err := decodePayload(loaded.Payload, dest); err != nil {
			return LookupMiss, err
		}
	}
	s.observeLookup(loaded.Status)
	return loaded.Status, nil
}

func (s *ReadThroughStore) getJSON(ctx context.Context, key string, dest any, observe bool) (LookupStatus, error) {
	raw, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		if observe {
			s.observeLookup(LookupMiss)
		}
		return LookupMiss, nil
	}
	if err != nil {
		return LookupMiss, err
	}

	status, payload, err := decodeEnvelope(raw)
	if err != nil {
		return LookupMiss, err
	}
	if status == LookupHit {
		if err := decodePayload(payload, dest); err != nil {
			return LookupMiss, err
		}
	}
	if observe {
		s.observeLookup(status)
	}
	return status, nil
}

func (s *ReadThroughStore) writeEnvelope(ctx context.Context, key string, envelope cacheEnvelope, ttl time.Duration, jitter bool) error {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if jitter {
		ttl = s.effectiveTTL(ttl)
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	return s.client.Set(ctx, key, encoded, ttl).Err()
}

func (s *ReadThroughStore) effectiveTTL(base time.Duration) time.Duration {
	if base <= 0 {
		return time.Second
	}
	if s.ttlJitter <= 0 {
		return base
	}

	deltaMax := int64(s.ttlJitter)
	deltaRange := big.NewInt(deltaMax*2 + 1)
	randomDelta, err := rand.Int(rand.Reader, deltaRange)
	if err != nil {
		return base
	}
	delta := randomDelta.Int64() - deltaMax
	ttl := base + time.Duration(delta)
	if ttl <= 0 {
		return time.Second
	}
	return ttl
}

func (s *ReadThroughStore) observeLookup(status LookupStatus) {
	if s.metrics == nil {
		return
	}
	switch status {
	case LookupHit, LookupNull:
		s.metrics.ObserveCacheLookup(true)
	case LookupMiss, LookupLoaded:
		s.metrics.ObserveCacheLookup(false)
	}
}

func decodeEnvelope(raw []byte) (LookupStatus, json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return LookupNull, nil, nil
	}

	var envelope cacheEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return LookupMiss, nil, err
	}
	if envelope.Null {
		return LookupNull, nil, nil
	}
	if len(envelope.Data) == 0 {
		return LookupHit, json.RawMessage(raw), nil
	}
	return LookupHit, envelope.Data, nil
}

func decodePayload(payload json.RawMessage, dest any) error {
	if dest == nil {
		return errors.New("cache decode destination must be a non-nil pointer")
	}
	value := reflect.ValueOf(dest)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return errors.New("cache decode destination must be a non-nil pointer")
	}
	return json.Unmarshal(payload, dest)
}
