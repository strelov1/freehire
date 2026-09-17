## 1. Data layer: reply-rate queries

- [x] 1.1 Add a `GetGlobalCompanyResponse` sqlc query to `internal/platform/db/queries/insights.sql` that sums `applications`/`answered` across every row of `insights_company_response`.
- [x] 1.2 Add a `GetUserResponseRate` sqlc query to the same file, scoped to one `user_id`, mirroring `RebuildInsightsCompanyResponse`'s `observable`/`answered` CTEs (connected-mailbox gate, non-retracted `employer_reply`) instead of grouping by `company_slug`.
- [x] 1.3 Run `make sqlc` and confirm the generated code compiles.

## 2. Service layer: personal benchmark computation and gating

- [x] 2.1 Add a `ReplyRateBenchmark` type to `internal/application/userjob` (you/global application and answered counts).
- [x] 2.2 Add `ReplyRate *ReplyRateBenchmark` (`json:"reply_rate,omitempty"`) to `userjob.Pipeline` in `internal/application/userjob/counts.go`.
- [x] 2.3 Add a repository method to `internal/application/jobtracking/repository.go` wrapping the two new sqlc queries.
- [x] 2.4 Implement the sample-gate helper: both the caller's own observable count and the global observable count must be ≥ 10, else the benchmark is absent (`nil`) — no separate "mailbox connected" branch, since a caller with no mailbox already has an observable count of zero.
- [x] 2.5 Wire the new repository calls and the gate into `Service.Pipeline` in `internal/application/jobtracking/jobtracking.go`, alongside the existing `PipelineCounts` call.

## 3. Frontend: Pipeline tab comparison card

- [x] 3.1 Add the optional `reply_rate` field to the pipeline response TS type consumed by the Pipeline tab.
- [x] 3.2 Add a reply-rate comparison card to `PipelineView.svelte` (or a small extracted component), rendered only when `reply_rate` is present, laid out beside the existing Interview Rate / Offer Rate donut cards.
- [x] 3.3 Compute the displayed percentages client-side from the raw counts, following the `interviewRate`/`offerRate` precedent in `web/src/lib/pipeline.ts` rather than serving a pre-divided rate from the backend.

## 4. Verification

- [x] 4.1 Unit tests in `internal/application/jobtracking/jobtracking_test.go` for the gating helper: both sides clear the gate, caller below gate, global below gate, caller has no connected mailbox (observable count zero).
- [x] 4.2 Integration test (build-tagged) for `GetGlobalCompanyResponse` and `GetUserResponseRate`, mirroring the existing `RebuildInsightsCompanyResponse` integration test's fixture shape.
- [x] 4.3 Frontend test for the comparison card's presence/absence in `PipelineView`.
- [x] 4.4 `gofmt -l .`, `go vet ./...`, `go test ./...`, and the SPA's `pnpm run check` all pass. (`cmd/billing-sync`'s `TestTheStoreProviderAloneKeepsTheWorkerRunning` fails on this machine only — a pre-existing, unrelated local port collision on `localhost:5432` with a sibling project's Postgres container, not touched by this change; verified root cause, see session notes.)
