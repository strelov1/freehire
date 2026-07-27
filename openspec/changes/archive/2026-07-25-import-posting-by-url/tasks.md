## 1. Extract the import path

- [x] 1.1 Create `internal/linkimport`: build the link-source registry (host-scoped
  adapters, then `NewGeneric`) and write a resolved vacancy through `job.New` →
  `UpsertJob` + `EnqueueJobEnrichment` in one transaction, with the best-effort search
  push — lifted from `cmd/resolve-url` without behavior change. Expose one entry point
  that takes a URL and reports the written posting's slug, or that nothing parsed.
- [x] 1.2 Rewrite `cmd/resolve-url` as a CLI over the package (argument/stdin reading and
  logging stay in the command).
- [x] 1.3 Integration test: a page whose HTML carries a `JobPosting` block is written under
  `weblink` with the page URL as its `external_id`, enqueued for enrichment, and answers
  its slug; a page without one reports "not parsed" and writes nothing; a re-import of the
  same URL yields the same posting and no second row.

## 2. Reusable catalog lookup

- [x] 2.1 Extract the two-tier resolve from `FindJob` (identity, then stored URL) into
  `catalogSlugForURL`, called by both `FindJob` and the new handler; `/jobs/find` behavior
  unchanged and its existing tests stay green.

## 3. The endpoint

- [x] 3.1 `POST /api/v1/jobs/resolve`: catalog lookup first (200 `found`), then import
  (201 `imported`), then contribution triage (202 `queued`); non-`http(s)` is 422.
- [x] 3.2 Wire the route with `keyAuth` and the contribution limiter built once and shared
  with `POST /me/contributions`, so both count against one per-user budget.
- [x] 3.3 Handler integration tests: found / imported / queued / 422 / 401, and a second
  submit of an imported URL answering the same slug without a second row.

## 4. Verification

- [x] 4.1 `go build ./...`, `go vet ./...`, `go test ./...` (unit, whole repo) and
  `go test -tags=integration ./internal/db/ ./internal/handler/ ./internal/linkimport/
  ./internal/contribution/` — all green (535s / 428s / 31s / 31s).
- [ ] 4.2 Confirm `cmd/resolve-url` still imports a real URL end to end after the
  extraction. **Needs a database and a live page — not run here.**
