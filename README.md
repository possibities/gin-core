# gin-core

Production-ready Go backend boilerplate built on Gin, following a strict layered architecture (Handler → Service → Repository) with compile-time dependency injection via Wire.

## Architecture

```
cmd/server/          Entry point
internal/
  handler/           HTTP layer: bind → call service → respond
  service/           Business logic, transactions, domain events
  repository/        Data access (GORM), read/write split
  middleware/        Auth, RBAC, rate limit, tracing, metrics, timeout
  event/             In-process event bus + transactional outbox
  worker/            Bounded async worker pool
  scheduler/         Asynq periodic tasks with distributed lock
pkg/
  jwt/               Token generation, parsing, kid-based key rotation
  cache/             Redis read-through, singleflight, distributed lock with watchdog
  config/            Viper 3-layer config (YAML + ENV + runtime secrets)
  errors/            Typed BizError with HTTP/business code mapping
  response/          Unified JSON response format
  metrics/           Prometheus registry and observation helpers
  tracing/           OpenTelemetry setup, GORM/Redis hooks
  storage/           File storage abstraction (local, S3)
  i18n/              Error message localization (en, zh-CN)
  mq/                Message queue publisher (RabbitMQ)
```

## Quick Start

### Docker Compose (recommended)

```bash
docker compose up -d
```

Starts PostgreSQL 16, Redis 7, and the app on `:8080` with auto-migration.

### Manual

```bash
# Prerequisites: Go 1.24+, PostgreSQL, Redis

# Set required secrets
export APP_JWT_ACCESS_SECRET=your-access-secret
export APP_JWT_REFRESH_SECRET=your-refresh-secret

# Run
go run ./cmd/server
```

## Configuration

Three-layer model:

| Layer | Source | Example |
|-------|--------|---------|
| Static | `config/config.yaml` | timeouts, pool sizes |
| Environment | `APP_*` env vars | `APP_DB_HOST`, `APP_REDIS_ADDR` |
| Secrets | Runtime injection only | `APP_JWT_ACCESS_SECRET` |

Secrets are **never** stored in config files. `config.Validate()` panics on missing required values at startup.

## Key Features

### Authentication & Authorization
- JWT access/refresh token pair with `kid` header for zero-downtime key rotation
- Redis-backed token blacklist for forced logout
- Casbin RBAC with DB-backed policy hot-reload

### Caching
- Redis read-through cache with singleflight (prevents thundering herd)
- Null caching (prevents cache penetration)
- TTL jitter (prevents cache avalanche)
- Delayed double-delete for write-path consistency

### Distributed Lock
- Redis SET NX with Lua-based atomic release
- Watchdog auto-renewal for long-running critical sections

### Rate Limiting
- Redis sliding window (Lua script, atomic)
- RFC 6585/7231 response headers (`X-RateLimit-*`, `Retry-After`)
- Degrades open on Redis failure

### Observability
- Structured JSON logging (Zap) with trace ID on every entry
- Prometheus metrics: HTTP, DB, cache, worker queue, goroutines
- OpenTelemetry tracing with GORM and Redis hooks
- Alertmanager rules for error rate, P99 latency, cache hit rate

### Data Layer
- GORM with connection pool tuning and read/write split via DBResolver
- Transactional outbox for reliable event publishing
- golang-migrate for versioned schema migrations

### Async Processing
- Bounded worker pool with backpressure
- Asynq scheduled tasks with distributed lock
- In-process event bus with outbox dispatcher to MQ

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/healthz` | No | Liveness probe |
| GET | `/readyz` | No | Readiness probe (DB + Redis) |
| GET | `/metrics` | No | Prometheus metrics |
| GET | `/swagger/*` | No | Swagger UI (dev/staging only) |
| GET | `/api/v1/session` | Bearer | Current session |
| GET | `/api/v1/users/me` | Bearer | User profile |
| PATCH | `/api/v1/users/me` | Bearer | Update profile |
| GET | `/api/v1/admin/session` | Bearer + RBAC | Admin session (audited) |

## Middleware Chain

Registered in this exact order:

1. **Recovery** — catch panics
2. **RequestID** — generate trace ID
3. **Timeout** — 30s parent context deadline
4. **Tracing** — OpenTelemetry span
5. **Metrics** — request instrumentation
6. **Logger** — structured access log
7. **Locale** — i18n from Accept-Language
8. **CORS** — preflight handling
9. **RateLimiter** — sliding window per IP
10. **Auth** — JWT validation (route-group scoped)
11. **RBAC** — Casbin enforcement (admin-group scoped)

## Testing

```bash
go test ./...
```

27 test files across 15 packages covering handler, service, middleware, cache, config, JWT, tracing, and more.

Coverage gate: `internal/service/` must maintain ≥70% (enforced in CI).

## CI

GitHub Actions runs on every push to `master`:

- golangci-lint (errcheck, gosec, contextcheck, etc.)
- go vet
- Unit tests
- Service coverage gate (≥70%)
- govulncheck
- Wire generation drift check
- Swagger generation drift check

## Project Structure Conventions

- `internal/` is Go-private — no external imports allowed
- `pkg/` contains only generic utilities, no business logic
- Handlers do exactly three things: bind, call service, respond
- All secrets injected at runtime via env vars, never in config files
- Conventional Commits: `feat:`, `fix:`, `refactor:`, `test:`, `chore:`

## License

MIT
