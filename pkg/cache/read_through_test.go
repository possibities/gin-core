package cache

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/possibities/gin-core/pkg/config"
	"github.com/possibities/gin-core/pkg/metrics"
	"github.com/redis/go-redis/v9"
)

type fakeRedis struct {
	mu          sync.Mutex
	entries     map[string]fakeEntry
	getErr      error
	setErr      error
	delErr      error
	lastSetTTL  time.Duration
	lastDelKeys []string
}

type fakeEntry struct {
	value string
	ttl   time.Duration
}

func (f *fakeRedis) Get(_ context.Context, key string) *redis.StringCmd {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.getErr != nil {
		return redis.NewStringResult("", f.getErr)
	}
	entry, ok := f.entries[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(entry.value, nil)
}

func (f *fakeRedis) Set(_ context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.lastSetTTL = expiration
	if f.setErr != nil {
		return redis.NewStatusResult("", f.setErr)
	}
	if f.entries == nil {
		f.entries = make(map[string]fakeEntry)
	}
	f.entries[key] = fakeEntry{
		value: toStoredString(value),
		ttl:   expiration,
	}
	return redis.NewStatusResult("OK", nil)
}

func (f *fakeRedis) Del(_ context.Context, keys ...string) *redis.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.lastDelKeys = append([]string(nil), keys...)
	if f.delErr != nil {
		return redis.NewIntResult(0, f.delErr)
	}
	for _, key := range keys {
		delete(f.entries, key)
	}
	return redis.NewIntResult(int64(len(keys)), nil)
}

func TestKeyspaceBuildsRepositoryFriendlyKeys(t *testing.T) {
	keys := NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "billing"},
	})

	if got := keys.Entity("user", 42); got != "billing:user:42" {
		t.Fatalf("expected entity key with service prefix, got %q", got)
	}
	if got := keys.List("order", "abc123"); got != "billing:order:list:abc123" {
		t.Fatalf("expected list key, got %q", got)
	}
	if got := keys.Lock("invoice-1"); got != "billing:lock:invoice-1" {
		t.Fatalf("expected lock key, got %q", got)
	}
	if got := keys.RefreshToken(7, "jti-1"); got != "billing:auth:refresh:7:jti-1" {
		t.Fatalf("expected refresh token key, got %q", got)
	}
	if got := JWTBlacklistKey("jti-1"); got != "blacklist:jwt:jti-1" {
		t.Fatalf("expected blacklist key, got %q", got)
	}
}

