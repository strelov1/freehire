## Why

The filter modal shows a job count only on the role picker and the dynamic
selects, and those counts reflect the **already-applied** filters, not what the
user is picking right now. So while building a filter you can't see how many jobs
each option would add, and the seniority/specialization/other controls show no
counts at all. Users expect the Amazon/Booking experience: every option shows a
live count that updates as you pick, with the count staying meaningful for the
facet you're editing.

## What Changes

- **Counts on every facet control** in the modal — the Seniority pills, the
  Specialization chips, and every other `ChipFacet`/location control show their
  live match count, like the role picker already does.
- **Live recompute from the staged (in-progress) selection** — as the user
  toggles options, the counts recompute (debounced) from the staged filter, not
  the applied one. The **deferred-apply contract is unchanged**: nothing reaches
  the live list until Apply.
- **Disjunctive faceting** — each facet's own selection is excluded from its own
  distribution, so siblings stay visible (selecting "Senior" still shows
  "Junior 54k"). This needs a backend capability: for each requested facet, count
  under the full filter **minus that facet's own selection**, plus a grand total
  under the full filter. Implemented as concurrent Meilisearch facet queries in
  `internal/search` and exposed via `GET /api/v1/jobs/facets` behind a flag.
- **Loading state** — while a recompute is in flight, the modal's "Show N
  results" button shows a spinner (and the counts dim) instead of a stale number;
  the number returns when the fetch resolves.

## Capabilities

### Modified Capabilities
- `job-analytics`: the facet-distribution endpoint gains an opt-in **disjunctive**
  mode — each facet counted under the filter minus its own selection, plus the
  full-filter total — backed by a new concurrent `search` capability.
- `filter-modal`: all facet controls show live counts sourced from the staged
  selection (disjunctive), and the footer preview shows a loading state while a
  recompute is in flight.

## Impact

- **Backend**: `internal/search` (new `DisjunctiveFacetCounts` — concurrent
  per-facet queries + total), `internal/handler/facets.go` (build per-facet
  reduced filters, wire the disjunctive path + `facetCounter` interface).
- **Frontend**: `FilterModalShell` (debounced staged facet-counts fetch + loading
  state, spinner on the Show button), `ChipFacet`/`CategoryPane`/`PillGroup`
  (count props), `FilterModal` (pass staged counts to every pane), `api.ts`
  (`disjunctive` param), `JobsView` (provide the staged-counts fetcher).
- No index/schema change; no new dependency (`golang.org/x/sync` already vendored).
