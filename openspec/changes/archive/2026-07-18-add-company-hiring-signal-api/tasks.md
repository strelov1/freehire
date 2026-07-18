## 1. Schema

- [x] 1.1 Add migration `migrations/0030_insights_company_growth.sql` creating `insights_company_growth(company_slug text PRIMARY KEY, open_count int NOT NULL DEFAULT 0, open_count_prev int NOT NULL DEFAULT 0)` plus an index on `(open_count DESC)` (serves the `open` sort and the `min_open` filter), with a header comment matching the `insights_*` rollup convention (pure function of `jobs`, atomic rebuild by `cmd/rollup-company`, initdb-only + manual-on-prod).

## 2. Growth-scalar rollup (TDD)

- [x] 2.1 RED: add a failing `//go:build integration` test in `internal/db` seeding jobs (open now, open-then-closed within/before the 30d window, a duplicate, an empty-slug), running the growth rebuild, and asserting per-company `open_count` / `open_count_prev` and that duplicates/empty-slug rows are excluded.
- [x] 2.2 GREEN: add `DeleteAllInsightsCompanyGrowth` and `RebuildInsightsCompanyGrowth` (takes a `prev_ts`) to `internal/db/queries/insights.sql` using the `count(*) FILTER (…)` idiom over canonical rows; regenerate sqlc; make the test pass.
- [x] 2.3 Extend `cmd/rollup-company` to delete+rebuild `insights_company_growth` in the SAME transaction as `insights_company_stats`, computing `prev_ts = now − 30d` (a `growthWindowDays` const, as `cmd/rollup-stats`); update its log line + package doc.
- [x] 2.4 REFACTOR + simplify under green.

## 3. Leaderboard read + endpoint (TDD)

- [x] 3.1 RED: add a failing handler integration test (`internal/handler`) wiring `GET /api/v1/insights/companies`, asserting: `sort=growth` orders by `growth_30d` desc; `sort=-growth` orders asc; `min_open` filters; `sort=bogus` → 400; `limit` capped; `{data,meta}` envelope with `company_slug`/`company_name`/`open_now`/`open_prev_30d`/`growth_30d`.
- [x] 3.2 GREEN: add `ListInsightsCompanies` to `internal/db/queries/insights.sql` (LEFT JOIN `companies` for `company_name`, `WHERE open_count >= @min_open`, `ORDER BY` per sort, `LIMIT @lim`); regenerate sqlc. Add `InsightsCompanies` handler + `parseCompaniesSort` (growth/-growth/open) + `min_open` parse in `internal/handler/insights.go`, and wire the route in `internal/handler/handler.go`. Make the test pass.
- [x] 3.3 REFACTOR + simplify under green.

## 4. Docs

- [x] 4.1 Update `AGENTS.md` — note `cmd/rollup-company` now rebuilds both `insights_company_stats` and `insights_company_growth`; add `/api/v1/insights/companies` to the endpoint surface description if the insights endpoints are listed.
