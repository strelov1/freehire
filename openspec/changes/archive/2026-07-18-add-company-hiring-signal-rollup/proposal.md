## Why

The catalogue already retains closed jobs (`jobs.closed_at` is set, never deleted), so a per-company hiring time series is *latent* in the data but nowhere precomputed. Every existing rollup (`job_daily_stats`, `insights_role_stats`, `insights_skill_stats`, `insights_salary_stats`, `insights_velocity_daily`) is global or keyed by category/seniority/country/skill — **none is keyed by company**. A company-grain rollup is the missing foundation for a "company hiring-signal" product (who is ramping vs. freezing, and what they hire), and it can be built entirely from data we already hold.

## What Changes

- Add a new precomputed rollup table `insights_company_stats` keyed per `(company_slug, day)`, holding `added` / `removed` / `open` job counts derived from `jobs.created_at` / `jobs.closed_at`, plus current-vs-prev-30d open counts for a growth read.
- Add a rebuild SQL query in `internal/db/queries/insights.sql` that recomputes the table in one atomic `DELETE` + re-`INSERT`, matching the existing `insights_*` rollup pattern.
- Add a run-once-and-exit worker `cmd/rollup-company` that runs the rebuild in a single transaction (same shape as `cmd/rollup-stats`).
- The rollup is **retroactive**: on first run it reconstructs the full history back to the earliest `created_at`, using retained closed rows — no wait to accumulate data.

Out of scope for this change (deliberately, one change at a time):
- No HTTP API endpoint to read the rollup.
- No daily company-attribute snapshot table (accepting that `employee_count`/`maturity` history stays lossy for now).

## Capabilities

### New Capabilities
- `company-hiring-signal`: a precomputed per-company, per-day rollup of hiring velocity (jobs added/removed/open) and 30-day open-count growth, derived from the retained `jobs` lifecycle, produced by a standalone rollup worker.

### Modified Capabilities
<!-- None: existing insights rollups and the market-insights API are unchanged; this adds a new, independent company-grain rollup. -->

## Impact

- **Schema:** new migration adding `insights_company_stats` (+ supporting index). Follows the initdb-only migration convention.
- **DB layer:** new query in `internal/db/queries/insights.sql`; regenerated sqlc code in `internal/db/`.
- **New worker:** `cmd/rollup-company/main.go` (needs `DATABASE_URL` only); intended to run on a cron cadence like `cmd/rollup-stats`.
- **No API, frontend, or search impact.** Purely additive; no existing behavior changes.
