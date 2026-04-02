package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesBaseEnvAndEnvironmentVariables(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_DB_HOST", "db.internal")
	t.Setenv("APP_JWT_ACCESS_SECRET", "access-secret")
	t.Setenv("APP_JWT_REFRESH_SECRET", "refresh-secret")
	t.Setenv("APP_JWT_PREVIOUS_ACCESS_SECRET", "previous-access-secret")
	t.Setenv("APP_JWT_PREVIOUS_REFRESH_SECRET", "previous-refresh-secret")

	configDir := t.TempDir()
	writeConfigFile(t, configDir, "config.yaml", `
app:
  env: dev
  port: 8080
db:
  host: 127.0.0.1
  port: 5432
  user: postgres
  name: gin_boilerplate
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 12
  conn_max_lifetime_sec: 300
  conn_max_idle_time_sec: 180
migration:
  auto_run: false
  table: schema_migrations
  timeout_sec: 30
  statement_timeout_sec: 5
redis:
  addr: 127.0.0.1:6379
cache:
  null_ttl_sec: 30
  ttl_jitter_sec: 60
jwt:
  issuer: gin-boilerplate
log:
  level: info
  encoding: json
rbac:
  enabled: true
  autoload_interval_sec: 30
rate_limit:
  enabled: true
  rps: 20
  burst: 40
  window_sec: 1
worker:
  workers: 4
  queue_size: 128
  submit_timeout_ms: 100
storage:
  driver: local
  local_dir: data/files
  public_base_url: /files
`)
	writeConfigFile(t, configDir, "config.dev.yaml", `
app:
  port: 9090
migration:
  auto_run: true
`)
	t.Setenv("CONFIG_PATH", configDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Port != 9090 {
		t.Fatalf("expected app port from env-specific config, got %d", cfg.App.Port)
	}
	if cfg.DB.Host != "db.internal" {
		t.Fatalf("expected env var override for db.host, got %q", cfg.DB.Host)
	}
	if !cfg.Migration.AutoRun {
		t.Fatal("expected env-specific config to enable migration.auto_run")
	}
}

func TestLoadForMigrationAllowsMissingRuntimeSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "dev")

	configDir := t.TempDir()
	writeConfigFile(t, configDir, "config.yaml", `
app:
  env: dev
db:
  host: 127.0.0.1
  port: 5432
  user: postgres
  name: gin_boilerplate
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 12
  conn_max_lifetime_sec: 300
  conn_max_idle_time_sec: 180
migration:
  auto_run: false
  table: schema_migrations
  timeout_sec: 30
  statement_timeout_sec: 5
cache:
  null_ttl_sec: 30
  ttl_jitter_sec: 60
`)
	t.Setenv("CONFIG_PATH", configDir)

	cfg, err := LoadForMigration()
	if err != nil {
		t.Fatalf("LoadForMigration() error = %v", err)
	}

	if cfg.Migration.Table != "schema_migrations" {
		t.Fatalf("expected migration table to load, got %q", cfg.Migration.Table)
	}
}

func TestLoadRejectsSecretsFromConfigFiles(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_JWT_ACCESS_SECRET", "access-secret")
	t.Setenv("APP_JWT_REFRESH_SECRET", "refresh-secret")

	configDir := t.TempDir()
	writeConfigFile(t, configDir, "config.yaml", `
app:
  env: dev
db:
  host: 127.0.0.1
  port: 5432
  user: postgres
  name: gin_boilerplate
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 12
  conn_max_lifetime_sec: 300
  conn_max_idle_time_sec: 180
migration:
  auto_run: false
  table: schema_migrations
  timeout_sec: 30
  statement_timeout_sec: 5
redis:
  addr: 127.0.0.1:6379
cache:
  null_ttl_sec: 30
  ttl_jitter_sec: 60
jwt:
  issuer: gin-boilerplate
  access_secret: should-not-be-here
  previous_access_secret: should-not-be-here-either
log:
  level: info
  encoding: json
rbac:
  enabled: true
  autoload_interval_sec: 30
rate_limit:
  enabled: true
  rps: 20
  burst: 40
  window_sec: 1
worker:
  workers: 4
  queue_size: 128
  submit_timeout_ms: 100
storage:
  driver: local
  local_dir: data/files
  public_base_url: /files
`)
	t.Setenv("CONFIG_PATH", configDir)

	if _, err := Load(); err == nil {
		t.Fatal("expected Load() to reject file-backed secrets")
	}
}

