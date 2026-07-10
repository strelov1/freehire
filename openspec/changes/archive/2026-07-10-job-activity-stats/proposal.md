## Why

We have no public view of the catalogue's *flow* over time — how many vacancies
appear versus disappear each day. This is the single most legible "is the
aggregator alive and growing?" signal, both for visitors (trust) and for us
(spotting ingest outages or bulk-import spikes at a glance). The raw data
already exists on `jobs` (`created_at`, `closed_at`); it just isn't aggregated
or surfaced.

## What Changes

- Add a **materialized daily rollup** of job activity: a `job_daily_stats` table
  keyed by calendar day, holding `added` (jobs created that day) and `removed`
  (jobs whose current `closed_at` falls on that day).
- Add a **run-once rollup worker** (`cmd/rollup-stats`) that fully recomputes the
  rollup from `jobs` and upserts it, cron-scheduled once a day — the same
  run-once-and-exit shape as the other workers. Full recompute keeps reopen
  correct (a reopened job simply drops out of its old removed-day next run).
- Add a **public, unauthenticated** read endpoint
  `GET /api/v1/stats/jobs-activity` that serves the rollup aggregated to a
  requested granularity (`day` | `week` | `month`) over a date range, using
  Postgres `date_trunc` so the wire payload stays small.
- Add a **public SPA page `/trends`** rendering a grouped bar chart: green bar =
  added, red bar = removed, per period, with a day/week/month granularity
  toggle.

## Capabilities

### New Capabilities
- `job-activity-stats`: A materialized daily rollup of vacancies added vs.
  removed, a public granularity-aware read API over it, and the public
  dashboard page that visualises it.

### Modified Capabilities
<!-- none: job-analytics covers facet distributions, a distinct concern -->

## Impact

- **Schema**: new `migrations/` file adding `job_daily_stats` (+ supporting
  indexes on `jobs.created_at` / partial on `jobs.closed_at` if not already
  present). No versioned migration runner — apply manually before deploy.
- **DB access**: new `internal/db/queries/*.sql` (rollup upsert + range read),
  regenerated via `make sqlc`.
- **New worker**: `cmd/rollup-stats` (+ `internal/statsrollup` runner), wired
  into `worker.Main`; one new cron entry in ops.
- **API**: new `internal/handler/stats.go` + route registration; public read,
  no auth.
- **Frontend**: new `/trends` route under `web/`, a bar-chart component, and a
  nav/footer link.
- **Dependencies**: a lightweight chart approach in the SPA (reuse existing
  charting if present; otherwise hand-rolled SVG bars — decided in design).
