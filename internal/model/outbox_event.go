package model

import "time"

const (
	OutboxStatusPending    = "pending"
	OutboxStatusPublishing = "publishing"
	OutboxStatusPublished  = "published"
	OutboxStatusDead       = "dead"
)

type OutboxEvent struct {
	ID            string    `gorm:"primaryKey;size:36"`
	Topic         string    `gorm:"size:255;not null;index"`
	Payload       []byte    `gorm:"type:jsonb;not null"`
	Status        string    `gorm:"size:32;not null;index"`
	Attempts      int       `gorm:"not null"`
	NextAttemptAt time.Time `gorm:"not null;index"`
	PublishedAt   *time.Time
	LastError     string `gorm:"type:text"`
	LockedBy      string `gorm:"size:64;index"`
	LockedAt      *time.Time
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}
