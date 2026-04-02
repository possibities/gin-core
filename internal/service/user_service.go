package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/possibities/gin-core/internal/event"
	"github.com/possibities/gin-core/internal/model"
	"github.com/possibities/gin-core/internal/repository"
	"github.com/possibities/gin-core/pkg/cache"
	pkgerrors "github.com/possibities/gin-core/pkg/errors"
	pkglogger "github.com/possibities/gin-core/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const userProfileTTL = 5 * time.Minute

type UserProfile struct {
	ID        uint      `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	TenantID  string    `json:"tenant_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UpdateUserProfileInput struct {
	ActorUserID uint
	Email       string
	Name        string
	TenantID    string
	IPAddress   string
	TraceID     string
}

type UserService interface {
	GetProfile(ctx context.Context, userID uint) (*UserProfile, error)
	UpdateProfile(ctx context.Context, userID uint, input UpdateUserProfileInput) (*UserProfile, error)
}

type userService struct {
	users  repository.UserRepository
	outbox repository.OutboxRepository
	txm    repository.TxManager
	cache  cache.ReadStore
	keys   *cache.Keyspace
	bus    event.Publisher
	now    func() time.Time
}

func NewUserService(
	users repository.UserRepository,
	outbox repository.OutboxRepository,
	txm repository.TxManager,
	cacheStore cache.ReadStore,
	keys *cache.Keyspace,
	bus event.Publisher,
) UserService {
	return &userService{
		users:  users,
		outbox: outbox,
		txm:    txm,
		cache:  cacheStore,
		keys:   keys,
		bus:    bus,
		now:    time.Now,
	}
}

func (s *userService) GetProfile(ctx context.Context, userID uint) (*UserProfile, error) {
	profileKey := s.keys.Entity("user:profile", userID)

	var profile UserProfile
	status, err := s.cache.GetOrLoadJSON(ctx, profileKey, &profile, userProfileTTL, func(ctx context.Context) (any, error) {
		user, err := s.users.FindByID(ctx, userID)
		if errors.Is(err, pkgerrors.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return mapUserProfile(user), nil
	})
	if err != nil {
		return nil, err
	}
	if status == cache.LookupNull {
		return nil, pkgerrors.ErrNotFound
	}
	return &profile, nil
}

func (s *userService) UpdateProfile(ctx context.Context, userID uint, input UpdateUserProfileInput) (*UserProfile, error) {
	input.Email = normalizeEmail(input.Email)
	input.Name = strings.TrimSpace(input.Name)
	input.TenantID = strings.TrimSpace(input.TenantID)
	if input.Email == "" || input.Name == "" || !isValidEmail(input.Email) {
		return nil, pkgerrors.ErrInvalidRequest
	}

	var (
		before *UserProfile
		after  *UserProfile
		evt    UserProfileUpdatedEvent
	)
	err := s.txm.WithTx(ctx, func(tx *gorm.DB) error {
		users := s.users.WithTx(tx)
		outbox := s.outbox.WithTx(tx)

		user, err := users.FindByID(ctx, userID)
		if err != nil {
			return err
		}
		before = mapUserProfile(user)

		user.Email = input.Email
		user.Name = input.Name
		user.TenantID = input.TenantID

		if err := users.Update(ctx, user, "email", "name", "tenant_id"); err != nil {
			return err
		}

		updated, err := users.FindByID(ctx, userID)
		if err != nil {
			return err
		}
		after = mapUserProfile(updated)
		evt = UserProfileUpdatedEvent{
			ActorUserID: input.ActorUserID,
			UserID:      userID,
			Before:      *before,
			After:       *after,
			IPAddress:   input.IPAddress,
			TraceID:     input.TraceID,
		}

		outboxEvent, err := evt.OutboxEvent(s.now())
		if err != nil {
			return err
		}
		return outbox.Create(ctx, outboxEvent)
	})
	if err != nil {
		return nil, err
	}

	if err := s.cache.Delete(ctx, s.keys.Entity("user:profile", userID)); err != nil {
		pkglogger.FromContext(ctx).Warn("delete user profile cache failed",
			zap.Error(err),
			zap.Uint("user_id", userID),
		)
	}
	if err := s.bus.Publish(ctx, evt); err != nil {
		pkglogger.FromContext(ctx).Warn("publish user profile updated event failed",
			zap.Error(err),
			zap.Uint("user_id", userID),
		)
	}
	return after, nil
}

func mapUserProfile(user *model.User) *UserProfile {
	return &UserProfile{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Role:      user.Role,
		TenantID:  user.TenantID,
		CreatedAt: user.CreatedAt.UTC(),
		UpdatedAt: user.UpdatedAt.UTC(),
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isValidEmail(email string) bool {
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	return parsed.Address == email
}
