package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/possibities/gin-core/internal/event"
	"github.com/possibities/gin-core/internal/model"
	"github.com/possibities/gin-core/internal/repository"
	"github.com/possibities/gin-core/pkg/cache"
	"github.com/possibities/gin-core/pkg/config"
	pkgerrors "github.com/possibities/gin-core/pkg/errors"
	"gorm.io/gorm"
)

type stubUserRepository struct {
	findByID func(ctx context.Context, id uint) (*model.User, error)
	update   func(ctx context.Context, user *model.User, fields ...string) error
}

type stubOutboxRepository struct {
	create func(ctx context.Context, event *model.OutboxEvent) error
}

type stubTxManager struct {
	withTx func(ctx context.Context, fn func(*gorm.DB) error) error
}

func (r *stubUserRepository) FindByID(ctx context.Context, id uint) (*model.User, error) {
	return r.findByID(ctx, id)
}

func (r *stubUserRepository) FindByIDs(context.Context, []uint) ([]*model.User, error) {
	return nil, nil
}

func (r *stubUserRepository) FindByEmail(context.Context, string) (*model.User, error) {
	return nil, nil
}

func (r *stubUserRepository) Create(context.Context, *model.User) error {
	return nil
}

func (r *stubUserRepository) BatchCreate(context.Context, []*model.User) error {
	return nil
}

func (r *stubUserRepository) Update(ctx context.Context, user *model.User, fields ...string) error {
	if r.update != nil {
		return r.update(ctx, user, fields...)
	}
	return nil
}

func (r *stubUserRepository) Delete(context.Context, uint) error {
	return nil
}

func (r *stubUserRepository) WithTx(*gorm.DB) repository.UserRepository {
	return r
}

func (r *stubOutboxRepository) Create(ctx context.Context, event *model.OutboxEvent) error {
	if r.create != nil {
		return r.create(ctx, event)
	}
	return nil
}

func (r *stubOutboxRepository) AcquirePending(context.Context, int, time.Time, time.Time, string) ([]*model.OutboxEvent, error) {
	return nil, nil
}

func (r *stubOutboxRepository) MarkPublished(context.Context, string, time.Time) error {
	return nil
}

func (r *stubOutboxRepository) MarkFailed(context.Context, string, int, string, time.Time, bool) error {
	return nil
}

func (r *stubOutboxRepository) WithTx(*gorm.DB) repository.OutboxRepository {
	return r
}

func (m *stubTxManager) WithTx(ctx context.Context, fn func(*gorm.DB) error) error {
	if m.withTx != nil {
		return m.withTx(ctx, fn)
	}
	return fn(nil)
}

type stubReadStore struct {
	getOrLoad func(ctx context.Context, key string, dest any, ttl time.Duration, loader func(context.Context) (any, error)) (cache.LookupStatus, error)
	delete    func(ctx context.Context, key string) error
}

type stubPublisher struct {
	publish func(ctx context.Context, message event.Message) error
}

func (s *stubReadStore) GetJSON(context.Context, string, any) (cache.LookupStatus, error) {
	return cache.LookupMiss, nil
}

func (s *stubReadStore) GetOrLoadJSON(ctx context.Context, key string, dest any, ttl time.Duration, loader func(context.Context) (any, error)) (cache.LookupStatus, error) {
	if s.getOrLoad == nil {
		return cache.LookupMiss, nil
	}
	return s.getOrLoad(ctx, key, dest, ttl, loader)
}

func (s *stubReadStore) SetJSON(context.Context, string, any, time.Duration) error {
	return nil
}

func (s *stubReadStore) SetNull(context.Context, string, time.Duration) error {
	return nil
}

func (s *stubReadStore) Delete(ctx context.Context, key string) error {
	if s.delete != nil {
		return s.delete(ctx, key)
	}
	return nil
}

func (s *stubReadStore) DeleteWithDoubleDelete(ctx context.Context, key string, _ time.Duration) error {
	return s.Delete(ctx, key)
}

func (p *stubPublisher) Publish(ctx context.Context, message event.Message) error {
	if p.publish != nil {
		return p.publish(ctx, message)
	}
	return nil
}

