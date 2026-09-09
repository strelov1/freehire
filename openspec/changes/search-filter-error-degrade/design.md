## Context

`runJobSearch` (`internal/api/handler/search.go`) is the one place both `SearchJobs` and `AgentSearchJobs` (`agent_search.go`) build and run the Meilisearch query. Today, any error from `h.search.Search(...)` is returned as-is:

```go
res, err := h.search.Search(c.Context(), search.SearchParams{...})
if err != nil {
    return search.SearchResult{}, 0, 0, err
}
```

`search.Client.Search` (`internal/search/search/client.go`) already routes every query failure through `queryErr`, which wraps a Meilisearch 400-status response as `search.ErrBadQuery` — the sentinel `internal/api/handler/errors.go` already maps to a 400 "invalid search query" response. So the failure path already exists and is already classified; what's missing is a caller that, given that specific classification, tries the ordinary "widen and report" degrade instead of surfacing it as a failure.

`buildSearchFilter(c)` (`search.go`) is the only source of the dynamic filter; a request with none of the facet/scalar params set already builds a nil filter, so there is nothing a Meilisearch filter-rejection could be blaming in that case.

`internal/search/search/query_params.go` already has the exact inverse shape I need on the "unrecognized param" side (`UnknownParams`/`knownParams`), and `internal/api/handler/market_coverage.go` already establishes the merge idiom for folding a caller's own extra `UnknownParam` entries into the standard report: `search.SortAndCap(append(extra, search.UnknownParams(vals, nil)...))`.

## Goals / Non-Goals

**Goals:**
- A filter-classified Meilisearch rejection degrades to a widened, successful result with an honest `meta.ignored_params` report, the same convention every unrecognized param already gets.
- No new failure mode, no new latency in the common (successful) path — the retry only ever happens after the primary call has already failed.
- Reuse `search.ErrBadQuery`/`SortAndCap`/`UnknownParams` as they exist; no parallel classification or reporting mechanism.

**Non-Goals:**
- The sort-attribute hazard (`AGENTS.md`'s "Adding a sortable attribute"). Left as a separate, still-open, differently-shaped gap — see proposal.md.
- Parsing Meilisearch's error message to name the one offending param. Reports every active filter param instead.
- Changing `search.Client.Search`'s own signature or `queryErr`'s classification. The retry is a caller-side concern (a second ordinary call), not a new capability on the client.

## Decisions

**The retry is a second, ordinary `Search` call from the handler, not a client-level retry.** `search.Client.Search` stays a single query, one classification. Retrying with a different `SearchParams` (filter removed) is a policy decision about what to try next — the handler's job, matching how `runJobSearch` already owns query construction end to end.

**Gate the retry on two conditions, both checked before attempting it:** `errors.Is(err, search.ErrBadQuery)` AND the original `SearchParams.Filter` was non-nil. Without the second check, a filter-less request that somehow gets `ErrBadQuery` (a bad sort, a malformed vector — a different hazard) would retry with an unchanged, identical query and fail again identically — wasted latency on an already-failing request, and worse, it would MISREPORT the failure: `ActiveFilterParams` would build an empty list from a request that had no filter params to report, so a caller would see the confusing shape of "retried but reported nothing dropped." Checking `Filter != nil` first avoids ever entering that retry at all when there is nothing to blame.

**`ActiveFilterParams` reuses `knownParams(nil)` (the exported `UnknownParams`'s own unexported helper), not a second copy of the facet/scalar-filter vocabulary.** The two functions are natural mirrors of the same one list — an unrecognized param is one NOT in `knownParams`; an active filter param is one that IS. Keeping them backed by the same map is what stops the two reports from drifting the way `UnknownParams`'s own doc comment already warns duplication would.

**`ActiveFilterParams` never populates `DidYouMean`.** These are, by construction, params already IN the known vocabulary — there is no typo to suggest a correction for.

**`runJobSearch` grows a return value (`dropped []search.UnknownParam`) rather than a second method or an out-parameter.** Both existing callers already destructure four return values positionally; adding one more is the smallest change that keeps the "one function builds and runs the query" shape intact. An empty/nil `dropped` is the ordinary case (no retry happened), so neither caller's happy path changes shape.

**Both callers merge `dropped` into their own `ignoredParams(...)` call using the exact `market_coverage.go` idiom** (`search.SortAndCap(append(dropped, ignoredParams(c, ownParams)...))`), not a new merge helper — this is a two-line change at each call site, and inventing a shared wrapper for two call sites duplicating an already-established three-line idiom is not a simplification this change needs to make.

## Risks / Trade-offs

- **[Risk]** A request whose filter Meilisearch rejects for a reason OTHER than the deploy-window race (a genuinely malformed value that somehow reached the engine unescaped) would now silently widen instead of failing loudly. → **Mitigation**: `search.quote()` already escapes every facet/scalar value before it reaches a filter expression (Context, above; also proposal.md), so this class of bug is already closed upstream. If it were ever reopened, the widened response is still bounded to what the endpoint would ALREADY serve unfiltered — no data exposure change, only a wider result set, exactly the same trade-off every unrecognized param already accepts today.
- **[Trade-off]** A failing search now costs two Meilisearch round trips instead of one, but only in the already-failing case (the deploy-window race, or the rare rejected-filter case above) — never in the common path. Accepted: bounded to a narrow, already-documented window, and strictly better than today's outright failure for every request in it.
