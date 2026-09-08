## Why

`internal/search/search/AGENTS.md`'s own "Limitations" section names this as a known gap: "A Meili filter error 500s the page instead of degrading. That's the robustness seam." This contradicts the project's own stated convention (CLAUDE.md: "A dropped filter says so" — an endpoint whose answer widens when it does not understand a query param reports the params it did not read via `meta.ignored_params`, rather than failing the request). The realistic trigger is not a malicious client — every facet value is already escaped before reaching Meilisearch — it is the deploy-ordering race the same AGENTS.md file documents extensively: "Adding a filterable attribute creates a hard-500 window... ~26 min at catalogue scale." During that window every search touching the new facet fails outright, even though the rest of the catalogue is perfectly queryable.

## What Changes

- `runJobSearch` (`internal/api/handler/search.go`, shared by `SearchJobs` and `AgentSearchJobs`): when the primary Meilisearch call fails with `search.ErrBadQuery` **and** the request actually carried a dynamic facet filter, retry once with the filter dropped entirely (query text, sort, vector, and pagination unchanged). On retry success, every currently-active filter/facet query param is reported in `meta.ignored_params`, merged with whatever this endpoint already reports for genuinely-unrecognized params.
- `internal/search/search/query_params.go` gains `ActiveFilterParams(v url.Values) []UnknownParam` — the inverse of the existing `UnknownParams`: which of the request's params ARE part of the known filter vocabulary, reusing the existing (unexported) `knownParams` list rather than duplicating it.
- On retry failure, or when there was no filter to blame, or the original error is not `ErrBadQuery` (a genuine engine-down failure) — behavior is unchanged from today. This only changes the outcome for a filter-classified rejection that has something to drop.

**Explicitly not in scope:**
- The analogous sort-attribute hazard (`AGENTS.md`'s "Adding a sortable attribute", same root cause, same deploy window). Dropping a bad sort falls back to default ordering, not to "no filter" — a differently-shaped fix, left as a parallel, still-open gap, not silently folded into this change.
- Identifying which SPECIFIC facet param caused the rejection by parsing Meilisearch's error message. Its error is not reliably structured for that; the degrade reports every currently-active filter param honestly rather than guessing at one.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `job-search`: gains an ADDED requirement (the existing "Public job search endpoint" requirement's own text and filtering behavior are unchanged) describing the degrade-on-filter-rejection behavior — the same endpoint's own failure-mode contract, not a new capability.

## Impact

- `internal/search/search/query_params.go`: new `ActiveFilterParams`.
- `internal/api/handler/search.go`: `runJobSearch`'s signature gains a return value for the dropped-param report; its body gains the retry.
- `internal/api/handler/agent_search.go`: `AgentSearchJobs` updated for the new `runJobSearch` signature, merging the report the same way `SearchJobs` does.
- `internal/search/search/AGENTS.md`: the "Limitations" line is stale after this — update it to describe the degrade instead of the bare gap, and correct the "500" wording given `errors.go` already maps `ErrBadQuery` to 400 (a pre-existing documentation inaccuracy this change's own research surfaced, not something it introduces).
- No migration, no new query, no change to `fillProviders`/index settings/deploy process.
