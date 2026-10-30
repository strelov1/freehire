## 1. Honest growth (fix first — it is live today)

- [ ] 1.1 Integration test in `internal/platform/db` (tag `integration`): seed postings covering the four existence cases (old-published/new-seen, new/new, no `posted_at`, `posted_at` later than `created_at`) and assert `open_count_prev` of the role, skill and company-growth rollups — fails on main
- [ ] 1.2 Replace `created_at <= @prev_ts` with `LEAST(posted_at, created_at) <= @prev_ts` in every prior-window count in `internal/platform/db/queries/insights.sql` (role ×2, skill ×3, company growth ×2), with a comment naming why first-seen alone is wrong; `make sqlc`
- [ ] 1.3 Unit test that reads `insights.sql` and fails if any `open_count_prev` / prior-window `FILTER` uses a bare `created_at <=` (guards the rule across all readers)
- [ ] 1.4 Update the "Precomputed insights rollups" wording in `cmd/rollup-stats` and `internal/api/handler/insights.go` comments that describe growth as first-seen

## 2. Role × country rollup

- [ ] 2.1 Migration `0214_insights_role_country_stats.sql`: table `(category, country, open_count, open_count_prev)`, PK `(category, country)`, `country = ''` for all countries; run `pnpm check:sql`
- [ ] 2.2 Integration test: multi-country posting counts once per country and once in `''`; duplicate and private postings excluded; seniority-less posting counted; no-country posting only in `''`
- [ ] 2.3 Queries `DeleteAllInsightsRoleCountryStats`, `RebuildInsightsRoleCountryStatsGlobal`, `RebuildInsightsRoleCountryStatsByCountry` (canonical, public, categorized; the `LEAST` predicate); `make sqlc`
- [ ] 2.4 Call them from `cmd/rollup-stats` inside the existing transaction, beside the role rollup
- [ ] 2.5 Register the new table in `internal/platform/arch/layering` only if the guard requires it (it tracks packages, not tables — confirm, do not assume)

## 3. Endpoint

- [ ] 3.1 Query `ListInsightsRoleCountryColumns` (top N countries by `SUM(open_count)`) and `ListInsightsRoleCountryCells` (cells for `''` plus those countries)
- [ ] 3.2 Handler tests (integration, `internal/api/handler`): default 10 columns ordered by volume; `countries=3` restricts rows; `0` / over max / non-number → 400; aggregate-only payload
- [ ] 3.3 `InsightsRoleCountries` handler in `internal/api/handler/insights.go`, route `GET /insights/role-countries` in `stats.go` beside its siblings; `meta.countries`, `meta.window_days`
- [ ] 3.4 `go vet -tags=integration ./...`, `go test ./...`, `gofmt -l .`

## 4. Page

- [ ] 4.1 Pure helpers in `web/src/lib/` with unit tests: column share, growth percent with the `< 50` floor, ±100% clamp for shading, cell href (pair landing at ≥ 2 × `MIN_PAIR_OPEN` with a landing, else `/jobs` search; all-countries → `/roles/<slug>`)
- [ ] 4.2 API client method + types for the endpoint; fetch it in `web/src/routes/insights/+page.server.ts` beside the gate; hide the grid when `data` is empty
- [ ] 4.3 `RoleCountryGrid.svelte`: semantic `<table>`, sticky first column, open/growth switch, "show all" beyond 15 rows, ▲/▼ markers, caveat line; design-system tokens only — run the `impeccable` skill for the visual pass
- [ ] 4.4 Verify SSR output without JS (curl the page, grep the table and a pair-landing href); `pnpm check` and lint in `web/`
- [ ] 4.5 Look at it in a browser at desktop and phone widths against the local web pointed at prod API; screenshot both views

## 5. Ship

- [ ] 5.1 Before merge: `EXPLAIN` (no `ANALYZE`) the two new rebuild statements on prod and compare the plan with `RebuildInsightsRoleStatsByCountry`'s; read the last `rollup-stats` run duration from its journal so the extra pass's cost is known, not assumed
- [ ] 5.2 PR with the healthcare before/after growth figures in the description; read CodeRabbit, fix valid findings
- [ ] 5.3 Merge, deploy, run `rollup-stats` once, check `/insights` grid and `/insights/roles/healthcare` growth on prod
