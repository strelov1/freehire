## Context

Rate limiting today lives in two places:

- `internal/auth/throttler.go`: `PGThrottler`, a Postgres-backed sliding-window limiter
  (per-second buckets in a `rate_limits` table, aggregated with `SELECT ... FOR UPDATE` to
  serialize concurrent callers on the same key). Wired only into `auth.go`'s six routes.
  Falls back to a plain in-memory Fiber `limiter.New()` when `pool == nil`.
- Five other files (`mail_recall_limit.go`, `photo.go`, `contribution_limit.go`,
  `jd_resolve.go`, `match_analysis_limit.go`) plus an inline `tracerLimiter` in
  `handler.go`, each calling Fiber's `limiter.New(limiter.Config{...})` directly with their
  own `KeyGenerator` closure. Two of these (`mail_recall`, `match_analysis`) prefix their
  keys with a route tag (`"mailrecall:"`, `"match:"`); three (`photo`, `contribution`,
  `jd_resolve`) do not — they emit bare `"user:<id>"`/`"ip:<IP>"`. That's safe only because
  each `limiter.New()` call gets its own isolated in-process counter map.

The module has zero Redis dependency today (verified: absent from `go.mod`, `go.sum`,
`docker-compose.yml`). The app runs as a single instance per deploy color (blue/green on
host2), so nothing here is driven by a horizontal-scaling need — the driver is removing
duplicated, inconsistent code and closing the latent key-collision bug before it becomes a
real cross-route rate-limit leak.

## Goals / Non-Goals

**Goals:**
- One `Throttler` implementation and one Fiber middleware wrapper used by every
  rate-limited route in the API.
- Preserve every route's existing limit/window/observable behavior (429 + `Retry-After`)
  from the caller's perspective — this is a backend swap, not a policy change.
- Preserve fail-open semantics on backend failure, matching `PGThrottler` today.
- Fix the route-prefix key-collision bug as part of the migration, not as a follow-up.

**Non-Goals:**
- Feature/beta flags and response/LLM-answer caching on Redis — both were discussed as
  future uses of the same Redis instance but are explicitly out of scope for this change.
- Horizontal scaling of the app server — not a current requirement; Redis here is valued
  for cross-restart persistence and shared keyspace, not multi-instance coordination.
- Changing any route's numeric limit or window — those stay exactly as configured today.

## Decisions

**Redis client + algorithm: `go-redis/v9` + `go-redis/redis_rate/v10` (GCRA), not a
hand-rolled Lua script.** `redis_rate` is maintained by the `go-redis` authors, implements
the leaky-bucket/GCRA algorithm as a single atomic `Allow` call, and needs no custom script
to write or test. This matches the repo's stated preference for a library's intended API
over a clever shim. A hand-rolled fixed-window (`INCR`+`EXPIRE`) was considered and
rejected: it allows near-2x bursts across a window boundary, which matters for
security-sensitive routes like login. A hand-rolled sorted-set sliding window was also
considered — functionally equivalent to `redis_rate`, but more code to own for no benefit.

