## Why

Some Workday tenants (e.g. Accenture) cap the `total` their listing API reports at
2,000 and, past `offset=2000`, silently loop back to page 1 instead of erroring. The
current adapter (`internal/sources/workday.go`) latches that capped total and stops
there, so a single crawl of an affected board can never retrieve more than 2,000 of
its postings — with no error, no log line, and nothing distinguishing "read the whole
board" from "hit the ceiling." Verified live against Accenture's board
(`accenture.wd103.myworkdayjobs.com/AccentureCareers`, 2026-08-06): reported total
2,000 against a facet-count sum of ~51,500. A sample of 120 boards from
`sources/workday.yml` found the ceiling on ~1.7% of boards, extrapolating to roughly
110 of 6,462 — skewed toward the largest employers, since the cap only bites tenants
large enough to exceed it.

Separately, `internal/sources/http.go` treats HTTP 403 as fatal for every provider,
failing the whole board on one rate-limit response; `eightfold.go` already treats 403
as retryable for that provider. Workday needs the same per-provider judgment call, and
fixing it alongside the cap issue avoids a second board-drop failure mode surfacing
mid-crawl of a now much larger walk.

## What Changes

- `workday.listPostings` detects a capped board (`total == 2000` on page 1) and, only
  then, subdivides the walk by the listing response's own facet dimensions
  (`appliedFacets`), recursing into any still-capped slice with a different,
  not-yet-applied facet dimension, up to a bounded depth.
- Postings collected across slices are deduped by `ExternalPath` before being
  returned, since some facet dimensions multi-count.
- Boards below the cap are unaffected — same request count and pagination as today.
- If a slice is still capped after the depth limit, the adapter logs a warning
  (`log.Printf`, matching this package's existing convention) instead of silently
  truncating, and returns what it collected.
- Workday's HTTP calls retry on 403 (in addition to the shared client's existing
  429/5xx retries), mirroring `eightfold.go`'s `isRateLimited`/retry pattern.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — this fixes a defect against `source-ingest`'s existing requirement that an
adapter "fetch all current postings for one configured board." No SHALL-level
behavior is changing; the Workday adapter currently violates that requirement on
capped boards and this change makes it conform.

## Impact

- `internal/sources/workday.go`: `listPostings` becomes recursive; a new
  403-retry wrapper around `s.http` calls.
- `internal/sources/workday_test.go`: new test cases for facet-triggered
  subdivision, recursion depth, dedup, depth-exhaustion logging, and 403 retry.
- No schema, API, or config changes. No change to unaffected (non-capped) boards'
  behavior or request volume.
- Increased request volume during crawls of the ~110 affected boards (e.g. Accenture
  goes from ~100 listing requests to on the order of 2,500 across facet slices),
  running under the crawl's existing concurrency and retry handling.
