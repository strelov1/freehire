## 1. Backend: extract shared coverage step

- [x] 1.1 Extract `coverageFor(ctx, roleFilter any, coverageSkills, declared, body, all []string) (verdict.Verdict, error)` from `computeCoverage` in `internal/handler/resume_verdict.go` (the three facet queries + `verdict.Compute`); rewire `computeCoverage`/`GetResumeVerdict` to call it. `coverageSkills` drives covered/uncovered; declared/body/all score the breakdown (the two differ for the CV verdict). Existing verdict tests stay green (behavior-preserving).

## 2. Backend: stateless market-coverage endpoint

- [x] 2.1 RED: handler unit test `internal/handler/market_coverage_test.go` (fake facetCounter, no Docker) — `POST /market/coverage` with skills body + facet query returns coverage `data`; skills-from-body reach `AndNotSkills`, filter-from-query reaches the role query, supplied skills do NOT filter the market; empty skills → 400; too-many skills → 400; no search → 503. (Route-level 401/API-key acceptance is delivered by the shared `keyAuth` middleware — tested where that middleware is tested — see 2.3.)
- [x] 2.2 GREEN: `internal/handler/market_coverage.go` — parse `{"skills":[...]}` from body, trim/drop empties + cap at `maxCoverageSkills`, build filter via `marketFilter(c)` (full vocabulary, skills stripped), call `coverageFor(ctx, filter, skills, skills, nil, skills)`, return `{"data": verdict}` with `coherence_percent` zeroed.
- [x] 2.3 Register `POST /api/v1/market/coverage` behind `keyAuth` in `internal/handler/handler.go` (next to the other `keyAuth` job routes).
- [x] 2.4 Confirm the `verdict.Verdict` TS contract still regenerates cleanly (`cmd/gen-contracts`) — no new field, no `web/` change.

## 3. CLI (freehire-cli repo): client + command

- [x] 3.1 RED: `internal/client/client_test.go` — `Coverage(CoverageParams)` sends `POST /api/v1/market/coverage`, skills JSON body, facets as query; returns raw `data`.
- [x] 3.2 GREEN: add `Coverage`/`CoverageParams` to `internal/client/client.go`.
- [x] 3.3 RED: `internal/cli/cli_test.go` — `market-fit --skills go,docker --skills react --category backend --facet source=greenhouse` sends skills in the body (not query), facets in the query, prints `Coverage: N%` + gaps; missing `--skills` errors.
- [x] 3.4 GREEN: new `internal/cli/marketfit.go` command wired into root.
- [x] 3.5 Extract shared facet flags (`internal/cli/facets.go`: `addFacetFlags`/`facetsFromFlags`) — named high-traffic facets (`--region/--country/--city/--company/--category/--role/--seniority/--employment-type/--english-level`, `--remote`, `--salary-min`, `--visa`) + generic `--facet key=value`; wired into `market-fit` and refactored `search` onto it. `--skills` stays per-command (filter in search, measured set in market-fit).
- [x] 3.6 Update `DESIGN.md` / `README.md` with the `market-fit` command and the fuller filter flags.

## 4. Finish

- [x] 4.1 `go build ./... && go vet ./... && go test ./...` green in both repos.
- [x] 4.2 End-to-end verification: `market_coverage_integration_test.go` (real Postgres via testcontainers) — anonymous POST → 401, API-key POST → 200 with the coverage payload, and the query facet reaches the role query. (Facet backend stubbed; the Meili `FacetCounts` path is unchanged and covered by the verdict/search suites.)
