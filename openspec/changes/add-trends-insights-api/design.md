## Context

freehire promotes several enrichment facets to typed `jobs` columns —
`seniority`, `category`, `skills text[]`, `regions text[]`, `countries text[]`,
`work_mode`, `employment_type` — while salary stays inside the `enrichment` JSONB
(`salary_min`, `salary_max`, `currency`, `period`). The catalogue is ~1.75M jobs.

There is already a precedent for aggregate public reads: the `/api/v1/stats/*`
endpoints, the `job_daily_stats` rollup table, and the `cmd/rollup-stats`
run-once worker that recomputes it as an atomic delete-and-reinsert inside one
transaction. That worker treats the rollup as a pure function of current `jobs`
state and swaps it in so readers never see a partial rebuild. This change extends
exactly that pattern to market-insight aggregates.

Constraints:
- Endpoints are public and unauthenticated → aggregate-only, no PII, and no
  unbounded scans (existing `maxRangeDays` discipline in `stats.go`).
- Salary must never mix currencies or pay periods (the project's salary-currency
  discipline); percentiles are computed per (currency, period).
- Migrations apply via initdb on fresh volumes only; prod tables must be created
  manually before deploying code that reads them.

## Goals / Non-Goals

**Goals:**
- Four public reads under `/api/v1/insights` (roles, skills, velocity, salary).
- Cheap, abuse-safe reads backed by precomputed rollups.
- Reuse the `job_daily_stats` / `cmd/rollup-stats` architecture and the
  `stats.go` handler conventions (validated window, dense series, list envelope).

**Non-Goals:**
- SEO landing pages / SSR over these insights (explicit follow-up change).
- Authentication, quotas, or paid tiering.
- Cross-currency salary normalization (FX conversion) — out of scope; report per
  currency.
- Company-level tech-stack aggregation.

## Decisions

### D1: Precomputed rollups over on-the-fly aggregation

Serve reads from `insights_*` rollup tables recomputed by a worker, not `GROUP BY`
over `jobs` per request. Rationale: the endpoints are public and unauthenticated;
a `GROUP BY`/`unnest`/`percentile_cont` over ~1.75M rows per request is a resource
vector, and the project already worries about this on `/stats/*`. Reads become
indexed lookups on small tables. Alternative (on-the-fly + cache) rejected:
adds a cache layer and cold-start latency spikes without removing the abuse
vector on cache-miss.

### D2: "Open-as-of-date" from created_at/closed_at makes growth a pure function

A job is open on date `D` iff `created_at <= D AND (closed_at IS NULL OR
closed_at > D)`. Growth for roles/skills is `open_now` vs `open_as_of(now - W)`
for a fixed window `W` (e.g. 30 days), both derivable from current `jobs` state.
This keeps the rollup a pure function (re-runnable, idempotent) — no historical
snapshot table needed. Trade-off: reopened jobs (closed_at later cleared) are
approximated by current state, same simplification `job_daily_stats` already
accepts.

### D3: Rollup table shapes

- `insights_role_stats(category, seniority, country, open_count, open_count_prev)`
  — `country` includes a synthetic `''`/`ALL` bucket for the geography-agnostic
  rollup so the endpoint can serve both scoped and global reads from one table.
- `insights_skill_stats(skill, category, country, open_count, open_count_prev)`.
- `insights_salary_stats(category, seniority, country, currency, period,
  sample_size, p25, p50, p75)` — percentiles via `percentile_cont` computed in the
  recompute; rows below the min sample size are not emitted.
- Velocity reuses/extends `job_daily_stats` with an optional facet dimension:
  add `insights_velocity_daily(day, facet_kind, facet_value, added, removed)` so a
  single facet (category|seniority|country) can be scoped; the existing global
  `job_daily_stats` remains untouched for `/stats/jobs-activity`.

Exact column sets are finalized during implementation; the recompute SQL and the
read SQL are co-designed so each read is a single indexed `SELECT`.

### D4: One worker, extended

Extend `cmd/rollup-stats` to also rebuild the `insights_*` tables in the same
run/transaction as `job_daily_stats`, rather than adding a new binary. Rationale:
same cadence, same atomic-swap discipline, one cron timer to operate. Alternative
(new `cmd/rollup-insights`) rejected as premature separation. If recompute cost
grows, splitting is a later, measured decision.

### D5: Handler + routing conventions

New `internal/handler/insights.go` mirrors `stats.go`: whitelist-validated params
(`granularity`, `sort`, geography enums, bounded `limit`), injected `now` for
testable date defaulting, dense series for velocity, and the `{"data","meta"}`
envelope. Routes are registered public (no middleware) under
`/api/v1/insights` in `handler.go` next to the existing `/stats` group.

## Risks / Trade-offs

- **Recompute cost at 1.75M rows** → Mitigation: full-table hash aggregates are
  the same class of work `job_daily_stats` already does; if it gets heavy, bound
  the window or add a partial `jobs(closed_at)` index (deferred until measured).
- **Salary sparsity / identifiability** → Mitigation: per-(currency,period)
  bands with a minimum sample-size floor; bands below it are suppressed, which
  also prevents a single-job "band" from leaking an individual figure.
- **Stale current-day insights** between worker runs → Mitigation: schedule the
  worker intra-day (same as `rollup-stats`); insights are trend data, not
  real-time, so minor lag is acceptable.
- **Prod migration ordering** → Mitigation: create `insights_*` tables manually
  before deploying the reading code (documented in tasks + ops per the migrations
  gotcha).

## Migration Plan

1. Add migration creating the `insights_*` tables (initdb on fresh volumes).
2. On prod: run the `CREATE TABLE` statements manually before deploying.
3. Deploy worker change; run `cmd/rollup-stats` once to populate the rollups.
4. Deploy handler/routes; endpoints now read populated tables.
5. Add the insights recompute to the existing rollup cron timer.

Rollback: revert handler/route registration (endpoints disappear); the `insights_*`
tables and worker additions are inert if unread.

## Open Questions

- Growth window `W` default (30 days assumed) — confirm during implementation.
- Minimum salary sample-size floor value — pick a conservative default (e.g. 5)
  and make it a constant, revisit if bands look noisy.