func TestUserServiceGetProfileLoadsAndMapsUser(t *testing.T) {
	repo := &stubUserRepository{
		findByID: func(_ context.Context, id uint) (*model.User, error) {
			return &model.User{
				Model: gorm.Model{
					ID:        id,
					CreatedAt: time.Unix(100, 0),
					UpdatedAt: time.Unix(200, 0),
				},
				Email:    "alice@example.com",
				Name:     "Alice",
				Role:     "member",
				TenantID: "tenant-a",
			}, nil
		},
	}
	cacheStore := &stubReadStore{
		getOrLoad: func(ctx context.Context, _ string, dest any, _ time.Duration, loader func(context.Context) (any, error)) (cache.LookupStatus, error) {
			loaded, err := loader(ctx)
			if err != nil {
				return cache.LookupMiss, err
			}
			profile := loaded.(*UserProfile)
			target := dest.(*UserProfile)
			*target = *profile
			return cache.LookupLoaded, nil
		},
	}

	svc := NewUserService(repo, &stubOutboxRepository{}, &stubTxManager{}, cacheStore, cache.NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
	}), &stubPublisher{})

	profile, err := svc.GetProfile(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetProfile() error = %v", err)
	}
	if profile.ID != 42 || profile.Email != "alice@example.com" || profile.Role != "member" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func TestUserServiceGetProfileReturnsNotFoundOnCachedNull(t *testing.T) {
	svc := NewUserService(&stubUserRepository{}, &stubOutboxRepository{}, &stubTxManager{}, &stubReadStore{
		getOrLoad: func(context.Context, string, any, time.Duration, func(context.Context) (any, error)) (cache.LookupStatus, error) {
			return cache.LookupNull, nil
		},
	}, cache.NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
	}), &stubPublisher{})

	_, err := svc.GetProfile(context.Background(), 7)
	if err != pkgerrors.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUserServiceUpdateProfileInvalidatesCache(t *testing.T) {
	repo := &stubUserRepository{}
	var findCalls int
	currentEmail := "alice@example.com"
	currentName := "Alice"
	currentTenant := "tenant-a"
	repo.findByID = func(_ context.Context, id uint) (*model.User, error) {
		findCalls++
		return &model.User{
			Model: gorm.Model{
				ID:        id,
				CreatedAt: time.Unix(100, 0),
				UpdatedAt: time.Unix(200+int64(findCalls), 0),
			},
			Email:    currentEmail,
			Name:     currentName,
			Role:     "member",
			TenantID: currentTenant,
		}, nil
	}

	var updatedEmail string
	var updatedName string
	var updatedTenant string
	repo.update = func(_ context.Context, user *model.User, _ ...string) error {
		updatedEmail = user.Email
		updatedName = user.Name
		updatedTenant = user.TenantID
		currentEmail = user.Email
		currentName = user.Name
		currentTenant = user.TenantID
		return nil
	}

	var deletedKey string
	var outboxTopic string
	cacheStore := &stubReadStore{
		delete: func(_ context.Context, key string) error {
			deletedKey = key
			return nil
		},
	}

	svc := NewUserService(repo, &stubOutboxRepository{
		create: func(_ context.Context, evt *model.OutboxEvent) error {
			outboxTopic = evt.Topic
			if evt.Status != model.OutboxStatusPending {
				t.Fatalf("expected pending outbox status, got %q", evt.Status)
			}
			return nil
		},
	}, &stubTxManager{}, cacheStore, cache.NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
	}), &stubPublisher{
		publish: func(_ context.Context, message event.Message) error {
			evt, ok := message.(UserProfileUpdatedEvent)
			if !ok {
				t.Fatalf("expected UserProfileUpdatedEvent, got %T", message)
			}
			if evt.UserID != 42 || evt.ActorUserID != 42 || evt.After.Name != "Alice Updated" {
				t.Fatalf("unexpected event payload: %+v", evt)
			}
			return nil
		},
	})

	profile, err := svc.UpdateProfile(context.Background(), 42, UpdateUserProfileInput{
		ActorUserID: 42,
		Email:       " Alice@Example.com ",
		Name:        " Alice Updated ",
		TenantID:    " tenant-b ",
		IPAddress:   "127.0.0.1",
		TraceID:     "trace-1",
	})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if updatedEmail != "alice@example.com" || updatedName != "Alice Updated" || updatedTenant != "tenant-b" {
		t.Fatalf("unexpected updated values: %q %q %q", updatedEmail, updatedName, updatedTenant)
	}
	if deletedKey != "gin-core:user:profile:42" {
		t.Fatalf("expected profile cache invalidation, got %q", deletedKey)
	}
	if outboxTopic != userProfileUpdatedMQTopic {
		t.Fatalf("expected outbox topic %q, got %q", userProfileUpdatedMQTopic, outboxTopic)
	}
	if profile.ID != 42 || profile.Email != "alice@example.com" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func TestUserServiceUpdateProfileReturnsConflict(t *testing.T) {
	repo := &stubUserRepository{
		findByID: func(_ context.Context, id uint) (*model.User, error) {
			return &model.User{Model: gorm.Model{ID: id}, Email: "alice@example.com", Name: "Alice"}, nil
		},
		update: func(_ context.Context, _ *model.User, _ ...string) error {
			return pkgerrors.ErrUserEmailExists
		},
	}

	svc := NewUserService(repo, &stubOutboxRepository{}, &stubTxManager{}, &stubReadStore{}, cache.NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
	}), &stubPublisher{})

	_, err := svc.UpdateProfile(context.Background(), 42, UpdateUserProfileInput{
		ActorUserID: 42,
		Email:       "bob@example.com",
		Name:        "Bob",
		TenantID:    "tenant-a",
	})
	if err != pkgerrors.ErrUserEmailExists {
		t.Fatalf("expected ErrUserEmailExists, got %v", err)
	}
}

