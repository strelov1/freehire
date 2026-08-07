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

- [x] 7.1 `go build ./... && go vet ./... && gofmt -l .` clean.
- [x] 7.2 `go vet -tags=integration ./...` clean (per this repo's pre-push gate).
- [x] 7.3 `go test ./...` green.
- [x] 7.4 Dry-run against the live Accenture board from a scratch harness (no ingest
      write — list-only). **Result, and a real bug this step caught**: the first dry
      run showed `pickSplitDimension` picking the dimension with the *largest*
      single value instead of the *smallest* — backwards, since it should minimize
      the worst remaining branch, not maximize it — which made no real progress
      splitting Accenture's largest job family across 5+ minutes. Fixed (see
      `internal/sources/workday.go`'s `pickSplitDimension`) and covered by a new
      regression test (`TestWorkdaySplitPicksDimensionWithSmallestMaxValue`) that
      the old test suite could not have caught, since every existing fixture only
      ever offered one candidate dimension at a time. After the fix, Accenture's
      "Software Engineering" family (~24,221 postings) still exhausts
      `maxFacetDepth` on its worst branch — confirmed this is a genuine ceiling of
      Workday's own facet set for this tenant (see design.md's Non-Goals), not a
      remaining code defect: `timeType` is 99.96% one value and `locationMainGroup`
      is empty once `jobFamilyGroup` is applied, leaving no third useful dimension.
      The exhausted branch still returns up to 2,000 correct, distinct postings
      (never past offset 2,000, so never the wraparound bug this change fixes) —
      still a large improvement over one 2,000 ceiling for the whole board, just not
      100% complete for this one extreme tenant. Full end-to-end completion was not
      observed within a scratch harness's timeout (sequential, no added
      concurrency); watch actual duration at rollout (8.3).
- [x] 7.5 Dry-run against a smaller confirmed-capped board
      (`nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite`): **passed twice**,
      2,617 and 2,615 postings respectively (small run-to-run drift expected — the
      board changes between runs), both comfortably past the old 2,000 ceiling and
      with no facet-depth exhaustion. This is the representative case for the ~108
      of ~110 affected boards that aren't Accenture-scale.

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
