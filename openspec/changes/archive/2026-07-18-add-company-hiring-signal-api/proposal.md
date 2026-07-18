## Why

The per-company hiring-signal rollup (`insights_company_stats`, shipped in the prior change) is populated on prod but has no read surface — the "who is ramping vs. freezing" leaderboard that is the headline product value cannot be queried. This adds the first public read over that signal, matching the existing `market-insights` `/insights/*` family.

## What Changes

- Add a public, unauthenticated `GET /api/v1/insights/companies` endpoint returning companies ranked by hiring growth or open-count, with `sort` (`growth` = ramping, `-growth` = freezing, `open` = size), a `min_open` sanity filter (guards against ingest-artifact spikes — a board fully appearing/disappearing), and a capped `limit`. Standard `{"data": [...], "meta": {...}}` envelope.
- Add a small precomputed per-company scalar table `insights_company_growth(company_slug, open_count, open_count_prev)` — the ranked-read backing, mirroring `insights_role_stats`. The endpoint reads this with a trivial indexed `ORDER BY … LIMIT` rather than aggregating ~155k companies per request.
- Extend the existing `cmd/rollup-company` worker to rebuild `insights_company_growth` in the same atomic transaction as `insights_company_stats`, using a 30-day `prev_ts` (as `cmd/rollup-stats` does for `insights_role_stats`).

Out of scope (later slice): the single-company detail/time-series endpoint (`GET /api/v1/insights/companies/:slug`), and any auth/API-key gating.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `market-insights`: add a new public `GET /api/v1/insights/companies` leaderboard endpoint (existing `/insights/*` endpoints unchanged).
- `company-hiring-signal`: add a precomputed per-company open/growth scalar (`insights_company_growth`) produced by the rollup worker, alongside the existing per-day `insights_company_stats`.

## Impact

- **Schema:** new migration adding `insights_company_growth` (+ index for the ranked read). initdb-only + manual-on-prod per the migrations gotcha.
- **Worker:** `cmd/rollup-company` gains a second delete+rebuild in its existing transaction.
- **DB layer:** new `DeleteAllInsightsCompanyGrowth` / `RebuildInsightsCompanyGrowth` / `ListInsightsCompanies` queries in `internal/db/queries/insights.sql`; regenerated sqlc.
- **HTTP:** new handler `InsightsCompanies` in `internal/handler/insights.go` + route in `handler.go`. No auth, no frontend, no search impact.
