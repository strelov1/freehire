## Context

The catalogue already records each vacancy's lifecycle timestamps on `jobs`:
`created_at` (when we first ingested it) and `closed_at` (soft-close, NULL while
open — see the job-lifecycle convention). Nothing aggregates or surfaces this as
a time series. We want a public dashboard showing, per period, how many jobs
were added vs. removed, scalable across day/week/month.

Constraints from the codebase:
- **No versioned migration runner** — a new table ships as a `migrations/` file
  applied manually before deploy (per the migrations gotcha).
- **sqlc is the only DB layer** — queries go in `internal/db/queries/*.sql`,
  regenerated with `make sqlc`.
- **Workers are run-once-and-exit**, wired via `worker.Main`/`worker.Bootstrap`,
  cron-scheduled (see `cmd/liveness`, `cmd/enrich`).
- **API list envelope** is `{"data": [...], "meta": {...}}`; handlers signal
  failure by returning an error routed through the central `ErrorHandler`.
- **SPA has no charting library** and won't gain one — existing viz
  (`RateDonut`, `PipelineFunnel`, `FacetBreakdown`) is hand-rolled SVG.

## Goals / Non-Goals

**Goals:**
- A materialized daily rollup that reads fast and is cheap to keep current.
- A public API that serves the rollup at day/week/month granularity.
- A public `/trends` page with a green-added / red-removed grouped bar chart and
  a granularity toggle.

**Non-Goals:**
- Per-source / per-provider / per-region breakdowns (global totals only for v1).
- An event log of every open/close transition (we count by *current* state).
- Real-time/live updates — once-a-day freshness for past days, plus today's live
  slice is enough.
- Reusing or replacing the existing `/analytics` (facet-distribution) page — this
  is a distinct concern; folding the two together is a later option, noted below.

## Decisions

### D1. Rollup by full recompute + upsert, not incremental deltas

The worker recomputes the entire rollup from `jobs` each run and upserts every
day's row:

```sql
-- added per day
INSERT INTO job_daily_stats (day, added, removed) ...
  SELECT d::date, ... FROM (
    SELECT date(created_at) AS day, count(*) AS n FROM jobs GROUP BY 1
  ) a FULL OUTER JOIN (
    SELECT date(closed_at) AS day, count(*) AS n
      FROM jobs WHERE closed_at IS NOT NULL GROUP BY 1
  ) r USING (day)
ON CONFLICT (day) DO UPDATE SET added = EXCLUDED.added,
    removed = EXCLUDED.removed, computed_at = now();
```

**Why over incremental:** the "removed = current `closed_at`" rule (chosen with
the user) means a reopen retroactively changes a past day. A full recompute makes
the rollup a *pure function of current `jobs`*, so reopen, backfills, and manual
edits are all handled for free and re-running is idempotent — no trailing-window
bookkeeping, no drift. The cost is one `GROUP BY date(...)` scan per column per
day; with an index on `created_at` and a partial index on `closed_at` this is
sub-second even at hundreds of thousands of rows, and it runs once a day
off-peak. **Alternative rejected:** incremental "apply today's new/closed" — 30%
less scan but wrong under reopen and fragile across missed runs.

### D2. Table shape: `job_daily_stats(day date PK, added int, removed int, computed_at timestamptz)`

One row per calendar day (UTC). `day` is the natural PK and the upsert conflict
target. Tiny table (≈365 rows/yr), trivially fully-scannable by the read query.
Days with no activity simply have no row; the read fills gaps as zero so the
chart has no holes.

### D3. Granularity aggregation server-side via `date_trunc`

`GET /api/v1/stats/jobs-activity?granularity=day|week|month&from=&to=` reads the
daily rows and, for week/month, `date_trunc('week'|'month', day)` +
`sum(added)`, `sum(removed)`. **Why server-side:** matches freehire's "rich API,
thin SPA" convention and keeps the payload to one element per rendered bar. A
whitelist maps `granularity` → the trunc unit; anything else is a `400` before
touching the DB (fiber `NewError`). Default range is a sensible recent window
(e.g. last 90 days for `day`, wider for coarser units) resolvable server-side;
explicit `from`/`to` override. The `date_trunc` **field** is an ordinary text
argument, so it parameterizes cleanly — a single sqlc query
(`ListJobActivity(unit, from, to)`) covers all three granularities (`day` groups
by `date_trunc('day', …)`, i.e. per-day), rather than three near-identical named
queries. The handler passes the already-whitelisted `granularity` straight
through as the unit (never raw user input reaching `date_trunc`). Edge buckets at
the range boundaries reflect only the in-window days (a partial leading week/month
and the in-progress current one) — intentional, standard time-bucket behavior;
the series is keyed by each bucket's canonical start date.

