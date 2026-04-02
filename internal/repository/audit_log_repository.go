package repository

import (
	"context"

	"github.com/possibities/gin-core/internal/model"
	"gorm.io/gorm"
)

type AuditLogRepository interface {
	Create(ctx context.Context, auditLog *model.AuditLog) error
}

type auditLogRepository struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) AuditLogRepository {
	return &auditLogRepository{db: db}
}

func (r *auditLogRepository) Create(ctx context.Context, auditLog *model.AuditLog) error {
	return r.db.WithContext(ctx).Create(auditLog).Error
}
