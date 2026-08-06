## 1. Facet-scoped listing (single level)

- [x] 1.1 Add a failing test in `internal/sources/workday_test.go`: page 1 reports
      `total: 2000` and carries a `facets` array with one dimension
      (`jobFamilyGroup`, two values); the adapter re-queries once per facet value
      with `appliedFacets` set and returns the union of postings, instead of
      paging the unfiltered walk past offset 2000.
- [x] 1.2 Add `workdayCapTotal = 2000` and a `facets`/`appliedFacets` decode to the
      page-response struct in `listPostings`.
- [x] 1.3 Implement the single-level split: on `total == workdayCapTotal`, pick the
      facet dimension present in the response with the highest single-value count,
      and page each value's slice via `appliedFacets` instead of the unfiltered walk.
- [x] 1.4 Run `go test ./internal/sources/... -run Workday` — new test green,
      `TestWorkdayPagesByFirstPageTotal` and the rest of the existing suite stay green.

## 2. Depth-bounded recursion across dimensions

- [x] 2.1 Add a failing test: a facet value's own slice still reports `total: 2000`
      and carries a second, different facet dimension; the adapter recurses into
      that dimension (not re-applying the one already used) instead of stopping.
- [x] 2.2 Add a failing test: recursion stops at `maxFacetDepth = 3` combined
      dimensions — a slice still capped at that depth is paged as-is (best effort),
      not recursed further.
- [x] 2.3 Implement `maxFacetDepth` and the "not-yet-used dimension" selection,
      threading which dimensions are already applied down each recursion branch.
- [x] 2.4 Run `go test ./internal/sources/... -run Workday`.

## 3. Dedup across slices

- [x] 3.1 Add a failing test: two recursed slices both return a posting with the
      same `externalPath` (simulating a multi-counting dimension like
      `workerSubType`); the final result contains it once.
- [x] 3.2 Implement dedup by `ExternalPath` when merging slice results in
      `listPostings`.
- [x] 3.3 Run `go test ./internal/sources/... -run Workday`.

## 4. Depth-exhaustion and no-dimension logging

- [x] 4.1 Add a failing test (capturing `log` output or via a seam already used
      elsewhere in this package) confirming a `log.Printf` warning fires when a
      slice is still capped at `maxFacetDepth`, or when a capped response carries
      no usable not-yet-applied facet dimension at all.
- [x] 4.2 Implement the log line(s), matching the `provider: message` convention
      already used in `config.go` / `eightfold.go` / `djinni.go`.
- [x] 4.3 Run `go test ./internal/sources/... -run Workday`.

## 5. Boards below the cap are unaffected

- [x] 5.1 Add/confirm a test: `total < workdayCapTotal` on page 1 pages exactly as
      today, issuing no `appliedFacets` requests. (May already be covered by
      existing tests — extend if the new code path is reachable without gating.)
- [x] 5.2 Run the full `internal/sources` suite to confirm no regression to any
      other provider or to Workday's non-capped path.

## 6. 403 retry

- [x] 6.1 Add a failing test: a `PostJSON`/`GetJSON` call returns a 403 once, then
      succeeds; the Workday adapter retries and returns the successful result
      instead of failing the board.
- [x] 6.2 Add a failing test: a non-retryable error (e.g. 404) returns immediately
      without retrying.
- [x] 6.3 Implement a Workday-local retry wrapper mirroring `eightfold.go`'s
      `getJSONRetrying`/`isRateLimited` shape (403 and 429, capped exponential
      backoff), applied to both the listing `PostJSON` and detail `GetJSON` calls.
- [x] 6.4 Run `go test ./internal/sources/... -run Workday`.

## 7. Whole-change verification

- [ ] 7.1 `go build ./... && go vet ./... && gofmt -l .` clean.
- [ ] 7.2 `go vet -tags=integration ./...` clean (per this repo's pre-push gate).
- [ ] 7.3 `go test ./...` green.
- [ ] 7.4 Dry-run against the live Accenture board
      (`accenture.wd103.myworkdayjobs.com/AccentureCareers`) from a scratch harness
      (no ingest write — list-only): confirm the crawl now collects on the order of
      tens of thousands of postings instead of 2,000, and completes without an
      unhandled 403. Record the actual count reached.
- [ ] 7.5 Dry-run against one of the smaller confirmed-capped boards (e.g.
      `nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite`) and confirm its
      collected count now exceeds 2,000 and roughly matches its facet-sum estimate.

## 8. Rollout (ops — executed at Finish)

- [ ] 8.1 Deploy, then watch the next `workday` shard run for the confirmed-capped
      boards: `board_health.last_ingested_count` for Accenture, NVIDIA, and Babilou
      rises well past 2,000.
- [ ] 8.2 Confirm no new 403-driven board failures appear in `board_health` for
      Workday boards after rollout (the retry should reduce, not increase, failures).
- [ ] 8.3 Note the crawl-time cost for the largest affected boards (Accenture in
      particular) against the shard timeout, the way `workday-hydrating-crawl`
      tracked Dollar Tree's — subdivision trades fewer-but-larger boards' request
      count up significantly and this should be watched once live.
