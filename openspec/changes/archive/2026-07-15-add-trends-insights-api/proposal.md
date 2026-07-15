## Why

freehire already computes rich structured facets per job (seniority, category,
skills, geography, salary) but exposes them only as per-job fields and search
filters. There is no way to ask the catalogue aggregate questions — "which roles
are hiring most", "what skills are in demand", "how fast is a segment growing",
"what does this role pay". Competitors sell exactly this as a paid market-data
product. Surfacing it as a public read API is a low-cost first step (the raw data
already exists) that later powers programmatic-SEO landing pages as a follow-up
change.

## What Changes

- Add a new public, unauthenticated, aggregate-only **Trends & Insights API**
  under `/api/v1/insights` with four reads:
  - `GET /insights/roles` — ranked roles (category × seniority) by open-count and
    growth, optionally scoped by geography.
  - `GET /insights/skills` — skill-demand ranking by open-count and growth,
    optionally scoped by geography/category.
  - `GET /insights/velocity` — hiring velocity time series (added vs removed) per
    facet dimension.
  - `GET /insights/salary` — salary bands (percentiles) by role and geography,
    reported per currency and normalized pay period.
- Add precomputed **insights rollup tables**, recomputed by a cron worker
  (extending the existing rollup worker pattern), so the public endpoints read a
  cheap, abuse-safe snapshot rather than aggregating the full `jobs` table per
  request.
- Rollups are a pure function of current `jobs` state (open-as-of-date derived
  from `created_at`/`closed_at`), swapped in atomically like `job_daily_stats`.
- Salary aggregation is gated per single currency and normalized per pay period
  to avoid mixing incomparable figures (existing salary-currency discipline).

Out of scope (explicit follow-up change): SEO landing pages / SSR views over
these insights, and any authenticated/paid tiering.

## Capabilities

### New Capabilities
- `market-insights`: public aggregate market-intelligence reads over the job
  catalogue — role demand, skill demand, hiring velocity, and salary bands —
  served from precomputed rollups.

### Modified Capabilities
<!-- None: existing stats capabilities (job-activity-stats, engagement-stats,
     user-growth-stats) are untouched; this adds a new, separate capability. -->

## Impact

- **New DB tables**: `insights_*` rollup tables (migration; applied to prod
  manually before deploy per the migrations gotcha).
- **New SQL queries**: recompute (delete + rebuild) and read queries in
  `internal/db/queries/`, regenerated via sqlc.
- **Rollup worker**: extend `cmd/rollup-stats` (or a sibling worker) to recompute
  the insights rollups; new cron timer in prod ops.
- **HTTP handlers + routes**: new `internal/handler/insights.go` + route wiring in
  `internal/handler/handler.go` under `/api/v1/insights` (public).
- **API docs**: extend `web/static/openapi.yaml` and `docs/API.md`.
- No changes to auth, ingest, enrichment, or search paths.
