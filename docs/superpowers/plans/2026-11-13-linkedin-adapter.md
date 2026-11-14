# LinkedIn Adapter Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ingest public LinkedIn postings and close only on explicit page evidence.

**Architecture:** A bounded hydrating source shares its posting parser with a paced
liveness pass. Registry capabilities exempt it from unseen sweeps without pretending
it emits a change stream.

**Tech Stack:** Go, existing HTML transport, golang.org/x/net/html, existing rate limiter.

**Spec:** [LinkedIn source adapter](../specs/2026-11-13-linkedin-adapter-design.md)

## Global Constraints

- Public requests only; no credentials or production configuration changes.
- Unknown responses must never close or reopen a posting.
- Reuse existing pipeline identities and two-strike liveness writes.

## Review Focus

- Search truncation must not become a mass-close signal.
- Recommendation text must not supply the primary job's closure signal.
- Redirects and canonical IDs must identify the requested posting.
- All-unreadable details must fail rather than report an empty successful crawl.
- Rate refusal must stop the liveness pass and preserve state.

### Task 1: Source and safe lifecycle classification

- [x] Add parser, bounded crawl, hydration, identity, and sweep-exemption tests.
- [x] Run focused tests for the new adapter.
- [x] Implement `linkedin.go`, register the adapter, and expose an unseen-sweep exemption
  that distinguishes probe-only closure from a change stream.
- [x] Run sources tests.

### Task 2: Paced lifecycle verification

- [x] Add HTTP probe tests for live, closed, blocked, missing, and redirected pages.
- [x] Implement the LinkedIn paced pass using the source parser; add it to routing.
- [x] Run liveness and ingest tests, then Go tests, vet, and tagged compilation.
- [x] Document board configuration, limits, and explicit closure semantics.

## Execution notes

- Public spike snapshots were parsed by the Go implementation: job 4489559466 was live,
  job 4482050531 was explicitly closed; both retained title, employer and HTML body.
- Independent review found that returning jobs together with an error loses the whole
  buffered pipeline batch. A regression test failed for both a later-page timeout and a
  later detail refusal; the adapter now logs and preserves useful partial progress.
- The reviewer verified that fix and reported no remaining important findings.
- Full Go unit tests, vet, integration-tagged vet and integration tests were started.
  Final verification results are recorded below.

## Verification

- `go test -p 2 ./...`: passed on the final full run. The initial run exposed the
  reviewed provider-set fixture needing LinkedIn and a one-second timeout in
  `TestContributionLimiter_IsKeyedOnTheUser`; both were resolved before the final run.
- `go test -p 2 ./internal/ingest/sources ./cmd/liveness ./cmd/ingest`: passed.
- `go vet -p 2 ./...` and `go vet -p 2 -tags=integration ./...`: passed.
- `golangci-lint run --new-from-rev=HEAD --timeout=5m
  ./internal/ingest/sources/... ./cmd/ingest/... ./cmd/liveness/...`: zero issues.
- `gofmt -l .`: empty; `git diff --check`: clean.
- `make gen-contracts`: regenerated the source vocabulary with LinkedIn.
- `pnpm check:links`: all 291 relative links resolve.
- The broad integration run hit existing handler request timeouts (1000 ms) in
  `TestRoastCV_RealPDFFromAnAnonymousCaller`,
  `TestRoastCV_UndecodablePDFIsA400NotA500`, `TestEmployerEndToEnd`,
  `TestOGOpen_ServesAPNGFromThePublishedSnapshot`, `TestOGAbout_DegradesRatherThanFailing`,
  and `TestOGOpen_SurvivesEverythingFailing`. All six passed in a separate
  integration-tagged retry. These unrelated tests were not changed.

- The complete integration-tagged run finished with only the six handler timeout
  failures listed above; their separate retry passed. The integration-tagged
  `cmd/ingest`, `cmd/liveness`, source adapters, and database tests all passed.
- No commits, production boards, schedules, or deployments were created.
