## Why

Today a job's `view_count` (shown as "N views" on the detail page) and the "jobs
viewed" figure on `/open` count only **signed-in** users — a view is recorded
solely by the authenticated `POST /api/v1/jobs/:slug/view` beacon. The vast
majority of traffic is anonymous web visitors and external API consumers, whose
views are invisible. We want an honest, all-traffic view number without adding
any load to the read path: collection, update, and aggregation must stay off the
hot path so the job read stays cheap and cacheable.

## What Changes

- Introduce an **offline nginx access-log aggregation worker** (`cmd/rollup-views`)
  that runs on a daily cron, reads the completed (rotated) nginx access log for a
  day, and counts job views from log lines — no per-request work in the app.
- Count two log signals, deduplicated to **unique daily visitors** by
  `hash(client-IP + User-Agent)` per job:
  - `GET /jobs/<slug>` (SSR page opens) — anonymous + signed-in web, with a light
    known-bot User-Agent skip.
  - `GET /api/v1/jobs/<slug>` — external API consumers (the SSR→backend call
    bypasses public nginx via `API_INTERNAL_URL`, so it never appears here; bots
    are not filtered on this path, by decision).
- **BREAKING (semantics):** redefine `jobs.view_count` from "distinct signed-in
  users who opened the page" to "distinct daily visitors across all traffic
  (anonymous + signed-in + API)". `POST /jobs/:slug/view` stops bumping
  `view_count`; it remains only for per-user tracking (`user_jobs.viewed_at`).
- Persist a per-day rollup `job_daily_views(day, job_id, uniques)` written in the
  same pass, giving per-job daily view trends for free; `view_count` is the
  running sum maintained by a batched `UPDATE`.
- Track processed days in a `processed_view_logs(day)` marker table (the cursor),
  so re-runs and backfill are idempotent per day.
- Provide a one-shot **backfill** (flag on the same worker) over historical
  rotated logs to seed `view_count`/`job_daily_views` from existing history.
- Change `/open`'s "jobs viewed" (`GET /api/v1/stats/engagement` → `viewed`) to
  derive from `SUM(jobs.view_count)` so it reflects all traffic.
- Ops (separate `freehire-ops` repo, out of this change's code): add a dedicated
  JSON `log_format` + scoped `access_log`, and a systemd timer for the worker.

## Capabilities

### New Capabilities
- `view-count-aggregation`: Offline aggregation of nginx access logs into
  per-job view counts — log-signal selection, unique-daily-visitor dedup, bot
  handling, the daily rollup table, the processed-day cursor, batched counter
  updates, and one-shot backfill from historical logs.

### Modified Capabilities
- `job-engagement-counts`: `view_count` is redefined to count distinct daily
  visitors across all traffic (anonymous + signed-in + API), sourced from the
  log-aggregation worker; `POST /jobs/:slug/view` no longer increments it.
- `engagement-stats`: the `viewed` figure is derived from `SUM(jobs.view_count)`
  (all traffic) instead of the count of `user_jobs` rows with `viewed_at` set.

## Impact

- **New:** `cmd/rollup-views` worker; migration for `job_daily_views` and
  `processed_view_logs`; a log-parsing package (line → slug / bot / skip);
  DB queries for the batched counter update, the daily upsert, and the cursor.
- **Modified:** `RecordJobView` query (drop the `view_count` bump CTE);
  `GetEngagementStats` query (`viewed` → `SUM(view_count)`).
- **Unchanged:** the public job wire shape still serves `view_count` from the
  `jobs` row; the SPA still renders "N views" — only the number's meaning widens.
- **Ops (not in this repo):** nginx `log_format`/`access_log` addition and a
  systemd timer; local/dev has no log file, so the worker no-ops there.
- **Prerequisite:** confirm on host2 that `access_log` is enabled, its path, and
  the rotation cadence / retained history before finalizing the parser.
