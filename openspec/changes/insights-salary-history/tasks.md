## 1. Effective salary

- [x] 1.1 Integration test: postings with only manual / only source / only enrichment / all three salaries, one non-technical with only a source salary; assert the bands' samples and that the manual tier wins with its own currency and period — fails on main
- [x] 1.2 Migration: `jobs_effective_salary` view; lint with squawk
- [x] 1.3 Both band rebuilds read the view; `make sqlc`; existing salary tests stay green

## 2. Monthly rollup

- [x] 2.1 Integration test: publication-month bucketing (late-found posting, closed posting), the 30 floor, currency/period separation, canonical/public/categorized population, 12-month window
- [x] 2.2 Migration: `insights_salary_monthly` table (same migration as 1.2)
- [x] 2.3 `DeleteAllInsightsSalaryMonthly`, `RebuildInsightsSalaryMonthly`, `ListInsightsSalaryHistory`; `make sqlc`; wire into `cmd/rollup-stats`

## 3. Endpoint

- [x] 3.1 Handler integration test: rows for the category only, ordering, 400 for missing/unknown category, aggregate-only
- [x] 3.2 `GET /api/v1/insights/salary/history` handler + route
- [x] 3.3 `go vet -tags=integration,llmlive ./...`, gofmt, unit tests

## 4. Page

- [x] 4.1 Pure chart model with vitest tests: default series, ≥3-point rule, gaps split segments, partial current month, y scale
- [x] 4.2 API client + load in `+page.server.ts` (a failed read drops the chart, short cache)
- [x] 4.3 Chart component (SVG, median line + p25–p75 band, series switch, hidden data table, caveat) on the salary page
- [x] 4.4 svelte-check, eslint, vitest; SSR check without JS; screenshots at 1440 and 390 px

## 5. Ship

- [ ] 5.1 `EXPLAIN` the monthly rebuild on prod; note the cost in the PR
- [ ] 5.2 PR with the band before/after for one non-technical category; CodeRabbit handled
