## Why

Every public surface that quotes catalogue scale is quoting a number that is
wrong by 58% and drifts on its own. `GET /api/v1/jobs` reports
`meta.total = 5,226,661`; the catalogue actually holds **3,300,658** open,
deduplicated, non-private postings. The `/about` landing strip, the `/open`
transparency page, and the API all read that figure, so the headline claim on
the marketing pages is inflated — and because the estimate rides on planner
statistics, it steps to a new wrong value every time autovacuum runs `ANALYZE`.

Two independent defects produce it, both in `estimate_open_jobs()`:

1. It estimates `WHERE closed_at IS NULL` only. The list it labels also applies
   `duplicate_of IS NULL AND NOT is_private`, so every suppressed repost is
   counted in the total but absent from the results.
2. The planner's row estimate derives from `reltuples`, which currently reads
   9,574,771 against 7,356,316 real rows. Table bloat inflates it, and the
   figure only moves when statistics are refreshed — which is what makes it
   jump rather than drift.

A second, smaller instance of the same problem: `/about` and `/open` each issue
their own `limit=1` list read, so two adjacent pages can show two different
open-job counts from two estimates taken at different moments.

## What Changes

- Add `internal/cache`: a small best-effort key/value layer with TTL — a `Cache`
  interface, a Redis-backed implementation, and an in-memory one for tests and
  for a deployment without Redis. Failure handling follows the existing
  `ratelimit.Throttler` precedent: the implementation reports the error, one
  caller decides to fail open.
- Add `internal/catalogstats`: owns the catalogue-scale figures as a single
  `Snapshot` (open jobs, companies, ATS platforms, Telegram channels, computed
  at). `Compute` runs the exact counts; `Load` reads the cached snapshot and, on
  a miss, degrades to the existing estimate. The exact count never runs on a
  request path.
- Write the snapshot from `cmd/rollup-stats`, which already walks `jobs` on an
  intra-day cron. No new worker, no new systemd unit. The cached snapshot
  outlives a skipped run (24h TTL), so a missed cron degrades to a slightly
  stale figure rather than back to a wrong one.
- Add `GET /api/v1/stats/catalog` returning the whole snapshot. `/about`,
  `/open`, and `GET /api/v1/jobs`'s `meta.total` all read the same snapshot, so
  every surface shows one mutually consistent set of numbers.
- Remove the hardcoded `ATS_PLATFORMS = 166` and `TELEGRAM_CHANNELS = 88` from
  `web/src/routes/open/+page.svelte` — the backend already knows both from
  `sources.Taxonomy()` and `sources/telegram.yml`. Refresh the API-unavailable
  fallbacks in `HomeView.svelte` (`3.4M+` / `200K+`) to match reality.
- Correct `estimate_open_jobs()` to apply `duplicate_of IS NULL AND NOT
  is_private`, via a new migration. It stays approximate, but as the degraded
  path it should not be systematically wrong.

## Capabilities

### New Capabilities

- `catalog-scale-snapshot`: the exact, periodically recomputed catalogue-scale
  figures — how they are computed, cached, served over
  `GET /api/v1/stats/catalog`, and what happens when the cache is cold.

### Modified Capabilities

- `job-search`: the requirement "DB-backed jobs list is index-served with an
  approximate total" changes. `meta.total` becomes the exact cached snapshot
  value when one is available, and falls back to the estimate only when it is
  not. The prohibition on per-request work that scales with catalogue size is
  unchanged and is what forces the cache.
- `open-transparency-page`: the catalogue-scale stat strip sources every figure
  from the snapshot endpoint. The ATS-platform and Telegram-channel counts stop
  being frontend constants.

## Impact

- **Schema:** one new migration amending `estimate_open_jobs()`. No table
  changes.
- **New packages:** `internal/cache`, `internal/catalogstats`.
- **Modified:** `internal/handler` (new `/stats/catalog` route; the jobs list
  total), `cmd/rollup-stats` (writes the snapshot), `cmd/server` (constructs the
  cache, already builds the Redis client for rate limiting).
- **Frontend:** `web/src/routes/open/+page.svelte`,
  `web/src/routes/open/+page.server.ts`, `web/src/routes/about/+page.server.ts`,
  `web/src/lib/components/HomeView.svelte`.
- **Runtime dependency:** the API gains a soft dependency on Redis for this
  figure. Redis being unreachable degrades the count to the estimate; it never
  fails a request.
- **Docs:** `README.md` and `docs/sources.md` headline figures already corrected
  to the true counts as part of the investigation that surfaced this.