**New package `internal/ratelimit`, not `internal/auth`.** The existing `Throttler`
abstraction lives in `internal/auth`, but only one of its eventual six-plus call sites is
actually an auth route. `internal/auth`'s AGENTS.md-documented scope is JWT/API
keys/cookie transport — rate limiting is a distinct cross-cutting concern that outgrew its
original home. Moving it now (while there's license to break the internal interface) avoids
carrying the misplaced ownership forward.

**Redis is a required dependency, not optional.** `PGThrottler` falls back to an in-memory
Fiber limiter when `pool` is nil, and `MeiliURL`/`MeiliKey` empty disables search while the
server still starts. Rate limiting has no equivalent safe degraded mode worth preserving:
an in-memory fallback would silently reintroduce the exact per-instance-memory,
non-persistent behavior this change removes. `docker-compose.yml`'s `app` service depends
on `redis` the same way it depends on `db`.

**Key namespacing: every key-builder helper requires an explicit route prefix.**
`KeyByIP(prefix string)` and `KeyByUserOrIP(prefix string)` both bake the prefix into every
key they generate — there is no variant that omits it. This is a deliberate API constraint,
not just a convention: it makes the `photo`/`contribution`/`jd_resolve` bug (bare
`"user:<id>"` keys colliding once they share a keyspace) impossible to reintroduce by
construction, since the helpers have no zero-prefix call shape.

**`jd_resolve`'s conditional skip stays route-local.** Its current Fiber `Next:` callback
(skip limiting when the request body carries no URL) is specific to that one route's
semantics and doesn't generalize. `ratelimit.Middleware` takes no `Next`-equivalent
parameter; `jd_resolve.go` keeps a thin wrapper that inspects the body and only invokes the
generated `fiber.Handler` when a URL is present.

**Fail-open with a bounded per-call timeout.** `PGThrottler` fails open on any Postgres
error today (see its doc comment), and the repo has an established fail-open convention
elsewhere (LLM spend attribution). `RedisThrottler` keeps this: any error from
`redis_rate.Limiter.Allow` (including a timeout) is logged and treated as allowed.
Postgres transactions already inherit the request's context deadline; a bare Redis call
does not carry an implicit bound, so `ratelimit.Middleware` wraps each `Allow` call in a
~100ms derived context — long enough for a healthy local/same-DC Redis round-trip, short
enough that a partitioned Redis degrades to "every request logs a warning and passes
through" rather than hanging the request.

**Testing on `miniredis`, not testcontainers.** `PGThrottler`'s existing tests need real
Postgres via testcontainers (`-tags=integration`, Docker). `alicebob/miniredis/v2` is an
in-process fake Redis server compatible with `go-redis`, so `RedisThrottler` tests run under
plain `go test ./...` — simpler than what it replaces, not just equivalent.

## Risks / Trade-offs

- **New stateful service to operate** (backup/monitoring/disk not previously needed) →
  Mitigated by giving it no persistent volume and treating its data as pure cache: a lost
  Redis is a fail-open no-op, not a data-loss incident. No backup story is needed.
- **Fail-open means a sustained Redis outage removes rate limiting from every route,
  including login/register** → Same exposure `PGThrottler` already accepts for Postgres
  outages; unchanged risk posture, not a new one introduced by this change.
- **Migrating 12 call sites in one change is a wide diff** → Mitigated by every route
  keeping its existing limit/window; the diff is mechanical (swap constructor + add a
  prefix), not thirteen independent behavior decisions.
- **`docker-compose.yml` `app` depending on a new required service** → Local dev and any
  environment that runs `make up` needs a small resource-footprint bump; Redis's baseline
  memory cost is negligible next to the existing Postgres/Meilisearch/Minio services.

## Migration Plan

1. Ship `internal/ratelimit` package + tests (no call sites touched yet) — purely additive,
   safe to land independently.
2. Add `redis` to `docker-compose.yml`, `REDIS_URL` to `internal/config`, wire the shared
   client + `RedisThrottler` construction into `cmd/server`.
3. Migrate call sites one route at a time (auth's six, then mail-recall, photo,
   contribution, jd-resolve, match-analysis, tracer), each swap independently testable via
   the route's existing 429-after-N-requests test pointed at a `miniredis` instance.
4. Once every call site is migrated, delete `internal/auth/throttler.go` + test.
5. Add the `DROP TABLE rate_limits;` migration.
6. Deploy note: the prod compose/env must provision `redis` and set `REDIS_URL` *before*
   this change's code reaches prod — same ordering constraint the project already documents
   for schema-adding migrations (`migrate` before code that depends on them).

No rollback path beyond standard revert-and-redeploy is needed: nothing here is a one-way
data migration except the `DROP TABLE`, and `rate_limits` holds only transient counters, not
durable state worth preserving.

## Open Questions

None outstanding — scope, backend choice, and key-namespacing rule were settled in
brainstorming before this document was written.