func TestValidateRejectsNonJSONEncoding(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Name:               "gin-boilerplate",
			Port:               8080,
			ReadTimeoutSec:     10,
			WriteTimeoutSec:    10,
			ShutdownTimeoutSec: 30,
		},
		DB: DBConfig{
			Host:               "127.0.0.1",
			Port:               5432,
			User:               "postgres",
			Name:               "gin_boilerplate",
			SSLMode:            "disable",
			DialTimeoutSec:     2,
			MaxOpenConns:       25,
			MaxIdleConns:       12,
			ConnMaxLifetimeSec: 300,
			ConnMaxIdleTimeSec: 180,
		},
		Migration: MigrationConfig{
			AutoRun:             true,
			Table:               "schema_migrations",
			TimeoutSec:          30,
			StatementTimeoutSec: 5,
		},
		Redis: RedisConfig{
			Addr:            "127.0.0.1:6379",
			DialTimeoutSec:  2,
			ReadTimeoutSec:  2,
			WriteTimeoutSec: 2,
		},
		Cache: CacheConfig{
			NullTTLSec:   30,
			TTLJitterSec: 60,
		},
		JWT: JWTConfig{
			Issuer:        "gin-boilerplate",
			AccessTTLSec:  7200,
			RefreshTTLSec: 604800,
			AccessSecret:  "access-secret",
			RefreshSecret: "refresh-secret",
		},
		Log: LogConfig{
			Level:    "info",
			Encoding: "console",
		},
		Tracing: TracingConfig{
			Enabled:     true,
			SampleRatio: 1,
			TimeoutSec:  5,
		},
		RBAC: RBACConfig{
			Enabled:             true,
			AutoLoadIntervalSec: 30,
		},
		RateLimit: RateLimitConfig{
			Enabled:   true,
			RPS:       1,
			Burst:     1,
			WindowSec: 1,
		},
		Worker: WorkerConfig{
			Workers:         4,
			QueueSize:       128,
			SubmitTimeoutMs: 100,
		},
		Storage: StorageConfig{
			Driver:        "local",
			LocalDir:      "data/files",
			PublicBaseURL: "/files",
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate() to reject non-json log encoding")
	}
}

func TestValidateRejectsInvalidMigrationConfig(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Name:               "gin-boilerplate",
			Port:               8080,
			ReadTimeoutSec:     10,
			WriteTimeoutSec:    10,
			ShutdownTimeoutSec: 30,
		},
		DB: DBConfig{
			Host:               "127.0.0.1",
			Port:               5432,
			User:               "postgres",
			Name:               "gin_boilerplate",
			SSLMode:            "disable",
			DialTimeoutSec:     2,
			MaxOpenConns:       25,
			MaxIdleConns:       12,
			ConnMaxLifetimeSec: 300,
			ConnMaxIdleTimeSec: 180,
		},
		Migration: MigrationConfig{
			AutoRun:             true,
			Table:               "",
			TimeoutSec:          0,
			StatementTimeoutSec: 0,
		},
		Redis: RedisConfig{
			Addr:            "127.0.0.1:6379",
			DialTimeoutSec:  2,
			ReadTimeoutSec:  2,
			WriteTimeoutSec: 2,
		},
		Cache: CacheConfig{
			NullTTLSec:   30,
			TTLJitterSec: 60,
		},
		JWT: JWTConfig{
			Issuer:        "gin-boilerplate",
			AccessTTLSec:  7200,
			RefreshTTLSec: 604800,
			AccessSecret:  "access-secret",
			RefreshSecret: "refresh-secret",
		},
		Log: LogConfig{
			Level:    "info",
			Encoding: "json",
		},
		Tracing: TracingConfig{
			Enabled:     true,
			SampleRatio: 1,
			TimeoutSec:  5,
		},
		RBAC: RBACConfig{
			Enabled:             true,
			AutoLoadIntervalSec: 30,
		},
		RateLimit: RateLimitConfig{
			Enabled:   true,
			RPS:       1,
			Burst:     1,
			WindowSec: 1,
		},
		Worker: WorkerConfig{
			Workers:         4,
			QueueSize:       128,
			SubmitTimeoutMs: 100,
		},
		Storage: StorageConfig{
			Driver:        "local",
			LocalDir:      "data/files",
			PublicBaseURL: "/files",
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate() to reject invalid migration config")
	}
}

func TestValidateRejectsInvalidCacheConfig(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Name:               "gin-boilerplate",
			Port:               8080,
			ReadTimeoutSec:     10,
			WriteTimeoutSec:    10,
			ShutdownTimeoutSec: 30,
		},
		DB: DBConfig{
			Host:               "127.0.0.1",
			Port:               5432,
			User:               "postgres",
			Name:               "gin_boilerplate",
			SSLMode:            "disable",
			DialTimeoutSec:     2,
			MaxOpenConns:       25,
			MaxIdleConns:       12,
			ConnMaxLifetimeSec: 300,
			ConnMaxIdleTimeSec: 180,
		},
		Migration: MigrationConfig{
			AutoRun:             false,
			Table:               "schema_migrations",
			TimeoutSec:          30,
			StatementTimeoutSec: 5,
		},
		Redis: RedisConfig{
			Addr:            "127.0.0.1:6379",
			DialTimeoutSec:  2,
			ReadTimeoutSec:  2,
			WriteTimeoutSec: 2,
		},
		Cache: CacheConfig{
			NullTTLSec:   0,
			TTLJitterSec: -1,
		},
		JWT: JWTConfig{
			Issuer:        "gin-boilerplate",
			AccessTTLSec:  7200,
			RefreshTTLSec: 604800,
			AccessSecret:  "access-secret",
			RefreshSecret: "refresh-secret",
		},
		Log: LogConfig{
			Level:    "info",
			Encoding: "json",
		},
		Tracing: TracingConfig{
			Enabled:     true,
			SampleRatio: 1,
			TimeoutSec:  5,
		},
		RBAC: RBACConfig{
			Enabled:             true,
			AutoLoadIntervalSec: 30,
		},
		RateLimit: RateLimitConfig{
			Enabled:   true,
			RPS:       1,
			Burst:     1,
			WindowSec: 1,
		},
		Worker: WorkerConfig{
			Workers:         4,
			QueueSize:       128,
			SubmitTimeoutMs: 100,
		},
		Storage: StorageConfig{
			Driver:        "local",
			LocalDir:      "data/files",
			PublicBaseURL: "/files",
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate() to reject invalid cache config")
	}
}

