# Deploy / ops notes — add-trends-insights-api

These steps live in the ops repo (`../freehire-ops`), not this codebase. Record only.

## 1. Apply the migration manually BEFORE deploying the reading code

Migrations run via Postgres initdb only on a fresh volume. On the existing prod
volume, run `migrations/0022_insights_rollups.sql` by hand first (it creates the
four `insights_*` tables and their indexes). Deploying the handler before the
tables exist would 500 every `/api/v1/insights/*` read.

Order: **migrate → deploy → populate → schedule.**

## 2. Populate the rollups once

After deploy, run the rollup worker once so the endpoints return data instead of
empty sets:

```
go run ./cmd/rollup-stats   # now also rebuilds insights_* alongside job_daily_stats
```

## 3. Extend the existing rollup cron

`cmd/rollup-stats` already recomputes `job_daily_stats` on an intra-day timer; it
now recomputes the `insights_*` tables in the same run and transaction. No new
worker or binary — the existing timer covers it. If the recompute noticeably
lengthens the run at prod scale, the levers (bounded window / partial index) are
noted in `design.md`; defer until measured.

## Tuning constants (in `cmd/rollup-stats/main.go`)

- `growthWindowDays = 30` — the role/skill growth comparison window.
- `minSalarySample = 5` — the smallest disclosed-salary count a (currency, period)
  band needs to be published; smaller bands are suppressed.
