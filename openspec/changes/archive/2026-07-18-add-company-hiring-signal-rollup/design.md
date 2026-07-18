## Context

The catalogue retains closed jobs (`jobs.closed_at` is set, rows are never deleted), so a per-company hiring time series is latent in `jobs` but not precomputed anywhere. The existing rollups established the pattern to follow:

- `migrations/0022_insights_rollups.sql` — `insights_*` tables, each a pure function of current `jobs`, rebuilt as an atomic `DELETE`+`INSERT` in one transaction. Openness for a date is `created_at <= D AND (closed_at IS NULL OR closed_at > D)`.
- `cmd/rollup-stats/main.go` — the run-once worker: `worker.Bootstrap`, one transaction, `SET LOCAL work_mem = '256MB'` for the big aggregates, `DeleteAll…` then `Rebuild…`, commit, exit non-zero on failure.
- `internal/db/queries/insights.sql` / `stats.sql` — the `count(*) FILTER (…)` open-count idiom and the `generate_series` gap-free calendar idiom.

This change adds one more rollup of the same kind, keyed by company.

## Goals / Non-Goals

**Goals:**
- Precompute a per-`(company_slug, day)` velocity series (`added`, `removed`, running `open`) from current `jobs`.
- Make it fully retroactive and a pure function of `jobs` — no snapshot dependency.
- Match the existing rollup conventions exactly (atomic rebuild worker, same idioms).

**Non-Goals:**
- No HTTP API/read endpoint (a later change).
- No daily company-attribute snapshot (`employee_count`/`maturity` history stays lossy for now).
- No stored "previous-window" column — growth is derived from the `open` series.

## Decisions

### One per-day table, activity days only, with running `open`

`insights_company_stats(company_slug text, day date, added int, removed int, open int, PRIMARY KEY (company_slug, day))`.

- A row exists only for a company's **activity days** (a day where `added > 0` or `removed > 0`), keeping the table bounded rather than companies × all-days.
- `open` on a row is the company's open count as of the end of that day, i.e. the running total `cumulative(added) − cumulative(removed)` up to and including that day. Because a job is always created no later than it closes, `open_at(D) = (created ≤ D) − (closed ≤ D)`, so the running difference equals the point-in-time open count — no per-day scan needed.
- Growth over any window is a read-time carry-forward lookup on `open` (see spec), so **no `open_count_prev` column** is stored.

**Alternatives considered:**
- *Scalar-per-company table* (`open_count`, `open_count_prev`) like `insights_role_stats`: smaller, but throws away the time series that is the actual signal. Rejected — the series is the product.
- *Two tables* (per-day velocity + scalar growth): more surface for no gain, since `open` per day already yields growth. Rejected.
- *Store `open` for every company on every day*: gap-free per-company calendar explodes row count. Rejected in favour of activity-days + carry-forward.

### Canonical rows only

The rebuild counts rows with non-empty `company_slug` and `duplicate_of IS NULL`, matching `RefreshCompanyFacets`/`companies.job_count` semantics so per-company numbers reconcile with the rest of the app. Repost copies and company-less rows contribute nothing.

### Rebuild as a single `INSERT … SELECT`

Per company, unnest the two event streams — `created_at::date` as `+1 added`, `closed_at::date` as `+1 removed` — aggregate to `(company_slug, day)`, then a window `SUM` ordered by day yields the running `open`. Delivered as `RebuildInsightsCompanyStats` alongside `DeleteAllInsightsCompanyStats` in `internal/db/queries/insights.sql`; sqlc regenerates `internal/db/`.

### Separate worker `cmd/rollup-company`

A new run-once worker mirroring `cmd/rollup-stats` (own transaction, `SET LOCAL work_mem`, delete+rebuild, atomic commit). Kept separate rather than folded into `rollup-stats` so its (heavier, company-grained) cadence can be scheduled independently and a failure doesn't block the public `/insights` rollups.

## Risks / Trade-offs

- **Row volume**: companies × activity-days is larger than the facet-bounded `insights_velocity_daily`. → Bounded to activity days only; it is an internal rollup with no live-read latency budget yet. Revisit if it grows unwieldy.
- **Lossy reopen history**: only the current `closed_at` survives, so a reopen/reclose cycle is not fully reconstructed. → Accepted and already true for every existing rollup; documented, not fixed here.
- **Enrichment reflects current, not post-time values**: role/skill/salary per company would use today's enrichment. → Out of scope; this change stores only counts (`added`/`removed`/`open`), no enrichment dimensions yet.

## Migration Plan

- New migration `migrations/00NN_insights_company_stats.sql` creating the table (+ an index for as-of/`day` scans). Applied by initdb on a fresh volume; on prod it must be run manually **before** deploying code that reads it (existing migrations gotcha) — though nothing reads it yet in this change.
- Deploy the worker; schedule it on cron like `rollup-stats`. Re-running is safe (idempotent atomic rebuild).
- Rollback: drop the table and remove the worker; nothing else depends on it.

## Open Questions

- Cron cadence for `cmd/rollup-company` (likely daily, given company grain) — an ops decision, not a code decision for this change.
