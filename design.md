# Enterprise Go/Gin Backend — Architecture Design & Strategy

> **Purpose**: This document is the authoritative design reference for the project. Every module, pattern, and decision recorded here must be followed consistently. All code changes must align with this document as the ground truth for architectural intent.

------

## Table of Contents

1. [Core Architecture Principles](#1-core-architecture-principles)
2. [Project Structure](#2-project-structure)
3. [Configuration Management](#3-configuration-management)
4. [Middleware Chain](#4-middleware-chain)
5. [Authentication — JWT](#5-authentication--jwt)
6. [Authorization — RBAC](#6-authorization--rbac)
7. [Error Handling System](#7-error-handling-system)
8. [Handler Layer](#8-handler-layer)
9. [Service Layer](#9-service-layer)
10. [Repository Layer & GORM](#10-repository-layer--gorm)
11. [Cache Strategy — Redis](#11-cache-strategy--redis)
12. [Rate Limiting](#12-rate-limiting)
13. [Async Processing — Worker Pool & MQ](#13-async-processing--worker-pool--mq)
14. [Scheduled Tasks](#14-scheduled-tasks)
15. [File Storage](#15-file-storage)
16. [Internationalization (i18n)](#16-internationalization-i18n)
17. [Observability — Logging, Metrics, Tracing](#17-observability--logging-metrics-tracing)
18. [Security](#18-security)
19. [API Documentation — Swagger](#19-api-documentation--swagger)
20. [Dependency Injection — Wire](#20-dependency-injection--wire)
21. [Database Migrations](#21-database-migrations)
22. [Graceful Shutdown & Health Check](#22-graceful-shutdown--health-check)
23. [Testing Strategy](#23-testing-strategy)
24. [CI/CD & Quality Gates](#24-cicd--quality-gates)
25. [Module Priority & Rollout Order](#25-module-priority--rollout-order)

------

## 1. Core Architecture Principles

These principles are non-negotiable. Every code generation decision must be consistent with them.

### 1.1 Unidirectional Dependency

```
Handler → Service → Repository → Database
```

- No reverse dependencies. Repository must never import Service; Service must never import Handler.
- If a circular dependency appears, it signals a missing abstraction layer. Introduce a new interface or domain object to resolve it.

### 1.2 Interface Over Implementation

- Service layer holds `Repository` **interfaces**, not concrete struct pointers.
- This enables test-time Mock injection without a real database.
- Wire handles interface-to-implementation binding at compile time.

### 1.3 Failure Is First-Class

- Every external call (DB, Redis, HTTP, MQ) **must** have: timeout control, error handling, and a degradation strategy.
- Never write only the happy path. Always define: "what does the system do when X is unavailable?"

**Recommended Timeout Reference:**

| Call Type              | Timeout   | Notes                                            |
| ---------------------- | --------- | ------------------------------------------------ |
| DB query (simple)      | 3 s       | Single-table CRUD                                |
| DB query (complex)     | 5 s       | Joins, aggregation, reporting queries            |
| DB transaction         | 10 s      | Multi-step transactions                          |
| Redis read/write       | 1 s       | Single key operations                            |
| Redis pipeline/Lua     | 3 s       | Batch operations and scripts                     |
| Outbound HTTP          | 5–10 s    | Adjust per upstream SLA                          |
| MQ publish             | 3 s       | Fail fast; queue retries via outbox              |
| MQ consume processing  | 30 s      | Per-message; extend via heartbeat if needed      |
| File upload (S3/OSS)   | 30 s      | Depends on file size; use multipart for > 50 MB  |

All timeouts are enforced via `context.WithTimeout`. The parent HTTP request context (set by Gin) should have an overall ceiling of **30 s**; individual call timeouts must be shorter than the parent.

### 1.4 Observability Before Features

- TraceID, structured logging, and core metrics must be live **before** the first feature ships.
- Retrofitting observability after the fact is expensive and always discovered at the worst moment — during a production incident.

### 1.5 Stateless Process

- The process itself holds zero state. No in-memory sessions. No local caches that diverge across instances.
- All state lives in Redis or the database.
- Any instance can be destroyed and recreated at any time without business impact. This is the prerequisite for horizontal scaling.

### 1.6 Explicit Over Magic

- Prefer `wire` compile-time DI over reflection-based IoC containers.
- Prefer explicit `context.Context` propagation over goroutine-local storage.
- Prefer typed error codes over unstructured `error` strings.

------

## 2. Project Structure

```
project/
├── cmd/
│   └── server/
│       └── main.go              # Entry point: init config, logger, wire, http server
├── internal/
│   ├── handler/                 # HTTP layer: bind, validate, respond
│   │   ├── user_handler.go
│   │   └── order_handler.go
│   ├── service/                 # Business logic layer
│   │   ├── user_service.go
│   │   └── order_service.go
│   ├── repository/              # Data access layer
│   │   ├── user_repo.go
│   │   └── order_repo.go
│   ├── model/                   # Domain models (GORM structs)
│   │   ├── user.go
│   │   └── order.go
│   ├── middleware/              # Gin middleware
│   │   ├── auth.go
│   │   ├── rbac.go
│   │   ├── logger.go
│   │   ├── recovery.go
│   │   ├── ratelimit.go
│   │   ├── cors.go
│   │   └── trace.go
│   ├── router/                  # Route registration
│   │   └── router.go
│   ├── worker/                  # Async worker pool
│   │   └── pool.go
│   ├── scheduler/               # Cron / distributed jobs
│   │   └── scheduler.go
│   └── wire.go                  # Wire provider sets
├── pkg/                         # Reusable, project-agnostic packages
│   ├── response/                # Unified HTTP response
│   ├── errors/                  # Business error codes
│   ├── jwt/                     # Token generate/parse
│   ├── logger/                  # Zap wrapper
│   ├── cache/                   # Redis cache interface
│   ├── config/                  # Config struct + loader
│   ├── storage/                 # File storage abstraction
│   ├── i18n/                    # Internationalization
│   └── tracing/                 # OpenTelemetry helpers
├── migrations/                  # SQL migration files (golang-migrate)
├── config/
│   ├── config.yaml              # Base config (no secrets)
│   ├── config.dev.yaml
│   └── config.prod.yaml
├── docs/                        # Swagger generated output
├── scripts/                     # Build, migration, deployment scripts
├── Makefile
└── docker-compose.yaml
```

**Rules:**

- `internal/` is Go-private. External packages cannot import it.
- `pkg/` contains only generic utilities with no business logic.
- Never put business logic in `handler/`. Handlers do exactly three things: bind input, call service, write response.

------

## 3. Configuration Management

### 3.1 Three-Layer Model

| Layer              | Storage                                | Example                         |
| ------------------ | -------------------------------------- | ------------------------------- |
| Static config      | YAML file (version-controlled)         | log format, timeout values      |
| Environment config | Environment variables (override YAML)  | DB host, Redis address          |
| Secret config      | Runtime secret injection (Vault / K8s Secret / CI secret) | passwords, JWT secret, API keys |

**Never commit secrets to any file.** The YAML file must be safe to open-source.

### 3.2 Loading Strategy

- Use `viper` as the unified config reader. It merges YAML + ENV transparently.
- ENV variable naming convention: `APP_DB_HOST` maps to `db.host` in YAML (viper handles this automatically with `SetEnvKeyReplacer`).
- On startup, call `config.Validate()` immediately. If any required field is missing or invalid, `panic` immediately — do not allow the process to start in a broken state.

### 3.3 Config Struct Design

```go
// pkg/config/config.go
type Config struct {
    App      AppConfig
    DB       DBConfig
    Redis    RedisConfig
    JWT      JWTConfig
    Log      LogConfig
    Tracing  TracingConfig
}

type DBConfig struct {
    Host         string // env: APP_DB_HOST
    Port         int    // env: APP_DB_PORT
    Name         string // env: APP_DB_NAME
    User         string // env: APP_DB_USER
    Password     string // secret: runtime-only, never in YAML
    MaxOpenConns int
    MaxIdleConns int
}

type JWTConfig struct {
    Issuer                string
    AccessTTLSec          int    // seconds; default 7200 (2 hours)
    RefreshTTLSec         int    // seconds; default 604800 (7 days)
    AccessSecret          string // secret: runtime-only, never in YAML — see §5.2 for rotation
    RefreshSecret         string // secret: runtime-only, never in YAML
    PreviousAccessSecret  string // secret: runtime-only — non-empty only during key rotation grace period
    PreviousRefreshSecret string // secret: runtime-only — non-empty only during key rotation grace period
}
```

Fields marked `// secret: runtime-only` must be provided exclusively via Vault, K8s Secret, or equivalent. `config.Validate()` must verify these fields are non-empty at startup and panic if missing — this prevents the service from starting with an incomplete secret configuration.

- All config is typed. No `viper.GetString("db.host")` scattered in business code.
- Config struct is passed via dependency injection. Never call `viper.Get*` outside `pkg/config/`.

------

## 4. Middleware Chain

### 4.1 Registration Order (Mandatory)

```go
r := gin.New() // Never gin.Default() - take explicit control

r.Use(
    middleware.Recovery(),             // 1st: catch panics from the entire chain
    middleware.RequestID(),            // 2nd: establish trace_id and request-scoped logger context
    middleware.Timeout(cfg),           // 3rd: set 30s parent context deadline for all downstream calls
    middleware.Tracing(provider),      // 4th: start root span after trace_id exists
    middleware.Metrics(registry),      // 5th: measure the whole request lifecycle
    middleware.ZapLogger(),            // 6th: write structured access logs with trace_id
    middleware.Locale(translator),     // 7th: resolve locale before handlers and response mappers run
    middleware.CORS(),                 // 8th: satisfy preflight before auth or handlers reject it
    middleware.RateLimiter(cfg, ...),  // 9th: shed abusive traffic before token validation
)

api := r.Group("/api/v1")
api.Use(middleware.Auth(tokenManager))

adminGroup := api.Group("/admin")
adminGroup.Use(middleware.RBAC(enforcer))
```

**Rationale for each position:**

- Recovery stays first so any downstream panic is caught.
- RequestID must run before tracing, metrics, and logging so all three can attach the same trace identifier.
- Timeout sets the parent context deadline (30s) early so all downstream middleware and handlers inherit it. Individual call timeouts (DB, Redis) must be shorter than this ceiling.
- Tracing starts before business middleware so spans cover auth, handlers, and persistence.
- Metrics wraps the whole request so latency includes middleware and handler time.
- Locale runs before handlers and response helpers so validation and BizError messages can be localized.
- CORS stays ahead of auth so browser preflight requests are not blocked.
- RateLimit stays ahead of Auth to avoid spending CPU on token validation during flood traffic.
- Auth is route-group scoped, not global, so public endpoints such as `/healthz`, `/readyz`, `/metrics`, and `/swagger/*any` remain reachable.
- RBAC is route-group scoped under authenticated modules; it is never registered globally.

### 4.2 Middleware Scope

```go
// Global: applies to every route
r.Use(middleware.Recovery(), middleware.RequestID(), middleware.Tracing(...), middleware.Metrics(...), ...)

// Authenticated API: applies to /api/v1/*
api := r.Group("/api/v1")
api.Use(middleware.Auth(tokenManager))

// Admin-only API: applies to /api/v1/admin/*
adminGroup := api.Group("/admin")
adminGroup.Use(middleware.RBAC(enforcer))

// Public endpoints stay outside the authenticated groups
r.GET("/healthz", healthHandler.Healthz)
r.GET("/metrics", gin.WrapH(metricsRegistry.Handler()))
```

------

## 5. Authentication — JWT

### 5.1 Token Architecture

| Token         | TTL     | Purpose                 |
| ------------- | ------- | ----------------------- |
| Access Token  | 2 hours | API authentication      |
| Refresh Token | 7 days  | Obtain new Access Token |

- Access Token is short-lived to limit exposure window.
- Refresh Token is stored server-side (Redis), enabling forced logout.
- Both are signed with `HS256` using secrets injected at runtime by Vault, K8s Secret, or equivalent secret manager, **not** config files.

### 5.2 Key Rotation Support

To support quarterly secret rotation (see §18.3) without logging out all users, the system maintains up to two active signing keys:

- **Primary key**: used to sign all new tokens.
- **Previous key**: accepted for verification only, during a grace period equal to the Access Token TTL (2 hours).

Each key is identified by a `kid` (Key ID) in the JWT header. The auth middleware selects the correct verification key by matching `kid`. If `kid` is missing or unknown, reject the token.

```go
// pkg/jwt/key_manager.go
type SigningKey struct {
    KID    string // unique key identifier, e.g. "2026Q2"
    Secret []byte // injected at runtime from Vault / K8s Secret
}

type KeyManager struct {
    Primary  SigningKey
    Previous *SigningKey // nil if no rotation in progress
}

// Sign always uses Primary. Verify tries Primary first, then Previous.
```

Rotation procedure:
1. Inject the new secret as `Primary` and demote the old secret to `Previous`.
2. After the grace period (≥ Access Token TTL), remove `Previous`.
3. At no point should more than two keys be active simultaneously.

### 5.3 Claims Design

```go
type Claims struct {
    UserID   uint     `json:"user_id"`
    Role     string   `json:"role"`
    TenantID string   `json:"tenant_id"` // for multi-tenant
    jwt.RegisteredClaims
}
```

The `kid` header is set during signing via `jwt.NewWithClaims` + `token.Header["kid"] = key.KID`. It is **not** part of the claims payload.

### 5.4 Auth Middleware Behavior

1. Extract `Authorization: Bearer <token>` header.
2. If missing → `401` with code `10002`.
3. Parse and validate token (signature + expiry).
4. If invalid or expired → `401` with code `10003`.
5. Check Redis blacklist (for forced logout). If present → `401` with code `10004`.
6. Inject `user_id`, `role`, `tenant_id` into `gin.Context`.
7. Call `c.Next()`.

### 5.5 Forced Logout

- On logout or password change: write the token's `jti` (JWT ID) to Redis with TTL = remaining token lifetime.
- Auth middleware checks blacklist on every request. Key: `blacklist:jwt:{jti}`.

------

## 6. Authorization — RBAC

### 6.1 Model Selection

| Scenario                            | Approach                                      |
| ----------------------------------- | --------------------------------------------- |
| ≤5 fixed roles, simple rules        | Role field in JWT claims, middleware `switch` |
| Dynamic roles, resource-level rules | Casbin with DB-backed policy                  |
| Multi-tenant, attribute-based rules | OPA (only if genuinely needed)                |

**Default choice: Casbin.** Do not use OPA unless the project explicitly requires multi-tenant attribute-based access control.

### 6.2 Casbin Policy Model

Use the `(subject, object, action)` triple:

```
p, admin, /api/v1/users/*, *
p, editor, /api/v1/posts/*, GET|POST|PUT
p, viewer, /api/v1/posts/*, GET
```

- Policies are stored in the database and hot-reloaded at runtime — no restart required when permissions change.
- Use `casbin.NewSyncedEnforcer` with `StartAutoLoadPolicy` for automatic reload.

### 6.3 Two-Layer Enforcement

```
Layer 1: Middleware — HTTP path + method level
Layer 2: Service — Row-level ("user can only see their own orders")
```

Both layers are required. Middleware alone is insufficient for row-level security.

------

## 7. Error Handling System

### 7.1 Three-Layer Error Model

```
Layer 1: BizCode    — visible to client, maps to UI messages
Layer 2: HTTP Code  — expresses HTTP semantics only
Layer 3: Internal   — full stack trace in structured log, never sent to client
```

### 7.2 BizCode Namespace

| Module            | Range       | Example                             |
| ----------------- | ----------- | ----------------------------------- |
| Common            | 10000–10099 | 10001 not found, 10002 unauthorized |
| User              | 10100–10199 | 10101 email already exists          |
| Order             | 10200–10299 | 10201 insufficient stock            |
| Internal / System | 50000–50099 | 50001 database error                |

### 7.3 Error Type

```go
type BizError struct {
    HTTPCode int
    Code     int
    Message  string
}

func (e *BizError) Error() string { return e.Message }

var (
    ErrNotFound     = &BizError{HTTPCode: 404, Code: 10001, Message: "resource not found"}
    ErrUnauthorized = &BizError{HTTPCode: 401, Code: 10002, Message: "unauthorized"}
    ErrForbidden    = &BizError{HTTPCode: 403, Code: 10003, Message: "forbidden"}
    ErrInternal     = &BizError{HTTPCode: 500, Code: 50001, Message: "internal server error"}
)
```

### 7.4 Error Flow

```
Service returns BizError
    → Handler calls errors.As() to unwrap
    → If BizError: respond with BizCode + HTTPCode
    → If unknown error: log full stack, respond with ErrInternal
    → Never expose raw error.Error() strings to client
```

### 7.5 Unified Response Structure

```json
{
  "code": 0,
  "message": "success",
  "data": { ... },
  "trace_id": "abc-123"
}
```

- `code: 0` always means success.
- On error: `code` is the BizCode, `data` is `null`.
- `trace_id` is present on every response, enabling log correlation.

------

## 8. Handler Layer

### 8.1 Responsibilities (Exactly Three)

1. **Bind** request (path params, query params, JSON body) using `c.ShouldBind*`.
2. **Call** the appropriate service method.
3. **Write** response using `response.Success()` or `response.Fail()`.

**No business logic in handlers.** If a handler is doing anything other than the above three, the logic belongs in Service.

### 8.2 Always Return After Response

```go
// WRONG — execution continues after response is written
if err != nil {
    response.Fail(c, ...)
}
doMoreStuff() // this runs even after the error response

// CORRECT
if err != nil {
    response.Fail(c, ...)
    return
}
```

### 8.3 goroutine Safety with Context

```go
// WRONG — gin context is NOT safe to use in goroutines
go func() {
    doWork(c) // c will be recycled by gin
}()

// CORRECT — copy the context first
cCopy := c.Copy()
go func() {
    doWork(cCopy)
}()
```

------

## 9. Service Layer

### 9.1 Responsibilities

- All business logic lives here.
- Orchestrate multiple repository calls within a single transaction when needed.
- Publish domain events after successful state changes.
- Return typed `BizError` for expected failure paths; return raw `error` for unexpected infrastructure failures.

### 9.2 Transaction Pattern

```go
func (s *orderService) PlaceOrder(ctx context.Context, req *dto.PlaceOrderReq) error {
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // Use tx-scoped repo, not the regular repo
        if err := s.inventoryRepo.WithTx(tx).Deduct(ctx, req.ProductID, req.Qty); err != nil {
            return err
        }
        if err := s.orderRepo.WithTx(tx).Create(ctx, order); err != nil {
            return err
        }
        return nil
    })
}
```

### 9.3 Domain Event Pattern

After a successful write, publish an event to the event bus. Consumers handle side effects (send email, update analytics, trigger downstream service) asynchronously.

```
PlaceOrder succeeds → publish OrderPlaced event
    → Worker A: send confirmation email
    → Worker B: update inventory analytics
    → Worker C: notify warehouse service via MQ
```

This decouples side effects from the core transaction, keeping Service lean.

------

## 10. Repository Layer & GORM

### 10.1 Interface Design

```go
type UserRepository interface {
    FindByID(ctx context.Context, id uint) (*model.User, error)
    FindByIDs(ctx context.Context, ids []uint) ([]*model.User, error)
    FindByEmail(ctx context.Context, email string) (*model.User, error)
    Create(ctx context.Context, user *model.User) error
    BatchCreate(ctx context.Context, users []*model.User) error
    Update(ctx context.Context, user *model.User, fields ...string) error
    Delete(ctx context.Context, id uint) error
    List(ctx context.Context, filter *dto.UserFilter) ([]*model.User, int64, error)
    WithTx(tx *gorm.DB) UserRepository
}
```

- `FindByIDs` and `BatchCreate` cover common bulk-operation patterns. Batch inserts use GORM `CreateInBatches` with a batch size of 100 to avoid oversized SQL statements.
- `WithTx` enables transaction-scoped repository usage in Service.
- Always pass `context.Context` as the first argument for timeout and tracing propagation.

### 10.2 Connection Pool (Mandatory Settings)

```go
sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)      // recommend 25–50 for production
sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)      // recommend 50% of MaxOpenConns
sqlDB.SetConnMaxLifetime(5 * time.Minute)    // prevent connection leaks
sqlDB.SetConnMaxIdleTime(3 * time.Minute)
```

### 10.3 GORM Anti-Patterns to Avoid

| Anti-Pattern                        | Problem                                 | Fix                                                       |
| ----------------------------------- | --------------------------------------- | --------------------------------------------------------- |
| `db.Updates(user)` with zero values | GORM silently ignores zero-value fields | Use `db.Model(&u).Select("field1","field2").Updates(map)` |
| `db.Delete(&User{})` without Where  | Global delete — destroys entire table   | Set `AllowGlobalUpdate: false` in GORM config             |
| Lazy loading associations           | N+1 query problem                       | Use `Preload("Orders")` or `Joins` explicitly             |
| Not using `WithContext`             | Queries can't be cancelled on timeout   | Always `db.WithContext(ctx)`                              |
| Raw string SQL interpolation        | SQL injection                           | Always use parameterized queries                          |

### 10.4 Read/Write Splitting

- Use a write (master) DB for all `INSERT`, `UPDATE`, `DELETE`.
- Use read (replica) DBs for `SELECT` queries where eventual consistency is acceptable.
- **Exception**: queries that immediately follow a write (e.g., "fetch order after placing it") must use the write DB to avoid replication lag issues.
- Implement via GORM's `ConnPool` interface or a custom DB resolver plugin.

### 10.5 Soft Delete

All domain models embed `gorm.Model`, which includes `DeletedAt`. GORM automatically filters soft-deleted records. Never hard-delete business data unless explicitly required by compliance.

------

## 11. Cache Strategy — Redis

### 11.1 Cache-Aside Pattern

```
Read:
  1. Check cache → hit → return
  2. Cache miss → query DB
  3. Write result to cache with TTL
  4. Return result

Write:
  1. Write to DB
  2. Invalidate (delete) cache key
  3. Do NOT write to cache on write path — avoid race conditions
```

**Consistency caveat:** The "write DB → delete cache" sequence has a small window where a concurrent read can re-populate stale data. Mitigation options (choose based on consistency requirements):

| Strategy          | Consistency | Complexity | When to Use                            |
| ----------------- | ----------- | ---------- | -------------------------------------- |
| Single delete     | Eventual    | Low        | Default — acceptable for most entities |
| Delayed double-delete | Strong eventual | Medium | Hot keys with frequent reads        |
| Binlog subscription (Canal/Debezium) | Near-strong | High | Critical data (inventory, balance) |

For **delayed double-delete**: after the initial delete, schedule a second delete after a short delay (e.g., 500 ms) to clear any stale cache entry that a concurrent read may have written back.

### 11.2 Three Cache Problems

| Problem           | Cause                                                    | Solution                                               |
| ----------------- | -------------------------------------------------------- | ------------------------------------------------------ |
| Cache penetration | Query for non-existent key bypasses cache, hammers DB    | Cache null result with short TTL (30s)                 |
| Cache avalanche   | Many keys expire simultaneously, flood DB                | Add random TTL jitter (±60s) to all cache writes       |
| Cache stampede    | Hot key expires, concurrent requests all miss and hit DB | `singleflight.Group` to deduplicate concurrent fetches |

### 11.3 Key Naming Convention

```
{service}:{entity}:{id}         → user:profile:12345
{service}:{entity}:list:{hash}  → product:list:a3f8c2
{service}:lock:{resource}       → order:lock:99887
blacklist:jwt:{jti}             → blacklist:jwt:abc-def-ghi
```

- Always use `:` as separator.
- Include service prefix to avoid key collisions across services.
- Document TTL for every key type in a central reference table.

### 11.4 Distributed Lock

For operations that must not run concurrently across instances (e.g., deducting stock, running a scheduled job):

```
1. SET lock_key unique_value NX PX {ttl_ms}
2. If SET fails → another instance holds the lock → abort or retry
3. Execute critical section
4. DEL lock_key ONLY if value equals unique_value (Lua script for atomicity)
```

TTL must be set to 1.5× the maximum expected execution time of the critical section.

**Lock Renewal (Watchdog):**

If the critical section may exceed the initial TTL (e.g., variable-length batch jobs), implement a watchdog goroutine:

```
1. After acquiring the lock, start a background goroutine.
2. Every TTL/3, extend the lock TTL (via Lua: check value matches, then PEXPIRE).
3. When the critical section completes (or its context is cancelled), stop the goroutine.
4. If the goroutine fails to renew (e.g., Redis unreachable), cancel the critical section context
   to prevent operating without lock protection.
```

This prevents premature lock release while ensuring the lock is not held indefinitely if the holder crashes (the original TTL still acts as a safety net).

------

## 12. Rate Limiting

### 12.1 Algorithm Selection

| Algorithm              | Use Case                                                  |
| ---------------------- | --------------------------------------------------------- |
| Token bucket           | General API rate limiting — allows burst                  |
| Sliding window (Redis) | Distributed, per-user limiting                            |
| Fixed window           | Simple, lower accuracy, acceptable for non-critical paths |

### 12.2 Granularity Levels

```
Global:       protect the entire service from overload
Per-IP:       prevent abuse from a single client
Per-user:     differentiated limits (free vs. paid tier)
Per-endpoint: expensive endpoints (export, report) get stricter limits
```

Implement global and per-IP at middleware level. Per-user and per-endpoint at route group or handler level.

### 12.3 Redis Sliding Window

Use a Lua script for atomicity. The script:

1. Removes entries older than the window.
2. Counts remaining entries.
3. If count ≥ limit → deny.
4. Otherwise → add current timestamp, set key expiry, allow.

Single Lua script execution is atomic in Redis — no race conditions.

### 12.4 Rate Limit Response Headers

All rate-limited endpoints must include the following headers in every response (not just 429):

```
X-RateLimit-Limit: 100          # max requests allowed in the window
X-RateLimit-Remaining: 42       # requests remaining in current window
X-RateLimit-Reset: 1672531200   # Unix timestamp when the window resets
```

When the limit is exceeded, return `429 Too Many Requests` with an additional header:

```
Retry-After: 30                 # seconds until the client should retry
```

This follows RFC 6585 and RFC 7231 conventions. The BizCode for rate limiting is `10005`.

------

## 13. Async Processing — Worker Pool & MQ

### 13.1 Decision Matrix

```
Message loss acceptable + same process → Worker Pool (goroutine + channel)
Message loss NOT acceptable → Message Queue (Kafka / RabbitMQ)
Cross-service communication → Message Queue (always)
```

### 13.2 Worker Pool Design

```
Job channel (buffered)
    ├── Worker 1
    ├── Worker 2
    └── Worker N

Backpressure: when channel is full, reject new jobs (return 429) or block with timeout.
Never: spawn unlimited goroutines on demand — this causes memory exhaustion under load.
```

- Pool size = number of workers. Tune based on job type: I/O-bound → more workers; CPU-bound → `runtime.NumCPU()`.
- On shutdown, drain the channel before exiting (part of graceful shutdown).

### 13.3 Message Queue Design

**Producer:**

- Use transactional outbox pattern for critical events: write event to `outbox` table in the same DB transaction as the business write, then a background process reads and publishes. This guarantees at-least-once delivery.

**Consumer:**

- Always implement idempotent processing. The same message may be delivered more than once. Use `message_id` to deduplicate (check a `processed_messages` table or Redis set).
- Use DLQ (Dead Letter Queue) for messages that fail after N retries. Never silently discard failed messages.
- Commit offset (Kafka) or ACK (RabbitMQ) **only after** successful processing.

------

## 14. Scheduled Tasks

### 14.1 Single-Instance Risk

In a multi-instance deployment, a naive cron job runs on every instance simultaneously. This causes duplicate execution, data corruption, and wasted resources.

### 14.2 Distributed Lock Approach

```
1. At trigger time, attempt to acquire Redis lock with NX + TTL
2. If acquired → execute task → release lock on completion
3. If not acquired → another instance is running it → skip silently
4. Lock TTL = 1.5 × max expected task duration
```

### 14.3 Recommended Tool: Asynq

Asynq (backed by Redis) provides:

- Periodic task scheduling with cron syntax
- Task retry with exponential backoff
- Task deduplication
- Web UI for task inspection and manual retry
- Priority queues

Use Asynq over raw `time.Ticker` cron for any production scheduled task that requires visibility or retry logic.

------

## 15. File Storage

### 15.1 Storage Abstraction

```go
type FileStorage interface {
    Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (string, error)
    Download(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
    SignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}
```

Implementations: local filesystem (dev), MinIO (self-hosted), AWS S3 / Alibaba OSS (production). Switch by config, not by changing business code.

### 15.2 Upload Security

- Validate file type by inspecting the file header magic bytes, not the MIME type from the request (easily spoofed).
- Enforce maximum file size at middleware level before reading the body.
- Store files with a server-generated UUID key, never the user-provided filename. This prevents path traversal and filename collision.
- Virus scan if handling user-uploaded executables or documents (ClamAV integration).

------

## 16. Internationalization (i18n)

### 16.1 Scope

- Error messages returned to clients
- Validation failure messages
- Email / notification templates

**Not in scope for i18n:** internal log messages (always English), audit logs.

### 16.2 Implementation

- Use `go-i18n` (nicksnyder/go-i18n).
- Message catalog files in `locales/` directory: `en.yaml`, `zh-CN.yaml`.
- Detect locale from `Accept-Language` header, with `en` as fallback.
- Inject localizer into Gin context in a middleware. Handlers and error mappers call `i18n.Localize(c, msgID)`.
- BizCode maps to a message ID, not a hardcoded string. This keeps error codes stable while messages are translatable.

------

## 17. Observability — Logging, Metrics, Tracing

The three pillars answer different questions:

- **Logging**: what happened?
- **Metrics**: how much / how often?
- **Tracing**: where did latency come from?

All three are mandatory. Implementing only logging is insufficient for production diagnosis.

### 17.1 Logging (Zap)

- Always structured JSON output. Never `fmt.Printf` in production code.
- Every log entry must carry `trace_id` (from context).
- Log levels: `DEBUG` (dev only), `INFO` (normal operations), `WARN` (recoverable anomalies), `ERROR` (needs attention + alert).
- Never log sensitive data: passwords, tokens, PII. Redact or omit.
- ERROR level logs must trigger an alert (PagerDuty / DingTalk webhook). Wire this up before first deployment.

### 17.2 Metrics (Prometheus)

Expose `/metrics` endpoint. Core metrics to instrument:

| Metric                                 | Type      | Description                     |
| -------------------------------------- | --------- | ------------------------------- |
| `http_requests_total`                  | Counter   | Labeled by method, path, status |
| `http_request_duration_seconds`        | Histogram | P50/P95/P99 latency             |
| `db_query_duration_seconds`            | Histogram | Per query type                  |
| `cache_hit_total` / `cache_miss_total` | Counter   | Cache effectiveness             |
| `goroutines_total`                     | Gauge     | Leak detection                  |
| `worker_queue_depth`                   | Gauge     | Backpressure monitoring         |

Set SLA alerts: P99 latency > 500ms → warning; error rate > 1% → critical.

### 17.3 Tracing (OpenTelemetry)

- Use OpenTelemetry SDK. Backend is pluggable (Jaeger, Zipkin, Tempo).
- Inject span in Gin middleware at request entry.
- Propagate context through all layers — `ctx` must be passed to every function call.
- Instrument: HTTP requests, DB queries (GORM hook), Redis calls, outbound HTTP calls, MQ produce/consume.
- Sampling rate: 100% in dev/staging; 10–20% in production (adjust based on volume).

### 17.4 TraceID Convention

- Generated in `RequestID` middleware using UUID v4.
- Injected into `gin.Context` as `"trace_id"`.
- Included in every response header: `X-Trace-Id`.
- Included in every log entry.
- Used as the OpenTelemetry trace ID.

------

## 18. Security

### 18.1 Transport Security

- TLS 1.2+ enforced on all external endpoints. TLS 1.0/1.1 disabled.
- Internal service-to-service communication uses mTLS where the threat model warrants it.
- HSTS header on all responses.

### 18.2 Input Security

| Attack          | Defense                                                      |
| --------------- | ------------------------------------------------------------ |
| SQL injection   | Parameterized queries only. Never string interpolation in SQL. |
| XSS             | Sanitize HTML input. Set `Content-Type: application/json`.   |
| CSRF            | Stateless JWT means standard CSRF is not applicable, but verify `Origin` header on sensitive mutations. |
| Path traversal  | UUID-based file keys. Never use user-supplied filenames in storage paths. |
| SSRF            | Whitelist allowed URL patterns for any feature that fetches external URLs. |
| Mass assignment | Explicit `Select()` on GORM updates. Never bind request body directly to model struct. |

### 18.3 Secret Management

- All secrets (DB passwords, JWT secret, API keys) must be injected at runtime by Vault, K8s Secret, or an equivalent secret manager.
- Secrets are never stored in config files, environment variable defaults, or source code; only runtime-provided values are allowed.
- Rotate JWT secret quarterly. Rotation uses the dual-key mechanism defined in §5.2 — the previous key remains valid for verification during a grace period equal to the Access Token TTL, so logged-in users are not immediately logged out.

### 18.4 Audit Logging

The following operations must write an immutable audit log record:

- User login / logout / failed login
- Permission changes
- Any data deletion
- Configuration changes
- Admin actions

Audit log schema: `(timestamp, actor_user_id, action, resource_type, resource_id, before_state, after_state, ip_address, trace_id)`.

Audit logs are written to a separate, append-only store. Application code must not have DELETE permission on audit records.

### 18.5 Dependency Security

- Run `govulncheck ./...` in CI on every pull request.
- Run `trivy image` on container images before deployment.
- Pin dependency versions in `go.sum`. Do not use `@latest` in production.

------

## 19. API Documentation — Swagger

- Use `swaggo/swag`. Annotations are written in handler files.
- Run `swag init` in CI as part of the build. Fail the build if generated docs are stale (diff check).
- Swagger UI is only exposed in `dev` and `staging` environments. Never in production.
- Every endpoint must document: summary, tags, security scheme, all parameters, success response, and all possible error responses.
- Version the API with a path prefix: `/api/v1/`. Breaking changes require `/api/v2/`.

### 19.2 API Version Migration Strategy

When a breaking change is unavoidable:

1. **Announce**: Add `Deprecation: true` and `Sunset: <date>` response headers to the old version at least 30 days before removal.
2. **Coexist**: Run v1 and v2 simultaneously. Both versions share the same Service layer — version differences are handled at the Handler layer via separate handler structs or request/response DTOs.
3. **Monitor**: Track v1 usage via the `api_version` label on `http_requests_total`. Only proceed with removal when v1 traffic drops below 1% of total.
4. **Remove**: After the sunset date, return `410 Gone` from v1 endpoints with a response body pointing to the v2 equivalent.

**Layer separation for multi-version support:**

```
internal/handler/v1/user_handler.go   # v1 request/response format
internal/handler/v2/user_handler.go   # v2 request/response format
internal/service/user_service.go      # shared — version-agnostic
internal/repository/user_repo.go      # shared — version-agnostic
```

Do not duplicate Service or Repository logic across versions. If business logic genuinely differs between versions, introduce a strategy pattern in the Service layer rather than forking the entire Service.

------

## 20. Dependency Injection — Wire

### 20.1 Why Wire

- All dependencies are resolved at compile time. A missing provider is a build error, not a runtime panic.
- The dependency graph is explicit and auditable.
- `wire graph` produces a visual dependency tree — useful for debugging and onboarding.

### 20.2 Provider Structure

```go
// internal/wire.go — provider sets grouped by layer
var RepositorySet = wire.NewSet(
    repository.NewUserRepository,
    repository.NewOrderRepository,
)

var ServiceSet = wire.NewSet(
    service.NewUserService,
    service.NewOrderService,
)

var HandlerSet = wire.NewSet(
    handler.NewUserHandler,
    handler.NewOrderHandler,
)
```

### 20.3 Rule

- Run `wire gen ./internal/` as part of every build.
- The generated `wire_gen.go` is committed to the repository (it is the artifact, not the source).
- Never call `wire.Build` outside of `wire.go` files tagged with `//go:build wireinject`.

------

## 21. Database Migrations

- Use `golang-migrate` or `goose`. All schema changes are version-controlled migration files.
- Every migration file has both an `up` and a `down` direction.
- Migration files are named: `{version}_{description}.up.sql` / `.down.sql`.
- Migrations run automatically on service startup in dev/staging. In production, migrations are a separate manual step before deploying the new service version.
- Never modify an already-applied migration file. Create a new migration to correct it.
- Test both `up` and `down` migrations in CI.

------

## 22. Graceful Shutdown & Health Check

### 22.1 Graceful Shutdown Sequence

```
1. Receive SIGINT or SIGTERM
2. Set health check to return 503 (stop receiving new traffic from LB)
3. Stop accepting new connections (http.Server.Shutdown)
4. Wait for in-flight requests to complete (timeout: 15s)
5. Drain worker pool job channel (timeout: 10s)
6. Close DB connections
7. Close Redis connections
8. Flush logger buffers
9. Exit 0
```

**Timeout and error handling rules:**

- **Total shutdown budget: 25 s.** This must be less than K8s `terminationGracePeriodSeconds` (default 30 s) to allow a buffer for SIGKILL.
- Each step has an individual timeout (shown above). If a step times out, log the error at WARN level and **continue** to the next step — do not block shutdown on a single stuck resource.
- Steps 6–8 (resource cleanup) execute in sequence with a shared 5 s budget. If any fails, log and proceed.
- If the total budget is exhausted, force-exit with `os.Exit(1)` and log all resources that were not cleanly closed.

Any service that does not implement graceful shutdown will drop in-flight requests during rolling deployments.

### 22.2 Health Check Endpoints

```
GET /healthz        → liveness probe
    Returns 200 if process is alive
    Returns 503 if in shutdown mode

GET /readyz         → readiness probe
    Returns 200 if DB ping OK AND Redis ping OK
    Returns 503 if any dependency is unavailable
```

K8s liveness probe uses `/healthz`. Readiness probe uses `/readyz`. The readiness probe prevents traffic from routing to an instance before its dependencies are ready.

------

## 23. Testing Strategy

### 23.1 Three-Layer Testing

| Layer                    | What to Test                            | Dependencies                    |
| ------------------------ | --------------------------------------- | ------------------------------- |
| Unit (Service)           | Business logic, edge cases, error paths | Mock repositories via interface |
| Unit (Handler)           | Request binding, response format, error mapping | Mock services via interface |
| Integration (Repository) | SQL correctness, GORM behavior          | Real DB (Docker Compose in CI)  |
| E2E                      | Core user journeys (3–5 critical paths) | Full stack                      |

Do not chase 100% coverage. Focus unit test coverage on Service layer logic. Repository layer is tested via integration tests.

### 23.2 Handler Testing

Handler tests verify the HTTP contract (status codes, response shape, error mapping) without touching any real service logic:

```go
func TestGetUser_NotFound(t *testing.T) {
    mockSvc := new(mocks.UserService)
    mockSvc.On("GetByID", mock.Anything, uint(999)).Return(nil, errors.ErrNotFound)

    h := handler.NewUserHandler(mockSvc)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Params = gin.Params{{Key: "id", Value: "999"}}

    h.GetUser(c)

    assert.Equal(t, 404, w.Code)
    assert.Contains(t, w.Body.String(), `"code":10001`)
}
```

Key rules:
- Use `httptest.NewRecorder` + `gin.CreateTestContext` — never start a real HTTP server for unit tests.
- Test both success and all documented BizError paths.
- Handler tests do **not** count toward the 70% coverage gate (that targets Service only).

### 23.2 Mock Generation

- Use `mockery` to auto-generate mocks from interfaces.
- Run `mockery --all` as part of code generation step.
- Generated mocks live in `internal/mocks/`.

### 23.3 Test Database

- Use `testcontainers-go` in integration tests to spin up a real PostgreSQL instance per test run.
- Never share test database state between test cases. Each test gets a clean schema via migration + teardown.

### 23.4 Coverage Gate

- Minimum 70% coverage on `internal/service/` packages enforced in CI.
- Coverage below threshold fails the build.

------

## 24. CI/CD & Quality Gates

### 24.1 Pipeline Stages

```
Push to branch:
  1. go vet + golangci-lint         → block on lint errors
  2. go test ./... (unit tests)     → block on test failure
  3. Coverage check                 → block if < 70% on service layer
  4. govulncheck                    → block on known vulnerabilities
  5. swag init (diff check)         → block if docs stale

Merge to main:
  6. Integration tests (testcontainers)
  7. Build Docker image
  8. trivy image scan               → block on HIGH/CRITICAL CVEs
  9. Push image to registry

Deploy to staging:
  10. Run DB migrations
  11. Deploy (rolling update)
  12. Smoke test (hit /readyz + 3 core endpoints)

Deploy to production:
  13. Manual approval gate
  14. Run DB migrations
  15. Deploy (rolling update, 1 instance at a time)
  16. Monitor error rate for 10 minutes before proceeding
```

### 24.2 GitHub Actions Quality Gates

The repository GitHub Actions workflow must fail the build when any of the following checks fail:

- `golangci-lint`
- `go vet ./...`
- `go test ./...`
- Service coverage gate for `./internal/service/...`
- `govulncheck ./...`
- Wire generation drift check against `internal/wire_gen.go`
- Swagger generation drift check against `docs/`

### 24.3 Linter Configuration

Use `.golangci.yml`. Mandatory rules:

```yaml
linters:
  enable:
    - errcheck        # must handle all errors
    - gosimple
    - govet
    - staticcheck
    - unused
    - gofmt
    - goimports
    - revive
    - gosec           # security lints
    - bodyclose       # http response body close
    - contextcheck    # context propagation
```

### 24.4 Commit Convention

Follow Conventional Commits: `feat:`, `fix:`, `refactor:`, `test:`, `chore:`. This enables automatic changelog generation and semantic versioning.

------

## 25. Module Priority & Rollout Order

Implement modules in this sequence. Each phase is a deployable increment.

### Phase 1 — Foundation (Day 1–3)

- [ ] Project structure + Makefile
- [ ] Config layering (Viper, 3-layer model)
- [ ] Logger initialization (Zap, structured JSON)
- [ ] Wire DI setup
- [ ] Graceful shutdown + health check endpoints
- [ ] Unified response structure + BizError system

### Phase 2 — Security & Access (Day 4–7)

- [ ] JWT auth (generate, parse, blacklist)
- [ ] Auth middleware
- [ ] RBAC (Casbin, DB-backed policy)
- [ ] Rate limiting (Redis sliding window)
- [ ] CORS middleware
- [ ] Audit log table + writer

### Phase 3 — Data Layer (Day 8–12)

- [ ] GORM setup with connection pool
- [ ] Repository interfaces + implementations
- [ ] Redis cache with singleflight
- [ ] DB migration tooling (golang-migrate)
- [ ] Read/write split (if required by load)

### Phase 4 — Business Features (Day 13–20)

- [ ] Domain models + service layer
- [ ] Event bus (in-process)
- [ ] Worker pool for async jobs
- [ ] File storage abstraction + S3/OSS implementation
- [ ] i18n (error messages + validation)

### Phase 5 — Async & Scale (Day 21–28)

- [ ] MQ integration (Kafka or RabbitMQ)
- [ ] Outbox pattern for reliable event publishing
- [ ] Asynq scheduled task framework
- [ ] Distributed lock wrapper

### Phase 6 — Observability (Parallel, starts Day 1)

- [ ] Prometheus metrics middleware + core gauges/counters
- [ ] OpenTelemetry tracing + Jaeger backend
- [ ] Alertmanager rules (error rate, P99 latency)
- [ ] Swagger docs + CI staleness check

------

## Appendix A — Technology Stack Reference

| Concern        | Library                                                     | Notes                          |
| -------------- | ----------------------------------------------------------- | ------------------------------ |
| HTTP framework | `gin-gonic/gin`                                             |                                |
| ORM            | `gorm.io/gorm`                                              | with `gorm.io/driver/postgres` |
| Config         | `spf13/viper`                                               |                                |
| DI             | `google/wire`                                               | compile-time                   |
| Logger         | `go.uber.org/zap`                                           |                                |
| JWT            | `golang-jwt/jwt/v5`                                         |                                |
| RBAC           | `casbin/casbin/v2`                                          |                                |
| Cache          | `redis/go-redis/v9`                                         |                                |
| Rate limit     | `golang.org/x/time/rate` (single) + Redis Lua (distributed) |                                |
| Tracing        | `go.opentelemetry.io/otel`                                  |                                |
| Metrics        | `prometheus/client_golang`                                  |                                |
| Async tasks    | `hibiken/asynq`                                             |                                |
| MQ (Kafka)     | `segmentio/kafka-go`                                        |                                |
| MQ (RabbitMQ)  | `rabbitmq/amqp091-go`                                       |                                |
| Migrations     | `golang-migrate/migrate`                                    |                                |
| Mocks          | `vektra/mockery`                                            |                                |
| Testing        | `stretchr/testify` + `testcontainers-go`                    |                                |
| File storage   | `aws/aws-sdk-go-v2/s3`                                      | abstracted behind interface    |
| i18n           | `nicksnyder/go-i18n/v2`                                     |                                |
| Swagger        | `swaggo/swag` + `swaggo/gin-swagger`                        |                                |
| Lint           | `golangci-lint`                                             |                                |
| Security scan  | `golang.org/x/vuln/cmd/govulncheck`                         |                                |

------

## Appendix B — Forbidden Patterns

The following patterns are explicitly banned. Any code generation that produces these patterns must be rejected and rewritten.

```
BANNED: gin.Default()                         → use gin.New() with explicit middleware
BANNED: hardcoded secrets in any file         -> use runtime secret injection (Vault / K8s Secret / equivalent)
BANNED: c.JSON(...) without return            → always return after writing response
BANNED: using *gin.Context in goroutines      → always c.Copy() first
BANNED: db.Updates() without field selection  → use db.Model.Select.Updates
BANNED: fmt.Println / fmt.Printf in handlers  → use zap.L().Info(...)
BANNED: errors.New("...") with user message   → use typed BizError
BANNED: SELECT * in production queries        → always select specific columns
BANNED: panic() outside of main/init          → return errors up the call stack
BANNED: global mutable state                  → inject via DI
BANNED: time.Sleep in production logic        → use context deadline or channel
```

------

*Document version: 1.1 — Keep this document in sync with the codebase. When architecture decisions change, update this document first, then update the code.*