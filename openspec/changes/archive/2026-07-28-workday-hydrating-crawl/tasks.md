## 1. Board-scoped seen-set

- [x] 1.1 Add a failing integration test in `internal/db/jobs_existing_ids_integration_test.go`: two boards of one provider whose names share a prefix (`acme.wd5.myworkdayjobs.com/acmeus`, `…/acmeca`) each hold a posting; the board-scoped query returns only the queried board's `external_id`s. Cover the sibling-prefix case explicitly — `acmeus:` must not match a row of `acmeca:`.
- [x] 1.2 Add `ExistingExternalIDsByBoard` to `internal/db/queries/jobs.sql` (`WHERE source = $1 AND external_id LIKE $2`, the caller passing `"<board>:%"`), documenting that it rides `jobs_source_extid_pattern_idx` and that a range predicate is wrong here because the database collation does not order punctuation byte-wise. Run `make sqlc`.
- [x] 1.3 Widen `pipeline.seenLookup` and `cmd/ingest/store.go`'s `ExistingExternalIDs` to take the board; an empty board keeps the provider-wide query (boardless adapters). `fetchBoard` passes `e.Board`.
- [x] 1.4 Run `go test ./internal/pipeline/... ./cmd/ingest/...` and `go test -tags=integration ./internal/db/`.

## 2. Catalogue-fit check on a liveness refresh

- [x] 2.1 Add a failing test in `internal/pipeline`: a hydrating adapter yields a `SeenRefresh` posting whose title the non-tech dictionary flags; the store records NO touch and the run counts one rejection, not one skip.
- [x] 2.2 Route the `SeenRefresh` branch of `ingestBoard` through the catalogue-fit check before `touch`, counting a flagged posting as a rejection. Keep the rejection denominator honest — a refresh that reaches the filter is a candidate.
- [x] 2.3 Confirm the existing hydration tests stay green (a refresh of an in-catalogue posting still touches, still does not re-upsert content).

## 3. Workday hydration

- [x] 3.1 Add a failing test in `internal/sources/workday_test.go`: with a seen predicate that reports one of two listed postings as seen, the fake transport records exactly one detail GET; the seen posting is emitted with `SeenRefresh` set and carries its namespaced-resolvable external id, title and URL.
- [x] 3.2 Add a failing test that `FetchNew` with a nil/empty seen-set fetches every posting's detail (the fallback path).
- [x] 3.3 Implement `workday.FetchNew`, sharing the existing `listPostings` walk and `fetchDetails` pool with `Fetch`; a seen posting short-circuits to a listing-only job with `SeenRefresh = true`.
- [x] 3.4 Run `go test ./internal/sources/...` — new tests pass and `TestWorkdayPagesByFirstPageTotal` plus the other workday tests stay green.

## 4. Whole-change verification

- [x] 4.1 `go build ./... && go vet ./... && gofmt -l .` clean.
- [x] 4.2 `go test ./...` green; `go test -tags=integration ./internal/db/` green.
- [x] 4.3 Dry-run the real board against the live API from a scratch harness: crawl `dollartree.wd5.myworkdayjobs.com/dollartreeus` with a seen-set standing in for the catalogue and confirm the run completes without a 429 and issues detail requests only for unseen postings. **Result: `OK in 15m52s: 23920 postings, 23920 refreshes, 0 hydrated` — the board crawls end to end, where every crawl since 2026-07-02 failed on 429. The 1,196 listing pages still cost ~16 minutes, which fits the 40-minute shard timeout and runs concurrently with the shard's other boards, but it is the remaining floor for a board this size.**

## 5. Rollout (ops — executed at Finish)

- [x] 5.1 Deployed 2026-07-28 (`release.sh freehire`, hire-green). A manual single-board reingest gave `ingested=22936 failed=0 skipped=0 rejected=570`; `board_health` now records `last_success_at=2026-07-28 22:44:14` and `last_ingested_count=22936`, where it had been NULL since the board was added.
- [x] 5.2 The sweep ran in the same pass (the unseen rows were 26 days stale, well past the 48h cutoff): `closed 2243 stale workday jobs`, and `manager-inventory-management-dollar-tree-a3hqkgo6` carries `closed_at=2026-07-28 22:44:14`. Dollar Tree's open rows are now the live board's own 22,925 rather than a month-old snapshot. NOTE: a sweep-closed job leaves the Meili index only on the next `cmd/reindex` — incremental indexing hangs off `UpsertJob` and `CloseUnseenJobs` bypasses it.
- [x] 5.3 31 workday boards still have `last_success_at IS NULL`, by cause: **422 x18, 429 x7, 403 x4, 404 x2**. Only the seven 429s are what this change addresses — they are the same rate-limit shape as Dollar Tree and should recover as their shard next fires. The other 24 fail for unrelated reasons (a wrong or retired board id, or the tenant blocking our egress) and need their own pass; out of scope here.
