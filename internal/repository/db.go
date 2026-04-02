package repository

import (
	"time"

	"github.com/possibities/gin-boilerplate/pkg/config"
	"github.com/possibities/gin-boilerplate/pkg/metrics"
	pkgtracing "github.com/possibities/gin-boilerplate/pkg/tracing"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

func NewDB(cfg *config.Config, tracingProvider *pkgtracing.Provider, metricsRegistry *metrics.Registry) (*gorm.DB, func(), error) {
	db, err := gorm.Open(postgres.Open(cfg.DB.DSN()), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, nil, err
	}

	if len(cfg.DB.Replicas) > 0 {
		replicas := make([]gorm.Dialector, 0, len(cfg.DB.Replicas))
		for _, r := range cfg.DB.Replicas {
			replicas = append(replicas, postgres.Open(r.DSN(cfg.DB)))
		}
		if err := db.Use(dbresolver.Register(dbresolver.Config{
			Replicas: replicas,
			Policy:   dbresolver.RandomPolicy{},
		}).SetMaxOpenConns(cfg.DB.MaxOpenConns).
			SetMaxIdleConns(cfg.DB.MaxIdleConns).
			SetConnMaxLifetime(time.Duration(cfg.DB.ConnMaxLifetimeSec) * time.Second).
			SetConnMaxIdleTime(time.Duration(cfg.DB.ConnMaxIdleTimeSec) * time.Second),
		); err != nil {
			return nil, nil, err
		}
	}

	if err := pkgtracing.RegisterGORMCallbacks(db, tracingProvider, metricsRegistry); err != nil {
		return nil, nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.DB.ConnMaxLifetimeSec) * time.Second)
	sqlDB.SetConnMaxIdleTime(time.Duration(cfg.DB.ConnMaxIdleTimeSec) * time.Second)

	cleanup := func() {
		_ = sqlDB.Close()
	}

	return db, cleanup, nil
}