func TestUserServiceUpdateProfileIgnoresCacheDeleteFailure(t *testing.T) {
	repo := &stubUserRepository{
		findByID: func(_ context.Context, id uint) (*model.User, error) {
			return &model.User{
				Model: gorm.Model{
					ID:        id,
					CreatedAt: time.Unix(100, 0),
					UpdatedAt: time.Unix(200, 0),
				},
				Email:    "alice@example.com",
				Name:     "Alice",
				Role:     "member",
				TenantID: "tenant-a",
			}, nil
		},
		update: func(_ context.Context, _ *model.User, _ ...string) error {
			return nil
		},
	}

	svc := NewUserService(repo, &stubOutboxRepository{}, &stubTxManager{}, &stubReadStore{
		delete: func(_ context.Context, _ string) error {
			return errors.New("redis down")
		},
	}, cache.NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
	}), &stubPublisher{})

	if _, err := svc.UpdateProfile(context.Background(), 42, UpdateUserProfileInput{
		ActorUserID: 42,
		Email:       "alice@example.com",
		Name:        "Alice Updated",
		TenantID:    "tenant-a",
	}); err != nil {
		t.Fatalf("expected cache delete failure to degrade open, got %v", err)
	}
}

func TestUserServiceUpdateProfileRejectsBlankFields(t *testing.T) {
	svc := NewUserService(&stubUserRepository{}, &stubOutboxRepository{}, &stubTxManager{}, &stubReadStore{}, cache.NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
	}), &stubPublisher{})

	_, err := svc.UpdateProfile(context.Background(), 42, UpdateUserProfileInput{
		ActorUserID: 42,
		Email:       "   ",
		Name:        "   ",
		TenantID:    "tenant-a",
	})
	if err != pkgerrors.ErrInvalidRequest {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestUserServiceUpdateProfileRejectsInvalidEmailFormat(t *testing.T) {
	svc := NewUserService(&stubUserRepository{}, &stubOutboxRepository{}, &stubTxManager{}, &stubReadStore{}, cache.NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
	}), &stubPublisher{})

	_, err := svc.UpdateProfile(context.Background(), 42, UpdateUserProfileInput{
		ActorUserID: 42,
		Email:       "invalid-email",
		Name:        "Alice",
		TenantID:    "tenant-a",
	})
	if err != pkgerrors.ErrInvalidRequest {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestUserServiceUpdateProfileIgnoresPublishFailure(t *testing.T) {
	repo := &stubUserRepository{
		findByID: func(_ context.Context, id uint) (*model.User, error) {
			return &model.User{
				Model: gorm.Model{
					ID:        id,
					CreatedAt: time.Unix(100, 0),
					UpdatedAt: time.Unix(200, 0),
				},
				Email:    "alice@example.com",
				Name:     "Alice",
				Role:     "member",
				TenantID: "tenant-a",
			}, nil
		},
		update: func(_ context.Context, _ *model.User, _ ...string) error {
			return nil
		},
	}

	svc := NewUserService(repo, &stubOutboxRepository{}, &stubTxManager{}, &stubReadStore{}, cache.NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
	}), &stubPublisher{
		publish: func(context.Context, event.Message) error {
			return pkgerrors.ErrTooManyRequests
		},
	})

	if _, err := svc.UpdateProfile(context.Background(), 42, UpdateUserProfileInput{
		ActorUserID: 42,
		Email:       "alice@example.com",
		Name:        "Alice Updated",
		TenantID:    "tenant-a",
	}); err != nil {
		t.Fatalf("expected publish failure to degrade open, got %v", err)
	}
}

func TestUserServiceUpdateProfileReturnsOutboxCreateError(t *testing.T) {
	repo := &stubUserRepository{
		findByID: func(_ context.Context, id uint) (*model.User, error) {
			return &model.User{Model: gorm.Model{ID: id}, Email: "alice@example.com", Name: "Alice"}, nil
		},
		update: func(_ context.Context, _ *model.User, _ ...string) error {
			return nil
		},
	}

	svc := NewUserService(repo, &stubOutboxRepository{
		create: func(context.Context, *model.OutboxEvent) error {
			return pkgerrors.ErrInternal
		},
	}, &stubTxManager{}, &stubReadStore{}, cache.NewKeyspace(&config.Config{
		App: config.AppConfig{Name: "gin-core"},
	}), &stubPublisher{})

	if _, err := svc.UpdateProfile(context.Background(), 42, UpdateUserProfileInput{
		ActorUserID: 42,
		Email:       "alice@example.com",
		Name:        "Alice Updated",
		TenantID:    "tenant-a",
	}); err != pkgerrors.ErrInternal {
		t.Fatalf("expected outbox error to abort update, got %v", err)
	}
}
