package model

import "time"

type AuditLog struct {
	ID           uint      `gorm:"primaryKey"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	ActorUserID  uint      `gorm:"not null"`
	Action       string    `gorm:"size:128;not null"`
	ResourceType string    `gorm:"size:128;not null"`
	ResourceID   string    `gorm:"size:128;not null"`
	BeforeState  string    `gorm:"type:text;not null;default:''"`
	AfterState   string    `gorm:"type:text;not null;default:''"`
	IPAddress    string    `gorm:"size:64;not null"`
	TraceID      string    `gorm:"size:64;not null"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
