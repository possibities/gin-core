# Cache Key Reference

| Key pattern | Purpose | TTL |
| --- | --- | --- |
| `blacklist:jwt:{jti}` | Blacklisted access token | Remaining access token lifetime |
| `{service}:auth:refresh:{user_id}:{jti}` | Refresh token allowlist entry | Remaining refresh token lifetime |
| `{service}:ratelimit:ip:{ip}` | Per-IP sliding-window rate limit | `rate_limit.window_sec` |
| `{service}:lock:scheduler:outbox-dispatch` | Asynq outbox dispatch singleton lock | `scheduler.lock_ttl_sec` |
| `{service}:user:profile:{id}` | Authenticated user profile cache | 5 minutes |
| `{service}:{entity}:{id}` | Entity read-through cache | Set by caller per use case |
| `{service}:{entity}:list:{hash}` | List/query read-through cache | Set by caller per use case |
| `{service}:lock:{resource}` | Distributed lock | 1.5x max critical-section runtime |

`ReadThroughStore` caches null reads for 30s by default and adds TTL jitter of up to 60s to normal cache writes to reduce penetration and avalanche risk.
