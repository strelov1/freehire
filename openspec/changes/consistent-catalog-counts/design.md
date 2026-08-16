## Context

`GET /api/v1/jobs` fills `meta.total` from `estimate_open_jobs()`, a plpgsql
function added in `migrations/0001_init.sql` that runs `EXPLAIN (FORMAT json)
SELECT 1 FROM jobs WHERE closed_at IS NULL` and returns the planner's `Plan
Rows`. It was the right call at the time: an exact `count(*)` over millions of
open rows is a per-request sequential scan, and the estimate is O(1).

Two things have since gone wrong with it, measured on production on 2026-08-16:

| Figure | Value |
| --- | --- |
| `estimate_open_jobs()` (what the site shows) | 5,226,661 |
| Exact `COUNT(*)` with the listing predicate | 3,300,658 |
| Rows in `jobs` | 7,356,316 |
| `pg_class.reltuples` for `jobs` | 9,574,771 |

The estimate omits `duplicate_of IS NULL AND NOT is_private`, which the list
itself applies, so it counts suppressed reposts. And it inherits `reltuples`,
which bloat has pushed 30% above the real row count and which only moves when
`ANALYZE` runs — so the published figure sits still and then jumps, rather than
tracking the catalogue.

Consumers today: `GET /api/v1/jobs` (`meta.total`), `/about`
(`+page.server.ts`, one `listJobs(1,0)` plus one `listCompanies('',1,0)`), and
`/open` (the same two reads, inside a 60s module-level memo). `/open`
additionally hardcodes `ATS_PLATFORMS = 166` and `TELEGRAM_CHANNELS = 88` in
`+page.svelte`; the real figures are 227 registered adapters and 95 configured
channels. `HomeView.svelte` carries `3.4M+` / `200K+` API-down fallbacks and a
hardcoded `'166'`.

Redis is already a runtime dependency: `cmd/server` builds a `*redis.Client`
from `cfg.RedisURL` and hands it to `ratelimit.NewRedisThrottler`. `miniredis`
is already a test dependency. `cmd/rollup-stats` already runs intra-day and
already recomputes rollups from `jobs` inside one transaction.

## Goals / Non-Goals

**Goals:**

- One exact open-job figure, identical on every surface that quotes it.
- Stable between recomputations — no movement that isn't a real catalogue change.
- No per-request cost that grows with catalogue size.
- A cache abstraction good enough that the next consumer does not reach for
  `*redis.Client` directly.
- Platform and channel counts derived from the backend, never a frontend
  literal.

**Non-Goals:**

- Real-time accuracy. A snapshot hours old is fine; the catalogue moves well
  under 1% per hour, and consistency is the actual ask.
- Making `/jobs/search`'s Meilisearch-estimated total exact. That is a different
  number with a different meaning (matched hits, not catalogue size).
- Caching anything beyond catalogue scale in this change. `internal/cache` is
  built to be reusable, but `/open`'s other five legs keep their existing
  module-level memo.
- Fixing the table bloat that inflated `reltuples`. Worth doing, separately.

## Decisions

### Redis, not a Postgres table

The snapshot is read on every `/jobs` request, which is the hottest public path
in the system. A `catalog_stats` table would survive a Redis flush and need no
TTL reasoning, but it puts a Postgres round-trip on that path to avoid a
Postgres round-trip, and Postgres is the resource already under I/O pressure on
this host. Redis is in the request path already for rate limiting.

*Alternative considered:* in-process memoization in the Go server, no Redis at
all. Rejected because the count would then differ per process and reset on every
deploy — which is the inconsistency this change exists to remove. The frontend's
existing module-level memo has exactly that flaw.

*Accepted cost:* a Redis flush or a restart without persistence drops the
snapshot until the next `rollup-stats` run, and the figure degrades to the
estimate meanwhile. The corrected estimate makes that degradation tolerable
rather than embarrassing, which is why fixing it is in scope.

### `internal/cache` as a generic layer, not a single typed helper

Building generic infrastructure ahead of need is the failure mode this codebase
avoids. It is not the case here: there are three present consumers — the open-job
count, the company count, and `/open`'s six-leg payload that today memoizes in a
single frontend process. A one-off `GetOpenJobCount` helper would be re-generalized
within the month.

The layer stays deliberately thin — `Get`/`Set` over `[]byte` with a TTL, plus
free `GetJSON[T]`/`SetJSON` functions, because Go does not permit type
parameters on methods. It models the `ratelimit.Throttler` shape already in the
tree: the interface reports errors, and a single caller decides to fail open, so
every implementation gets that behaviour uniformly. Two implementations ship:
`RedisCache` and `Memory` (map plus mutex) for tests and for running without
Redis.

### `internal/catalogstats` owns the figures; `internal/cache` only stores bytes

The cache knows nothing about jobs. `catalogstats` owns the `Snapshot` type,
the exact queries, the registry and channel-config counts, the cache key, and
the fallback rule. That keeps the read path expressible as one call whose
signature says what it does, and keeps the invariant — *never recompute on a
request* — in one place rather than at each call site.

`Load` returns the snapshot together with whether it is exact or degraded, so a
consumer can render `3,300,658` differently from `~3.3M`. The frontend need not
use that distinction on day one, but the API should not lie about which it is.

### Write from `cmd/rollup-stats`, not a new worker

`rollup-stats` already scans `jobs` on an intra-day cron and already exists as a
deployed systemd unit. Adding a snapshot write there costs one more query on a
run that is already doing heavier work, and costs nothing in ops surface — no
new unit file, no new timer, no new entry in the release script's worker list.

*Alternative considered:* a dedicated `cmd/snapshot-stats` on a tighter (say
10-minute) schedule for fresher numbers. Rejected as unjustified: nothing about
this figure needs ten-minute freshness, and a new cron worker is real ops cost.
The seam is there if that changes — `catalogstats.Compute` is callable from
anywhere.

### TTL longer than the cron interval

The TTL is 24h against an intra-day schedule. Setting it near the cron interval
would mean a single skipped or slow run drops every surface back to the
estimate. Making it long means a failed cron degrades to *stale but exact*,
which is strictly better than *fresh but wrong*. `ComputedAt` travels in the
snapshot so staleness is observable rather than hidden.

### Fix the estimate even though it becomes the fallback

The estimate stays approximate — that is its job — but a fallback that is
systematically 58% high is a worse failure mode than one that is a few percent
off. Adding `duplicate_of IS NULL AND NOT is_private` to the `EXPLAIN` costs
nothing at runtime, since the planner still answers from statistics. This ships
as a new migration; the applied `0001_init.sql` is not edited.

### One endpoint, not two list reads

`GET /api/v1/stats/catalog` returns all four figures. `/about` and `/open`
switch to it. Beyond halving their request count, this is what makes the two
pages structurally incapable of disagreeing: they render one snapshot, not two
independently-taken estimates.

## Risks / Trade-offs

- **Redis becomes load-bearing for a public figure** → It is a soft dependency
  by construction: a miss or an error is a miss, and the request completes with
  the estimate. A test asserts the unreachable-backend path returns a normal
  response, not a 5xx.
- **A stale snapshot silently misrepresents the catalogue** → `ComputedAt` ships
  in the API response, so staleness is inspectable rather than invisible. The
  24h TTL bounds it; past that the figure degrades to the estimate rather than
  going unboundedly stale.
- **The exact count is a sequential scan on an I/O-pressured host** → It runs in
  a cron worker that is already scanning `jobs`, not on a request path, and once
  per run rather than per request. Net request-path cost is negative: a Redis
  `GET` replaces an `EXPLAIN`.
- **Two surfaces still read `/api/v1/companies` for other purposes** → Out of
  scope. Only the catalogue-scale strip moves to the snapshot; the companies
  listing keeps its own count semantics.
- **The published headline drops from 5.2M to 3.3M** → It is a correction, not a
  regression, and the README and `docs/sources.md` have already been brought to
  the true figures. Worth being deliberate about the timing given the Product
  Hunt launch on 26 August 2026: better corrected before it than during it.

## Migration Plan

1. Ship the migration correcting `estimate_open_jobs()`. Safe alone: it only
   improves the number every current consumer already reads.
2. Ship `internal/cache` and `internal/catalogstats` with the `rollup-stats`
   write and the `/stats/catalog` endpoint. Until the first worker run the
   endpoint reports a degraded snapshot, which is correct behaviour, not a
   failure.
3. Run `rollup-stats` once by hand to populate the snapshot before the frontend
   depends on it.
4. Ship the frontend switch to `/stats/catalog` and delete the constants.

Rollback: reverting the frontend restores the list-read path; reverting the
backend leaves an unread Redis key that expires on its own. No schema rollback
is needed — the corrected `estimate_open_jobs()` is an improvement independent
of everything else.

## Open Questions

None blocking. One deliberately deferred: whether `/about` and `/open` should
visually distinguish an exact figure from a degraded one. The API carries the
distinction from day one; the frontend can adopt it whenever it is worth the
design.