func TestReadThroughStoreLoadsAndCachesValue(t *testing.T) {
	backend := &fakeRedis{}
	store := newReadThroughStore(backend, 30*time.Second, 0)

	type userProfile struct {
		Name string `json:"name"`
	}

	var profile userProfile
	status, err := store.GetOrLoadJSON(context.Background(), "billing:user:42", &profile, 2*time.Minute, func(context.Context) (any, error) {
		return userProfile{Name: "alice"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoadJSON() error = %v", err)
	}
	if status != LookupLoaded {
		t.Fatalf("expected LookupLoaded, got %v", status)
	}
	if profile.Name != "alice" {
		t.Fatalf("expected decoded profile, got %+v", profile)
	}
	if backend.lastSetTTL != 2*time.Minute {
		t.Fatalf("expected cached ttl 2m, got %s", backend.lastSetTTL)
	}

	var cached userProfile
	status, err = store.GetJSON(context.Background(), "billing:user:42", &cached)
	if err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if status != LookupHit {
		t.Fatalf("expected LookupHit, got %v", status)
	}
	if cached.Name != "alice" {
		t.Fatalf("expected cached profile, got %+v", cached)
	}
}

func TestReadThroughStoreCountsCacheMissOnlyOncePerLookup(t *testing.T) {
	backend := &fakeRedis{}
	store := newReadThroughStore(backend, 30*time.Second, 0)
	store.metrics = metrics.New(nil)

	var profile struct {
		Name string `json:"name"`
	}
	status, err := store.GetOrLoadJSON(context.Background(), "billing:user:42", &profile, time.Minute, func(context.Context) (any, error) {
		return struct {
			Name string `json:"name"`
		}{Name: "alice"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoadJSON() error = %v", err)
	}
	if status != LookupLoaded {
		t.Fatalf("expected LookupLoaded, got %v", status)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	store.metrics.Handler().ServeHTTP(recorder, req)

	body := recorder.Body.String()
	if !strings.Contains(body, "cache_miss_total 1") {
		t.Fatalf("expected a single cache miss, got %s", body)
	}
}

func TestReadThroughStoreReadsLegacyPlainJSONPayload(t *testing.T) {
	backend := &fakeRedis{
		entries: map[string]fakeEntry{
			"billing:user:42": {value: `{"name":"alice"}`},
		},
	}
	store := newReadThroughStore(backend, 30*time.Second, 0)

	type userProfile struct {
		Name string `json:"name"`
	}

	var profile userProfile
	status, err := store.GetJSON(context.Background(), "billing:user:42", &profile)
	if err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if status != LookupHit {
		t.Fatalf("expected LookupHit for legacy payload, got %v", status)
	}
	if profile.Name != "alice" {
		t.Fatalf("expected legacy payload to decode, got %+v", profile)
	}
}

func TestReadThroughStoreCachesNullResult(t *testing.T) {
	backend := &fakeRedis{}
	store := newReadThroughStore(backend, 30*time.Second, 0)

	var loaderCalls atomic.Int32
	var ignored struct{}
	status, err := store.GetOrLoadJSON(context.Background(), "billing:user:404", &ignored, time.Minute, func(context.Context) (any, error) {
		loaderCalls.Add(1)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoadJSON() error = %v", err)
	}
	if status != LookupNull {
		t.Fatalf("expected LookupNull, got %v", status)
	}
	if backend.lastSetTTL != 30*time.Second {
		t.Fatalf("expected null ttl 30s, got %s", backend.lastSetTTL)
	}

	status, err = store.GetOrLoadJSON(context.Background(), "billing:user:404", &ignored, time.Minute, func(context.Context) (any, error) {
		loaderCalls.Add(1)
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoadJSON() second call error = %v", err)
	}
	if status != LookupNull {
		t.Fatalf("expected cached LookupNull, got %v", status)
	}
	if loaderCalls.Load() != 1 {
		t.Fatalf("expected loader to run once, got %d", loaderCalls.Load())
	}
}

func TestReadThroughStoreSingleflightDeduplicatesConcurrentMisses(t *testing.T) {
	backend := &fakeRedis{}
	store := newReadThroughStore(backend, 30*time.Second, 0)

	type profile struct {
		Name string `json:"name"`
	}

	var loaderCalls atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	errCh := make(chan error, 8)

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			var result profile
			status, err := store.GetOrLoadJSON(context.Background(), "billing:user:9", &result, time.Minute, func(context.Context) (any, error) {
				loaderCalls.Add(1)
				time.Sleep(20 * time.Millisecond)
				return profile{Name: "hot-key"}, nil
			})
			if err != nil {
				errCh <- err
				return
			}
			if status != LookupLoaded {
				errCh <- errors.New("unexpected status")
				return
			}
			if result.Name != "hot-key" {
				errCh <- errors.New("unexpected decoded payload")
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent GetOrLoadJSON() error = %v", err)
	}
	if loaderCalls.Load() != 1 {
		t.Fatalf("expected loader to run once, got %d", loaderCalls.Load())
	}
}

func TestReadThroughStoreFailsOpenOnCacheReadError(t *testing.T) {
	backend := &fakeRedis{
		getErr: errors.New("redis unavailable"),
		setErr: errors.New("redis unavailable"),
	}
	store := newReadThroughStore(backend, 30*time.Second, 0)

	type profile struct {
		Name string `json:"name"`
	}

	var result profile
	status, err := store.GetOrLoadJSON(context.Background(), "billing:user:1", &result, time.Minute, func(context.Context) (any, error) {
		return profile{Name: "fallback"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoadJSON() error = %v", err)
	}
	if status != LookupLoaded {
		t.Fatalf("expected LookupLoaded, got %v", status)
	}
	if result.Name != "fallback" {
		t.Fatalf("expected loader result despite cache failure, got %+v", result)
	}
}

func TestReadThroughStoreAppliesTTLJitter(t *testing.T) {
	backend := &fakeRedis{}
	store := newReadThroughStore(backend, 30*time.Second, 10*time.Second)

	if err := store.SetJSON(context.Background(), "billing:user:1", map[string]string{"name": "alice"}, 30*time.Second); err != nil {
		t.Fatalf("SetJSON() error = %v", err)
	}
	if backend.lastSetTTL < 20*time.Second || backend.lastSetTTL > 40*time.Second {
		t.Fatalf("expected jittered ttl in [20s, 40s], got %s", backend.lastSetTTL)
	}
}

func toStoredString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}
