package model

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Email        string `gorm:"size:255;uniqueIndex;not null"`
	Name         string `gorm:"size:128;not null"`
	Role         string `gorm:"size:64;not null"`
	TenantID     string `gorm:"size:64"`
	PasswordHash string `gorm:"size:255"`
}
