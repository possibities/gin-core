# gin-core

生产级 Go 后端脚手架，基于 Gin 框架，遵循严格的分层架构（Handler → Service → Repository），通过 Wire 实现编译期依赖注入。

## 架构概览

```
cmd/server/          程序入口
internal/
  handler/           HTTP 层：绑定请求 → 调用服务 → 返回响应
  service/           业务逻辑、事务编排、领域事件
  repository/        数据访问（GORM）、读写分离
  middleware/        认证、RBAC、限流、链路追踪、指标采集、超时控制
  event/             进程内事件总线 + 事务性 Outbox
  worker/            有界异步工作池
  scheduler/         Asynq 定时任务 + 分布式锁
pkg/
  jwt/               令牌生成/解析、基于 kid 的密钥轮转
  cache/             Redis 读穿缓存、singleflight、分布式锁（含看门狗续期）
  config/            Viper 三层配置（YAML + 环境变量 + 运行时密钥注入）
  errors/            类型化业务错误码，HTTP/业务码映射
  response/          统一 JSON 响应格式
  metrics/           Prometheus 指标注册与观测
  tracing/           OpenTelemetry 初始化、GORM/Redis 链路钩子
  storage/           文件存储抽象（本地、S3）
  i18n/              错误消息国际化（en、zh-CN）
  mq/                消息队列发布（RabbitMQ）
```

## 快速启动

### Docker Compose（推荐）

```bash
docker compose up -d
```

自动启动 PostgreSQL 16、Redis 7 和应用服务，监听 `:8080`，开发环境自动执行数据库迁移。

### 手动运行

```bash
# 前置条件：Go 1.24+、PostgreSQL、Redis

# 设置必需的密钥
export APP_JWT_ACCESS_SECRET=your-access-secret
export APP_JWT_REFRESH_SECRET=your-refresh-secret

# 启动
go run ./cmd/server
```

## 配置管理

三层配置模型：

| 层级 | 来源 | 示例 |
|------|------|------|
| 静态配置 | `config/config.yaml` | 超时时间、连接池大小 |
| 环境配置 | `APP_*` 环境变量 | `APP_DB_HOST`、`APP_REDIS_ADDR` |
| 密钥配置 | 仅运行时注入 | `APP_JWT_ACCESS_SECRET` |

密钥**绝不**写入配置文件。启动时 `config.Validate()` 校验所有必填项，缺失则立即 panic 阻止启动。

## 核心特性

### 认证与授权
- JWT 双令牌（Access + Refresh），通过 `kid` 头实现密钥零停机轮转
- Redis 令牌黑名单，支持强制登出
- Casbin RBAC，策略存储于数据库，运行时热加载

### 缓存策略
- Redis 读穿缓存 + singleflight（防止缓存击穿/惊群效应）
- 空值缓存（防止缓存穿透）
- TTL 随机抖动（防止缓存雪崩）
- 延迟双删（写路径一致性保障）

### 分布式锁
- Redis SET NX + Lua 原子释放
- 看门狗自动续期，防止长任务锁过期

### 限流
- Redis 滑动窗口（Lua 脚本，原子操作）
- 符合 RFC 6585/7231 的响应头（`X-RateLimit-*`、`Retry-After`）
- Redis 不可用时自动降级放行

### 可观测性
- 结构化 JSON 日志（Zap），每条日志携带 trace ID
- Prometheus 指标：HTTP 请求、数据库查询、缓存命中、工作队列深度、goroutine 数量
- OpenTelemetry 链路追踪，集成 GORM 和 Redis 钩子
- Alertmanager 告警规则：错误率、P99 延迟、缓存命中率

### 数据层
- GORM 连接池调优 + DBResolver 读写分离
- 事务性 Outbox 模式，保证事件可靠发布
- golang-migrate 版本化数据库迁移

### 异步处理
- 有界工作池，支持背压控制
- Asynq 定时任务 + 分布式锁防重复执行
- 进程内事件总线 + Outbox 分发器推送至 MQ

## API 接口

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/healthz` | 无 | 存活探针 |
| GET | `/readyz` | 无 | 就绪探针（检查 DB + Redis） |
| GET | `/metrics` | 无 | Prometheus 指标 |
| GET | `/swagger/*` | 无 | Swagger 文档（仅 dev/staging） |
| GET | `/api/v1/session` | Bearer | 当前会话 |
| GET | `/api/v1/users/me` | Bearer | 用户信息 |
| PATCH | `/api/v1/users/me` | Bearer | 更新用户信息 |
| GET | `/api/v1/admin/session` | Bearer + RBAC | 管理员会话（记录审计日志） |

## 中间件链

按以下严格顺序注册：

1. **Recovery** — 捕获 panic
2. **RequestID** — 生成 trace ID
3. **Timeout** — 30 秒请求级 context 超时上限
4. **Tracing** — OpenTelemetry 创建根 span
5. **Metrics** — 请求指标采集
6. **Logger** — 结构化访问日志
7. **Locale** — 从 Accept-Language 解析语言
8. **CORS** — 处理预检请求
9. **RateLimiter** — 按 IP 滑动窗口限流
10. **Auth** — JWT 验证（路由组级别）
11. **RBAC** — Casbin 权限校验（管理员路由组级别）

## 测试

```bash
go test ./...
```

27 个测试文件，覆盖 15 个包：handler、service、middleware、cache、config、JWT、tracing 等。

覆盖率门槛：`internal/service/` 必须 ≥70%（CI 强制执行）。

## CI 流水线

每次推送至 `master` 自动执行：

- golangci-lint（errcheck、gosec、contextcheck 等）
- go vet
- 单元测试
- Service 层覆盖率检查（≥70%）
- govulncheck 漏洞扫描
- Wire 代码生成一致性检查
- Swagger 文档一致性检查

## 项目规范

- `internal/` 利用 Go 的包可见性机制，禁止外部导入
- `pkg/` 仅包含通用工具，不含业务逻辑
- Handler 只做三件事：绑定输入、调用服务、返回响应
- 所有密钥通过运行时环境变量注入，绝不写入配置文件
- 提交信息遵循 Conventional Commits：`feat:`、`fix:`、`refactor:`、`test:`、`chore:`

## 技术栈

| 关注点 | 技术选型 |
|--------|----------|
| HTTP 框架 | Gin |
| ORM | GORM + PostgreSQL |
| 配置 | Viper |
| 依赖注入 | Wire（编译期） |
| 日志 | Zap |
| JWT | golang-jwt/jwt/v5 |
| RBAC | Casbin |
| 缓存 | go-redis/v9 |
| 限流 | Redis Lua 滑动窗口 |
| 链路追踪 | OpenTelemetry |
| 指标 | Prometheus |
| 定时任务 | Asynq |
| 消息队列 | RabbitMQ |
| 数据库迁移 | golang-migrate |
| 文件存储 | 本地 / AWS S3 |
| 国际化 | go-i18n |
| Swagger | swaggo/swag |
| Lint | golangci-lint |

## License

MIT
