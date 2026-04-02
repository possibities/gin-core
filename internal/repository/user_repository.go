package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/possibities/gin-core/internal/model"
	pkgerrors "github.com/possibities/gin-core/pkg/errors"
	"gorm.io/gorm"
)

type UserRepository interface {
	FindByID(ctx context.Context, id uint) (*model.User, error)
	FindByIDs(ctx context.Context, ids []uint) ([]*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	Create(ctx context.Context, user *model.User) error
	BatchCreate(ctx context.Context, users []*model.User) error
	Update(ctx context.Context, user *model.User, fields ...string) error
	Delete(ctx context.Context, id uint) error
	WithTx(tx *gorm.DB) UserRepository
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) FindByID(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, translateNotFound(err)
	}
	return &user, nil
}

func (r *userRepository) FindByIDs(ctx context.Context, ids []uint) ([]*model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var users []*model.User
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Where("email = ?", strings.ToLower(strings.TrimSpace(email))).
		First(&user).
		Error
	if err != nil {
		return nil, translateNotFound(err)
	}
	return &user, nil
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	return translateUserRepoError(r.db.WithContext(ctx).Create(user).Error)
}

const batchCreateSize = 100

func (r *userRepository) BatchCreate(ctx context.Context, users []*model.User) error {
	if len(users) == 0 {
		return nil
	}
	for _, u := range users {
		u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	}
	return translateUserRepoError(r.db.WithContext(ctx).CreateInBatches(users, batchCreateSize).Error)
}

func (r *userRepository) Update(ctx context.Context, user *model.User, fields ...string) error {
	updates := selectUserFields(user, fields...)
	if len(updates) == 0 {
		return nil
	}

	tx := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", user.ID).
		Select(fieldNames(updates)).
		Updates(updates)
	if tx.Error != nil {
		return translateUserRepoError(tx.Error)
	}
	if tx.RowsAffected == 0 {
		return pkgerrors.ErrNotFound
	}
	return nil
}

func (r *userRepository) Delete(ctx context.Context, id uint) error {
	tx := r.db.WithContext(ctx).Delete(&model.User{}, id)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return pkgerrors.ErrNotFound
	}
	return nil
}

func (r *userRepository) WithTx(tx *gorm.DB) UserRepository {
	return &userRepository{db: tx}
}

func translateNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return pkgerrors.ErrNotFound
	}
	return err
}

func translateUserRepoError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return pkgerrors.ErrUserEmailExists
	}
	return err
}

func selectUserFields(user *model.User, fields ...string) map[string]any {
	allowed := map[string]any{
		"email":         strings.ToLower(strings.TrimSpace(user.Email)),
		"name":          user.Name,
		"role":          user.Role,
		"tenant_id":     user.TenantID,
		"password_hash": user.PasswordHash,
	}
	if len(fields) == 0 {
		return allowed
	}

	selected := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := allowed[field]; ok {
			selected[field] = value
		}
	}
	return selected
}

func fieldNames(values map[string]any) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	return names
}