### D4. "Today" freshness — intra-day cron, not a live-overlay query

The current day's bar must not read as ~0 just because a nightly run hasn't
happened yet. Rather than complicate the read with a live-overlay count (and add
a full-table `jobs(created_at)` index to make that count cheap), we keep the read
trivial — serve the materialized rollup, gap-fill missing days as zero — and get
today's freshness by **scheduling the recompute intra-day** (e.g. every ~3h). The
recompute is idempotent and a cheap batch aggregate, so re-running it a few times
a day is well within budget and needs no extra code or index. **Why over a live
overlay:** the user's ask was explicitly "materialise a once-a-day table"; a
live-overlay + supporting index is gold-plating beyond that and adds
write-amplification on every ingest/close. If a truly live current-bar is ever
wanted, the overlay is a localized read-query change — noted as a seam, not built
now.

### D5. Hand-rolled SVG grouped bar chart

New `web/src/lib/components/ActivityBars.svelte`: a viewBox-scaled SVG with two
`<rect>` per period (green added, red removed), a baseline axis, and simple
hover labels — same technique as `PipelineFunnel`/`RateDonut`. The `/trends`
route (`+page.svelte` + `+page.server.ts` for SSR initial load) fetches via the
existing `web/src/lib/api.ts` client and drives the granularity toggle by
re-fetching. **Why no lib:** adding Chart.js/d3 for two bars violates "no
dependencies that weren't asked for" and the repo already establishes the
hand-rolled-SVG pattern.

### D6. Worker is a thin main; correctness lives in SQL + integration test

The recompute is an **atomic rebuild** — `DELETE FROM job_daily_stats` then
`INSERT … SELECT` (the FULL OUTER JOIN of the two per-day aggregates) in **one
transaction**, so readers never see the table mid-rebuild and reopen-orphaned
rows (a day that had only closures, now reopened) are correctly dropped rather
than left stale. That is the whole worker: `cmd/rollup-stats/main.go` opens a tx
on the pool, runs the two sqlc queries, commits, logs a summary — `worker.Main`
wrapped, run-once-and-exit, like `cmd/liveness` which also keeps its logic in
`main`. **No `internal/statsrollup` package:** its only Go logic would be tx
orchestration, which a fake-store unit test can't meaningfully exercise; the real
semantics (added/removed/reopen/idempotency) are covered by the integration test
against a live Postgres. The genuinely unit-testable Go logic in this feature is
on the **read side** — granularity whitelisting and default-range resolution —
which is extracted as pure helpers and TDD'd (see D3). The dense, gap-free series
is produced in SQL via `generate_series` LEFT JOIN the rollup, so the handler
stays a thin mapper.

## Risks / Trade-offs

- **Full recompute cost grows with catalogue size** → It's a batch `GROUP BY
  date(...)` (seq scan + hash aggregate — an index doesn't help a full-table
  aggregate) run a few times a day; if it ever gets heavy, the recompute can be
  bounded to `day >= now() - interval 'N days'` while older rows stay frozen, or
  a partial `jobs(closed_at) WHERE closed_at IS NOT NULL` index added — both
  one-line changes, deferred until measured. No speculative index ships in v1.
- **Reopen makes past days mutable** → Accepted and specified (user chose
  "current `closed_at`"). The full recompute keeps them correct; a viewer
  refreshing an old range may see a red bar shrink. Acceptable for an aggregate.
- **UTC day boundaries** → Buckets are UTC, which may not match a viewer's local
  midnight. Documented; global aggregate tolerates it. Revisit only if a
  timezone requirement appears.
- **New table before a persistent-DB deploy** → Apply the migration manually
  before deploying the binary that reads it, else the read 500s (the unapplied-
  migration hazard). Migration Plan covers ordering.
- **Two analytics surfaces** (`/analytics` facets + `/trends` activity) → Slight
  IA overlap; mitigated by distinct titles/purpose and a possible later merge.

## Migration Plan

1. Ship + apply `migrations/00NN_job_daily_stats.sql` to prod **before**
   deploying the new server binary (creates table + the two supporting indexes).
2. Run `cmd/rollup-stats` once to populate history, then add its cron entry
   (once daily) in ops.
3. Deploy the server (new read endpoint) and the web build (`/trends` page +
   nav/footer link).
4. **Rollback:** the endpoint and page are additive; disabling the cron and
   hiding the nav link fully backs it out. The table can be dropped independently
   (nothing else references it).

## Open Questions

- Default date window per granularity — pick concrete defaults during implement
  (proposal: 90d/day, 12mo/week, all/month) unless product wants otherwise.
- Whether to expose a total-open trend line later (cumulative), or keep strictly
  to the added/removed flow bars — out of scope for v1.