func TestValidateRejectsInvalidWorkerConfig(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Name:               "gin-boilerplate",
			Port:               8080,
			ReadTimeoutSec:     10,
			WriteTimeoutSec:    10,
			ShutdownTimeoutSec: 30,
		},
		DB: DBConfig{
			Host:               "127.0.0.1",
			Port:               5432,
			User:               "postgres",
			Name:               "gin_boilerplate",
			SSLMode:            "disable",
			DialTimeoutSec:     2,
			MaxOpenConns:       25,
			MaxIdleConns:       12,
			ConnMaxLifetimeSec: 300,
			ConnMaxIdleTimeSec: 180,
		},
		Migration: MigrationConfig{
			Table:               "schema_migrations",
			TimeoutSec:          30,
			StatementTimeoutSec: 5,
		},
		Redis: RedisConfig{
			Addr:            "127.0.0.1:6379",
			DialTimeoutSec:  2,
			ReadTimeoutSec:  2,
			WriteTimeoutSec: 2,
		},
		Cache: CacheConfig{
			NullTTLSec:   30,
			TTLJitterSec: 60,
		},
		JWT: JWTConfig{
			Issuer:        "gin-boilerplate",
			AccessTTLSec:  7200,
			RefreshTTLSec: 604800,
			AccessSecret:  "access-secret",
			RefreshSecret: "refresh-secret",
		},
		Log: LogConfig{
			Level:    "info",
			Encoding: "json",
		},
		Tracing: TracingConfig{
			Enabled:     true,
			SampleRatio: 1,
			TimeoutSec:  5,
		},
		RBAC: RBACConfig{
			Enabled:             true,
			AutoLoadIntervalSec: 30,
		},
		RateLimit: RateLimitConfig{
			Enabled:   true,
			RPS:       1,
			Burst:     1,
			WindowSec: 1,
		},
		Worker: WorkerConfig{
			Workers:         0,
			QueueSize:       0,
			SubmitTimeoutMs: 0,
		},
		Storage: StorageConfig{
			Driver:        "local",
			LocalDir:      "data/files",
			PublicBaseURL: "/files",
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate() to reject invalid worker config")
	}
}

func TestValidateRejectsInvalidStorageConfig(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Name:               "gin-boilerplate",
			Port:               8080,
			ReadTimeoutSec:     10,
			WriteTimeoutSec:    10,
			ShutdownTimeoutSec: 30,
		},
		DB: DBConfig{
			Host:               "127.0.0.1",
			Port:               5432,
			User:               "postgres",
			Name:               "gin_boilerplate",
			SSLMode:            "disable",
			DialTimeoutSec:     2,
			MaxOpenConns:       25,
			MaxIdleConns:       12,
			ConnMaxLifetimeSec: 300,
			ConnMaxIdleTimeSec: 180,
		},
		Migration: MigrationConfig{
			Table:               "schema_migrations",
			TimeoutSec:          30,
			StatementTimeoutSec: 5,
		},
		Redis: RedisConfig{
			Addr:            "127.0.0.1:6379",
			DialTimeoutSec:  2,
			ReadTimeoutSec:  2,
			WriteTimeoutSec: 2,
		},
		Cache: CacheConfig{
			NullTTLSec:   30,
			TTLJitterSec: 60,
		},
		JWT: JWTConfig{
			Issuer:        "gin-boilerplate",
			AccessTTLSec:  7200,
			RefreshTTLSec: 604800,
			AccessSecret:  "access-secret",
			RefreshSecret: "refresh-secret",
		},
		Log: LogConfig{
			Level:    "info",
			Encoding: "json",
		},
		Tracing: TracingConfig{
			Enabled:     true,
			SampleRatio: 1,
			TimeoutSec:  5,
		},
		RBAC: RBACConfig{
			Enabled:             true,
			AutoLoadIntervalSec: 30,
		},
		RateLimit: RateLimitConfig{
			Enabled:   true,
			RPS:       1,
			Burst:     1,
			WindowSec: 1,
		},
		Worker: WorkerConfig{
			Workers:         4,
			QueueSize:       128,
			SubmitTimeoutMs: 100,
		},
		Storage: StorageConfig{
			Driver:        "unsupported",
			LocalDir:      "",
			PublicBaseURL: "",
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate() to reject invalid storage config")
	}
}

func writeConfigFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
