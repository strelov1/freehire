## 1. Filter-vocabulary helper

- [x] 1.1 Add `ActiveFilterParams(v url.Values) []UnknownParam` to `internal/search/search/query_params.go`, reusing `knownParams(nil)`; no `DidYouMean` populated.
- [x] 1.2 Unit test: a request with `work_mode=remote&salary_min=1000&bogus=1` reports `work_mode` and `salary_min` (sorted), not `bogus`; an empty `url.Values` reports nothing.

## 2. Retry in runJobSearch

- [x] 2.1 Add a `dropped []search.UnknownParam` return value to `runJobSearch` (`internal/api/handler/search.go`).
- [x] 2.2 On a `Search` error: if `errors.Is(err, search.ErrBadQuery)` and the original `Filter` was non-nil, retry once with `Filter` omitted (query/sort/vector/pagination unchanged); on retry success set `dropped = search.ActiveFilterParams(queryValues(c))` and proceed as success. On any other error, or when the retry itself fails, return the error unchanged (existing behavior).
- [x] 2.3 Unit test (fake `searcher` returning `ErrBadQuery` once then a result): a filtered request degrades to the retried result with `dropped` naming the active filter params. (`TestSearchJobs_AFilterRejectionDegradesToAWidenedResult`)
- [x] 2.4 Unit test: a filter-less request that somehow errors with `ErrBadQuery` does not retry. (`TestSearchJobs_NoFilterNeverRetries`)
- [x] 2.5 Unit test: a filtered request whose error is NOT `ErrBadQuery` does not retry and returns the original error. (`TestSearchJobs_ANonFilterFailureIsNotDegraded`)
- [x] 2.6 Unit test: a filtered request where BOTH the primary and the retry fail returns an error, not a degraded success. (`TestSearchJobs_ARetryThatAlsoFailsStillFails`)

## 3. Wire the report into both callers

- [x] 3.1 `SearchJobs` (`search.go`): merge `dropped` into the ignored-params report via `search.SortAndCap(append(dropped, ignoredParams(c, searchParams)...))`.
- [x] 3.2 `AgentSearchJobs` (`agent_search.go`): same merge, using `agentSearchParams`.
- [x] 3.3 Scenario test: covered by `TestSearchJobs_AFilterRejectionDegradesToAWidenedResult`, which already asserts the full JSON response (`meta.ignored_params`) through `SearchJobs`'s real route, not just `runJobSearch` in isolation.

## 4. Documentation correction

- [x] 4.1 Update `internal/search/search/AGENTS.md`'s "Limitations" line ("A Meili filter error 500s the page instead of degrading...") to describe the new degrade behavior, and correct the stale "500" wording (the failure was already classified as `ErrBadQuery` → 400 before this change; this change's own research found that inaccuracy, not this change's own regression).
- [x] 4.2 Note in the same file that the analogous sort-attribute hazard remains open — not fixed by this change.

## 5. Wrap-up

- [x] 5.1 `go vet -tags=integration ./...` and the full test suite for `internal/search/search` and `internal/api/handler`.
- [x] 5.2 `gofmt -l .`, `go vet ./...`, `go test ./...` clean before commit.
