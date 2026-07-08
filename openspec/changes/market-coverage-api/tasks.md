## 1. Backend: extract shared coverage step

- [x] 1.1 Extract `coverageFor(ctx, roleFilter any, coverageSkills, declared, body, all []string) (verdict.Verdict, error)` from `computeCoverage` in `internal/handler/resume_verdict.go` (the three facet queries + `verdict.Compute`); rewire `computeCoverage`/`GetResumeVerdict` to call it. `coverageSkills` drives covered/uncovered; declared/body/all score the breakdown (the two differ for the CV verdict). Existing verdict tests stay green (behavior-preserving).

## 2. Backend: stateless market-coverage endpoint

- [x] 2.1 RED: handler unit test `internal/handler/market_coverage_test.go` (fake facetCounter, no Docker) — `POST /market/coverage` with skills body + facet query returns coverage `data`; skills-from-body reach `AndNotSkills`, filter-from-query reaches the role query, supplied skills do NOT filter the market; empty skills → 400; too-many skills → 400; no search → 503. (Route-level 401/API-key acceptance is delivered by the shared `keyAuth` middleware — tested where that middleware is tested — see 2.3.)
- [x] 2.2 GREEN: `internal/handler/market_coverage.go` — parse `{"skills":[...]}` from body, trim/drop empties + cap at `maxCoverageSkills`, build filter via `marketFilter(c)` (full vocabulary, skills stripped), call `coverageFor(ctx, filter, skills, skills, nil, skills)`, return `{"data": verdict}` with `coherence_percent` zeroed.
- [x] 2.3 Register `POST /api/v1/market/coverage` behind `keyAuth` in `internal/handler/handler.go` (next to the other `keyAuth` job routes).
- [x] 2.4 Confirm the `verdict.Verdict` TS contract still regenerates cleanly (`cmd/gen-contracts`) — no new field, no `web/` change.

## 3. CLI (freehire-cli repo): client + command

- [ ] 3.1 RED: `internal/client/client_test.go` — `Coverage(skills, facets)` sends `POST /api/v1/market/coverage`, skills JSON body, facets as query; returns raw `data`; error envelopes → `*APIError`.
- [ ] 3.2 GREEN: add `Coverage` to `internal/client/client.go`.
- [ ] 3.3 RED: `internal/cli/*_test.go` — `market-fit --skills go,react [--facet k=v]` hits the right path, sends skills + facets, prints coverage + gaps table; `--json` passthrough.
- [ ] 3.4 GREEN: new `internal/cli/marketfit.go` command wired into root.
- [ ] 3.5 Add generic `--facet key=value` (repeatable) filtering to `market-fit` and `search`, plus high-traffic named flags (`--country`, `--city`, `--role`, `--employment-type`, `--english-level`, `--salary-min`, `--visa`); fold all into `url.Values`.
- [ ] 3.6 Update `DESIGN.md` / `README.md` with the `market-fit` command and the fuller filter flags.

## 4. Finish

- [ ] 4.1 `go build ./... && go vet ./... && go test ./...` (both repos); backend queue/integration tests where relevant.
- [ ] 4.2 Manual verification against a running stack: post a skill list, confirm coverage/gaps; exercise a non-default facet.
