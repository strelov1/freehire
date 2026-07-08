## 1. Backend: extract shared coverage step

- [ ] 1.1 Extract `coverageFor(ctx, roleFilter any, declared, body, all []string) (verdict.Verdict, error)` from `computeCoverage` in `internal/handler/resume_verdict.go` (the three facet queries + `verdict.Compute`); rewire `computeCoverage`/`GetResumeVerdict` to call it. Existing verdict tests stay green (behavior-preserving).

## 2. Backend: stateless market-coverage endpoint

- [ ] 2.1 RED: integration test `internal/handler/market_coverage_integration_test.go` — `POST /api/v1/market/coverage` with skills body + facet query returns coverage `data`; empty skills → 400; no search → 503; API-key auth accepted; anonymous → 401.
- [ ] 2.2 GREEN: `internal/handler/market_coverage.go` — parse `{"skills":[...]}` from body, build filter via `buildSearchFilter(c)`, call `coverageFor(ctx, filter, skills, nil, skills)`, return `{"data": verdict}` with `coherence_percent` omitted/zeroed.
- [ ] 2.3 Register `POST /api/v1/market/coverage` behind `keyAuth` in `internal/handler/handler.go` (next to the other `keyAuth` job routes).
- [ ] 2.4 Confirm the `verdict.Verdict` TS contract still regenerates cleanly (`cmd/gen-contracts`) — no new field, so no web change expected.

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
