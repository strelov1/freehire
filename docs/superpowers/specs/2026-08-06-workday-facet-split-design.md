# Workday: subdivide capped boards by facet, retry 403

## Problem

Workday's CXS jobs-listing API caps the `total` it reports at 2,000 for some tenants.
Past `offset=2000` those tenants return page 1 again rather than an empty page or an
error. `listPostings` (`internal/sources/workday.go:142-177`) latches the first
non-zero `total` and pages until it reaches that number, so on a capped board one
crawl reads at most 2,000 postings and exits cleanly — nothing distinguishes "read the
whole board" from "hit the ceiling."

Verified live against `accenture.wd103.myworkdayjobs.com/AccentureCareers`
(2026-08-06): reported `total: 2000`; the `jobFamilyGroup` facet in that same response
sums to ~51,500; `offset=0` and `offset=2000` return the identical first posting.
A random sample of 120 boards from `sources/workday.yml` (seed 11) found the ceiling
on 2/119 reachable boards (~1.7%), extrapolating to roughly 110 of the file's 6,462
boards — skewed toward the largest employers.

Second, smaller gap: `internal/sources/http.go:493-519` treats HTTP 403 as fatal
(`default` branch, immediate return), failing the whole board on one rate-limit
response. `internal/sources/eightfold.go:133-164` already treats 403 as a retryable
rate-limit signal for that provider (Eightfold returns 403 for throttling, not auth);
Workday needs the same per-provider judgment call.

## Goal

A single Workday crawl must retrieve every posting on a board, even when the board's
own `total` is capped at 2,000, without adding hardcoded per-company logic and without
extra "probe" requests beyond what listing already returns. A transient 403 must not
fail the whole board.

## Non-goals

- Not fixing the general "some boards report total only on page one" case — that is
  already handled (the existing latch).
- Not building a generic cross-provider facet-subdivision abstraction. This is a
  Workday-specific mechanism; nothing today suggests another provider needs it.
- Not guaranteeing 100% completeness on every conceivable board shape — depth is
  bounded (below), and exhausting it without resolving the cap is logged, not silently
  dropped.

## Design

### 1. Detecting a capped board

Trigger condition: the first listing page's `total == workdayCapTotal` (2000, named
constant). A board that genuinely has exactly 2,000 postings triggers the same path
harmlessly — subdivision still returns the correct (smaller) set of extra requests, it
just does more work than strictly necessary. No false negatives are possible from this
threshold; false positives cost extra requests, not correctness.

### 2. Facet-based subdivision

Workday's listing response carries a `facets` array alongside `jobPostings`, e.g.:

```json
{"facetParameter": "jobFamilyGroup", "values": [{"id": "...", "count": 24224}, ...]}
```

Re-querying with `appliedFacets: {"jobFamilyGroup": ["<id>"]}` scopes both the
postings and the *other* facet dimensions in the response to that slice (verified
live: applying `jobFamilyGroup=Security` returns `total: 817` matching the facet's
own count). The dimension used to filter does **not** rescope against itself — Workday
returns the same global breakdown for it — so going deeper on an oversized slice
means adding a **different**, not-yet-applied facet dimension (e.g. `workerSubType`,
`timeType`, `locationMainGroup`), verified live to properly rescope under an applied
`jobFamilyGroup` filter.

Algorithm (`listPostings` becomes recursive):

```
fetchSlice(appliedFacets, usedDimensions, depth):
    page 1 of the slice (offset=0)
    if total < workdayCapTotal or depth == maxFacetDepth:
        page normally (existing loop) and return postings
    pick the facet dimension present in this response, not in usedDimensions,
      with the highest single-value count (best split for an uneven slice)
    if no such dimension exists:
        log.Printf warning: board capped, no further dimension to split on
        page normally (existing loop) and return postings  // best effort
    for each value in that dimension's facet:
        recurse: fetchSlice(appliedFacets + {dimension: [value.id]}, usedDimensions + dimension, depth+1)
    merge all recursed results
```

`maxFacetDepth = 3` — bounded, not tuned per company. Confirmed sufficient for the
worst known case: Accenture's top-level split still leaves "Software Engineering" at
~24k, but a second dimension resolves it (verified live). Three combined dimensions
gives headroom beyond that without unbounded recursion.

### 3. Dedup across slices

`workerSubType` (and potentially others) multi-counts — a posting can carry more than
one value, so its facet sum exceeds the true total and the same posting can appear in
more than one recursed slice. Merge results into a map keyed by `ExternalPath` before
returning from `listPostings`, same identity key the pipeline already dedups on
(`jobs.UNIQUE (source, external_id)`). This makes over-inclusive splits safe by
construction — extra requests, never extra rows.

### 4. Boards below the cap: unaffected

`total < workdayCapTotal` on page 1 skips the whole mechanism and pages exactly as
today. Zero behavior change for the ~6,350 boards that were never capped.

### 5. 403 retry

Mirror `eightfold.go`'s `getJSONRetrying`/`isRateLimited` shape for Workday's
`PostJSON`/`GetJSON` calls: retry on 403 or 429 with capped exponential backoff (a
small, fixed retry count — match Eightfold's constants unless Workday's own throttling
proves different during testing). Kept as a Workday-local helper, not a shared-client
change — 403 semantics are provider-specific (Eightfold: rate-limit; some Workday
tenants: CSRF/auth, per the existing `ats-scrapers` note) and freehire's convention is
already to layer this per-provider over the shared client.

### 6. Observability

`log.Printf("workday: board %s capped at %d, splitting by %s", ...)` when
subdivision triggers, and a distinct warning when depth is exhausted without dropping
under the cap — matching the existing `log.Printf` convention in this package
(`config.go`, `eightfold.go`, `djinni.go`, etc.). No new logging infrastructure.

### 7. Cost

Requests scale with true board size for the ~110 affected boards only (e.g. Accenture
goes from ~100 listing requests to on the order of 50,000/20 ≈ 2,500 across all
slices). This runs under the crawl's existing concurrency and retry handling; no new
rate-limiting mechanism is being introduced.

## Testing

Extend `internal/sources/workday_test.go` using the existing `routedHTTP` /
`pagedWorkday` mock patterns:

- Capped total (`total: 2000` on page 1, with a `facets` array) triggers a
  facet-scoped re-query instead of paging past 2000.
- A slice that is itself still capped recurses into a second, different facet
  dimension (not the one already applied).
- Overlapping postings returned by two different facet values are deduped by
  `ExternalPath` in the final result.
- Depth exhausted (`maxFacetDepth` reached) without resolving under the cap: logs a
  warning and returns what was collected, rather than erroring or looping.
- A board below the cap (`total < 2000`) is unaffected — same request count as today.
- 403 on a listing or detail request retries and eventually succeeds; a non-retryable
  error (e.g. 404) returns immediately.

## Open questions resolved during brainstorming

- Scope: fix both the 2,000 cap and the 403-fatal gap together (user confirmed).
- Approach: generic adaptive facet-splitting, not a hardcoded per-company list (user
  confirmed) — it must work for the ~110 affected boards, not just Accenture.
