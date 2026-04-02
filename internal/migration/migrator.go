package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq" // Register postgres driver for database/sql migrations.
	projectmigrations "github.com/possibities/gin-boilerplate/migrations"
	"github.com/possibities/gin-boilerplate/pkg/config"
)

type Migrator struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Migrator {
	return &Migrator{cfg: cfg}
}

func (m *Migrator) AutoRunEnabled() bool {
	return m.cfg.Migration.AutoRun
}

func (m *Migrator) Up(ctx context.Context) error {
	runner, err := m.open(ctx)
	if err != nil {
		return err
	}
	defer runner.close()

	if err := runner.migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply up migrations: %w", err)
	}
	return nil
}

func (m *Migrator) Down(ctx context.Context) error {
	runner, err := m.open(ctx)
	if err != nil {
		return err
	}
	defer runner.close()

	if err := runner.migrator.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply down migrations: %w", err)
	}
	return nil
}

func (m *Migrator) Steps(ctx context.Context, steps int) error {
	runner, err := m.open(ctx)
	if err != nil {
		return err
	}
	defer runner.close()

	if err := runner.migrator.Steps(steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migration steps: %w", err)
	}
	return nil
}

func (m *Migrator) Version(ctx context.Context) (uint, bool, error) {
	runner, err := m.open(ctx)
	if err != nil {
		return 0, false, err
	}
	defer runner.close()

	version, dirty, err := runner.migrator.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read migration version: %w", err)
	}
	return version, dirty, nil
}

func (m *Migrator) Timeout() time.Duration {
	return time.Duration(m.cfg.Migration.TimeoutSec) * time.Second
}

type runner struct {
	migrator *migrate.Migrate
}

func (r *runner) close() {
	if r == nil || r.migrator == nil {
		return
	}
	_, _ = r.migrator.Close()
}

func (m *Migrator) open(ctx context.Context) (*runner, error) {
	db, err := sql.Open("postgres", m.cfg.DB.DSN())
	if err != nil {
		return nil, fmt.Errorf("open migration db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Duration(m.cfg.DB.ConnMaxLifetimeSec) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(m.cfg.DB.ConnMaxIdleTimeSec) * time.Second)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping migration db: %w", err)
	}

	sourceDriver, err := iofs.New(projectmigrations.Files, ".")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}

	databaseDriver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{
		MigrationsTable:  m.cfg.Migration.Table,
		DatabaseName:     m.cfg.DB.Name,
		StatementTimeout: time.Duration(m.cfg.Migration.StatementTimeoutSec) * time.Second,
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create postgres migration driver: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migrator: %w", err)
	}

	return &runner{migrator: migrator}, nil
}
