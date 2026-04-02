package service

import (
	"context"
	"testing"

	"github.com/possibities/gin-boilerplate/internal/model"
)

type stubAuditLogRepository struct {
	created *model.AuditLog
}

func (r *stubAuditLogRepository) Create(_ context.Context, auditLog *model.AuditLog) error {
	r.created = auditLog
	return nil
}

func TestAuditServiceRecordMapsFields(t *testing.T) {
	repo := &stubAuditLogRepository{}
	svc := NewAuditService(repo)

	err := svc.Record(context.Background(), AuditRecord{
		ActorUserID:  42,
		Action:       "admin.session.view",
		ResourceType: "session",
		ResourceID:   "current",
		BeforeState:  "{}",
		AfterState:   `{"scope":"admin"}`,
		IPAddress:    "127.0.0.1",
		TraceID:      "trace-123",
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if repo.created == nil {
		t.Fatal("expected audit log to be written")
	}
	if repo.created.ActorUserID != 42 || repo.created.Action != "admin.session.view" || repo.created.TraceID != "trace-123" {
		t.Fatalf("unexpected audit log: %+v", repo.created)
	}
}
