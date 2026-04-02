package service

import (
	"context"

	"github.com/possibities/gin-core/internal/model"
	"github.com/possibities/gin-core/internal/repository"
)

type AuditRecord struct {
	ActorUserID  uint
	Action       string
	ResourceType string
	ResourceID   string
	BeforeState  string
	AfterState   string
	IPAddress    string
	TraceID      string
}

type AuditService struct {
	auditLogs repository.AuditLogRepository
}

func NewAuditService(auditLogs repository.AuditLogRepository) *AuditService {
	return &AuditService{auditLogs: auditLogs}
}

func (s *AuditService) Record(ctx context.Context, record AuditRecord) error {
	return s.auditLogs.Create(ctx, &model.AuditLog{
		ActorUserID:  record.ActorUserID,
		Action:       record.Action,
		ResourceType: record.ResourceType,
		ResourceID:   record.ResourceID,
		BeforeState:  record.BeforeState,
		AfterState:   record.AfterState,
		IPAddress:    record.IPAddress,
		TraceID:      record.TraceID,
	})
}
