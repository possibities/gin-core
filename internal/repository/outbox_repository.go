package repository

import (
	"context"
	"time"

	"github.com/possibities/gin-boilerplate/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OutboxRepository interface {
	Create(ctx context.Context, event *model.OutboxEvent) error
	AcquirePending(ctx context.Context, limit int, now time.Time, staleBefore time.Time, owner string) ([]*model.OutboxEvent, error)
	MarkPublished(ctx context.Context, id string, publishedAt time.Time) error
	MarkFailed(ctx context.Context, id string, attempts int, lastError string, nextAttemptAt time.Time, dead bool) error
	WithTx(tx *gorm.DB) OutboxRepository
}

type outboxRepository struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) OutboxRepository {
	return &outboxRepository{db: db}
}

func (r *outboxRepository) Create(ctx context.Context, event *model.OutboxEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *outboxRepository) AcquirePending(ctx context.Context, limit int, now time.Time, staleBefore time.Time, owner string) ([]*model.OutboxEvent, error) {
	var records []*model.OutboxEvent

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		query := tx.Model(&model.OutboxEvent{}).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_attempt_at <= ?", model.OutboxStatusPending, now.UTC()).
			Or("status = ? AND locked_at <= ?", model.OutboxStatusPublishing, staleBefore.UTC()).
			Order("created_at ASC").
			Limit(limit)
		if err := query.Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		lockedAt := now.UTC()
		if err := tx.Model(&model.OutboxEvent{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":     model.OutboxStatusPublishing,
				"locked_by":  owner,
				"locked_at":  lockedAt,
				"updated_at": lockedAt,
			}).Error; err != nil {
			return err
		}

		return tx.Where("id IN ?", ids).Order("created_at ASC").Find(&records).Error
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (r *outboxRepository) MarkPublished(ctx context.Context, id string, publishedAt time.Time) error {
	published := publishedAt.UTC()
	return r.db.WithContext(ctx).
		Model(&model.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       model.OutboxStatusPublished,
			"published_at": published,
			"last_error":   "",
			"locked_by":    nil,
			"locked_at":    nil,
			"updated_at":   published,
		}).Error
}

func (r *outboxRepository) MarkFailed(ctx context.Context, id string, attempts int, lastError string, nextAttemptAt time.Time, dead bool) error {
	status := model.OutboxStatusPending
	if dead {
		status = model.OutboxStatusDead
	}

	return r.db.WithContext(ctx).
		Model(&model.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":          status,
			"attempts":        attempts,
			"last_error":      lastError,
			"next_attempt_at": nextAttemptAt.UTC(),
			"locked_by":       nil,
			"locked_at":       nil,
			"updated_at":      time.Now().UTC(),
		}).Error
}

func (r *outboxRepository) WithTx(tx *gorm.DB) OutboxRepository {
	return &outboxRepository{db: tx}
}
