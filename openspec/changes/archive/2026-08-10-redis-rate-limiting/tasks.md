## 1. `internal/ratelimit` package (additive, no call sites touched yet)

- [x] 1.1 Define the `Throttler` interface (`Allow(ctx, key, limit, window) (allowed bool, retryAfter time.Duration, err error)`) and `Middleware(throttler, keyFunc, limit, window) fiber.Handler` — wraps each `Allow` call in a ~100ms derived context and fails open (log + allow) on any error, including timeout.
- [x] 1.2 Add `github.com/redis/go-redis/v9` and `github.com/go-redis/redis_rate/v10` to `go.mod`; implement `RedisThrottler` (`NewRedisThrottler(client *redis.Client) *RedisThrottler`) satisfying `Throttler` via `redis_rate.Limiter.Allow`.
- [x] 1.3 Implement `KeyByIP(prefix string) func(*fiber.Ctx) string` and `KeyByUserOrIP(prefix string) func(*fiber.Ctx) string` — both always emit a prefix-qualified key; no zero-prefix variant exists.
- [x] 1.4 Add `github.com/alicebob/miniredis/v2` as a test dependency; write `RedisThrottler` unit tests: within-limit allowed, over-limit rejected with correct `retryAfter`, and a fail-open test against an unreachable address.

## 2. Redis infra and config wiring

- [x] 2.1 Add a `redis` service to `docker-compose.yml` (`redis:7-alpine`, no persistent volume, healthcheck via `redis-cli ping`); make `app` depend on it.
- [x] 2.2 Add `RedisURL` to `internal/config.Config` (`env("REDIS_URL", "redis://localhost:6379/0")`).
- [x] 2.3 Construct the shared `*redis.Client` and one `RedisThrottler` in `cmd/server`; pass it into `handler.NewHandlers` in place of the current pool-conditional `PGThrottler` construction.

## 3. Migrate auth routes

- [x] 3.1 Change `Handlers.throttler` field type from `auth.Throttler` to `ratelimit.Throttler`; update `auth.go`'s six limiter constructions (login/register/verify-request/verify-confirm/forgot/reset) to `ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("<route>"), limit, window)`, preserving each route's existing limit/window numbers.
- [x] 3.2 Remove the now-dead `pool == nil` in-memory-fallback branch in `auth.go` (Redis is a required dependency, no fallback).

## 4. Migrate remaining routes (bug fix: add missing route prefixes)

- [x] 4.1 `mail_recall_limit.go`: replace `limiter.New(...)` with `ratelimit.Middleware(h.throttler, ratelimit.KeyByUserOrIP("mailrecall"), mailRecallsPerHour, time.Hour)`.
- [x] 4.2 `photo.go`: same swap with prefix `"photo"` (previously unprefixed — this is the collision-bug fix for this route).
- [x] 4.3 `contribution_limit.go`: same swap with prefix `"contribution"` (previously unprefixed).
- [x] 4.4 `jd_resolve.go`: same swap with prefix `"jdresolve"` (previously unprefixed); keep the existing body-inspection skip as a thin wrapper around the generated `ratelimit.Middleware` handler, since `Middleware` has no `Next`-equivalent.
- [x] 4.5 `match_analysis_limit.go`: same swap, keep existing prefix `"match"`.
- [x] 4.6 `handler.go`: replace the inline `tracerLimiter := limiter.New(...)` with `ratelimit.Middleware(h.throttler, ratelimit.KeyByIP("tracer"), 60, time.Minute)`.

## 5. Remove the old implementation

- [x] 5.1 Delete `internal/auth/throttler.go` and its test once no call site references `auth.Throttler`/`auth.PGThrottler`/`auth.ThrottleMiddleware`.
- [x] 5.2 Add a new migration `DROP TABLE rate_limits;` (do not edit `migrations/0079_rate_limits.sql`); run `make sqlc` if any generated code references the table.
- [x] 5.3 `go mod tidy`.

## 6. Verification

- [x] 6.1 `go build ./...` and `go vet ./...`.
- [x] 6.2 `go test ./...` (covers new `ratelimit` package tests against miniredis).
- [x] 6.3 `go vet -tags=integration ./...` (per this repo's pre-push convention — catches any remaining `internal/handler` integration tests still referencing the removed constructors).
- [x] 6.4 `make up` locally; confirm the `redis` service starts healthy and a route's existing 429-after-N-requests behavior still holds end-to-end (e.g. hit `/api/v1/auth/login` past its limit).
