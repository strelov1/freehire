## 1. Backend — disjunctive facet counts (`internal/search`)

- [x] 1.1 Add `search.DisjunctiveFacetCounts(ctx, query, reqs []FacetReq,
  totalFilter any) (FacetResult, error)` where `FacetReq{Attr, Filter}` — runs one
  `Limit:0` facet query per req (facets=[attr]) plus one total-only query, all
  concurrent (`errgroup`), and merges each facet's own distribution + the grand
  total. Table/behaviour tests for the merge (fake responses) and that a query
  error propagates.

## 2. Backend — endpoint wiring (`internal/handler/facets.go`)

- [x] 2.1 Add `DisjunctiveFacetCounts` to the `facetCounter` interface (update the
  fake in `facets_test.go`).
- [x] 2.2 In `JobFacets`, when `disjunctive` is set, build per-facet reduced
  filters (clone values, delete `param`/`param_exclude`/`param_mode`,
  `FilterFromValues`) for each distribution facet, plus the full filter for the
  total, and call `DisjunctiveFacetCounts`; re-key to public params as today.
  Non-disjunctive requests keep the existing `FacetCounts` path.
- [x] 2.3 Handler test: `disjunctive=1` passes per-facet reduced filters (the
  seniority query's filter omits `seniority=`) and the total uses the full filter.

## 3. Frontend — API

- [x] 3.1 `api.facetCounts(params, { disjunctive })` — forward the flag as a query
  param; return the full `FacetCounts` (already does).

## 4. Frontend — live staged counts + loading (`FilterModalShell`)

- [x] 4.1 Replace the `previewCount` total-only fetch with a `stagedCounts(params)
  => Promise<FacetCounts>` (disjunctive) supplied by the wrapper; keep a
  `stagedCounts` + `loading` `$state`, gen-guarded and debounced on
  `staged.params()`.
- [x] 4.2 Show button: spinner while `loading`, else `Show {stagedCounts.total}`.
- [x] 4.3 Expose the staged distribution to the `pane` snippet (so panes read it
  instead of the applied `counts`).

## 5. Frontend — counts on every control (`FilterModal` + panes)

- [x] 5.1 Pass the staged distribution to **every** `ChipFacet` (work_mode,
  employment_type, domains, company_type, collections, relocation,
  salary_currency, english_level) and to `LocationPane`, in addition to the
  role/seniority/specialization pane.
- [x] 5.2 Confirm `PillGroup`/`ChipFacet`/`CategoryPane` render `opt.count` (from
  #487); extend `LocationPane` region/country/city rows with counts.

## 6. Frontend — JobsView wiring

- [x] 6.1 Provide `stagedCounts` fetcher to the modal (disjunctive facetCounts over
  staged+scope params); keep the applied `counts` for the sidebar.

## 7. Verify

- [x] 7.1 `go build ./... && go vet ./... && go test ./...`; web `svelte-check` +
  vitest green.
- [x] 7.2 Visual: modal shows counts on all controls; toggling an option keeps the
  edited facet's siblings visible and recomputes others; Show button spins then
  shows the number.
