## Context

The modal is deferred-apply: panes edit a `staged` store; nothing hits the live
list until Apply. Facet option counts today come from `JobsView.counts` — the
`/jobs/facets` distribution of the **applied** filters — passed down as a static
`counts` prop. `FilterModalShell` already fetches a **staged** total via a
debounced `previewCount(stagedParams)` for the "Show N" button. A prior change
(#487) gave `ChipFacet`/`CategoryPane`/`PillGroup` optional count props, so the
rendering side is ready; what's missing is (a) counts on every control, (b) the
counts following the staged selection disjunctively, and (c) a loading state.

Meilisearch computes a facet distribution **after** the query filter
(conjunctive), so a single call can't produce disjunctive counts — selecting a
value zeroes its siblings. Disjunctive faceting needs one query per facet, each
excluding that facet's own filter.

## Goals / Non-Goals

**Goals:** live, disjunctive counts on every modal facet control; a loading state
on recompute; keep deferred-apply; no new dependency; endpoint latency ≈ one facet
query (concurrent).

**Non-Goals:** changing the applied-list behaviour; per-facet caching; disjunctive
counts outside the modal (the sidebar/applied path keeps conjunctive counts).

## Decisions

**Disjunctive via concurrent per-facet queries (`search.DisjunctiveFacetCounts`).**
Given the query, a total filter, and a list of `{attr, filter}` (each filter is
the full filter minus that facet's own selection), run all queries + one
total-only query **concurrently** (`errgroup`, already vendored) against the facet
index with `Limit:0`. Merge each facet's own distribution and the grand total.
Wall time ≈ the slowest single query, not the sum. ~18 cheap `Limit:0` searches
per debounced recompute is acceptable (and covered by the loading state).

**Filter reduction stays in the handler.** The handler owns `url.Values` and
`search.StringFacets` (param→attr). For each facet param it clones the values,
deletes `param`, `param_exclude`, `param_mode`, and rebuilds via
`search.FilterFromValues` — the reduced filter for that facet. The total uses the
full values. This keeps all filter-string construction in one place; `search`
only runs the queries. The disjunctive path is gated on a `disjunctive` query flag
so the existing conjunctive endpoint (sidebar, ATS) is untouched.
`facetCounter` gains a `DisjunctiveFacetCounts` method (fake updated for tests).

**Frontend: the shell owns staged counts + loading.** `FilterModalShell` already
debounces on `staged.params()`. Generalize its `previewCount` into a
`stagedCounts(params) => Promise<FacetCounts>` (disjunctive) supplied by
`JobsView`; the shell keeps a `stagedCounts` + `loading` `$state`, and passes the
distribution down to the panes (replacing the applied `counts` prop within the
modal) and the total+loading to the Show button. `JobsView` still computes the
applied `counts` for the sidebar. The pane wiring passes counts to **every**
`ChipFacet` and the `LocationPane`.

**Loading state.** The Show button renders a spinner while `loading`; option
counts read from the last resolved `stagedCounts` (so they don't flicker to empty
mid-fetch) and may dim. A monotonic gen id drops stale responses (as `previewCount`
already does).

## Risks / Trade-offs

- **Meili query volume**: ~18 facet queries per debounced recompute. Cheap
  (`Limit:0`), concurrent, modal-only, debounced — acceptable; the loading state
  hides the latency. If it ever bites, batch via Meili multi-search.
- **Debounce feel**: 200ms debounce means counts trail the click slightly; the
  spinner makes that legible rather than janky.
- **Disjunctive is modal-only**: the applied/sidebar counts stay conjunctive
  (they reflect the applied filter, where conjunctive is correct). No divergence
  risk since they answer different questions.
