## Why

Rate limiting is split across two disconnected implementations today: `PGThrottler` (a
transactional, Postgres-backed sliding-window limiter) guards only the six auth routes
(login/register/verify-request/verify-confirm/forgot/reset), while five other
abuse-sensitive routes (mail recall, photo upload, board contribution, JD-URL resolve,
match-analysis) each roll their own in-process Fiber `limiter.New(...)` with duplicated
key-generation logic. No shared dependency backs any of this, and three of those five
routes key their limiter as bare `"user:<id>"`/`"ip:<IP>"` with no route prefix — harmless
today only because each Fiber limiter instance has isolated in-process memory. Unifying
onto one Redis-backed throttler removes the duplication, gives every route the same
persistence and cross-restart guarantees the auth routes already have, and forces the
route-prefix bug to be fixed rather than silently carried forward.

## What Changes

- Add Redis as a new required dependency: a `redis` service in `docker-compose.yml` and a
  `REDIS_URL` field in `internal/config` (default `redis://localhost:6379/0`), following
  the existing `MeiliURL` pattern but without an optional/fallback path — Redis is required
  like Postgres, not optional like Meili.
- New package `internal/ratelimit` (moved out of `internal/auth`, since rate limiting is
  not an auth primitive): a `Throttler` interface, a `RedisThrottler` implementation built
  on `github.com/go-redis/redis_rate/v10` (GCRA/leaky-bucket) over
  `github.com/redis/go-redis/v9`, a `Middleware(...)` Fiber wrapper, and two key-builder
  helpers (`KeyByIP(prefix)`, `KeyByUserOrIP(prefix)`) that always require a route prefix.
- Migrate all 12 existing limiter call sites (6 in `auth.go`, plus mail-recall, photo,
  contribution, JD-resolve, match-analysis, and the tracer-link redirect) onto
  `ratelimit.Middleware` backed by one shared `RedisThrottler` instance constructed once in
  `cmd/server`.
- **BREAKING** (internal only, no external contract change): fix the latent key-collision
  bug in `photo.go`, `contribution_limit.go`, and `jd_resolve.go` by giving each route its
  own key prefix (`"photo"`, `"contribution"`, `"jdresolve"`) — previously unprefixed
  `"user:<id>"`/`"ip:<IP>"` keys that only avoided colliding because each route's limiter
  had separate in-process memory.
- **BREAKING**: remove `internal/auth/throttler.go` (`Throttler`, `PGThrottler`,
  `ThrottleMiddleware`) and its test, and drop the `rate_limits` Postgres table via a new
  migration (the original `migrations/0079_rate_limits.sql` is left untouched, per this
  repo's migration convention).
- All rate-limit checks fail open on any Redis error (unreachable, timeout), matching
  `PGThrottler`'s current behavior; `ratelimit.Middleware` applies a short (~100ms) timeout
  around each `Allow` call so a hung Redis cannot stall requests before falling back open.

## Capabilities

### New Capabilities
- `api-rate-limiting`: a single Redis-backed rate-limiting facility used by every
  rate-limited HTTP route in the API — the shared `Throttler` contract, its Redis
  implementation, key-namespacing rules, and fail-open error handling.

### Modified Capabilities
(none — existing routes keep their current limits/windows/behavior from the caller's
perspective; only the backing implementation changes)

## Impact

- **New dependency**: Redis (`docker-compose.yml`, `REDIS_URL` config, `redis-cli`/ops
  familiarity on the deploy host).
- **New Go dependencies**: `github.com/redis/go-redis/v9`, `github.com/go-redis/redis_rate/v10`,
  `github.com/alicebob/miniredis/v2` (test-only).
- **Code removed**: `internal/auth/throttler.go` + test, `rate_limits` table.
- **Code touched**: `internal/handler/auth.go`, `mail_recall_limit.go`, `photo.go`,
  `contribution_limit.go`, `jd_resolve.go`, `match_analysis_limit.go`, `handler.go`
  (tracer limiter), `cmd/server` (Redis client construction + wiring), `internal/config`.
- **New migration**: drop `rate_limits` table.
- **Deploy**: `make up` / prod compose must provision the new `redis` service before this
  change ships; `REDIS_URL` must be set in prod env alongside `DATABASE_URL`/`MEILI_URL`.
