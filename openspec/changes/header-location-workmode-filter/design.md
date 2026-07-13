## Context

The header search box is context-adaptive (see `header-navigation`): on list pages
(`/`, `/companies`, `/companies/:slug`) it renders `HeaderListSearch`, a thin proxy
that drives the active page's filter store through the `ListSearchTarget` bridge
(`web/src/lib/listSearch.svelte.ts`). Jobs-backed lists (`/`, `/companies/:slug`)
register a `FilterStore`; the company list registers a `CompanyFilterStore`.

Geography and work format are already modelled as facets (`regions`, `countries`,
`cities`, `work_mode`) on that store, exposed today only via the full `FilterModal`
and its `LocationPane` (`web/src/lib/components/filters/LocationPane.svelte`).
`FilterStore` satisfies the `FacetStore` interface (`facet`/`cycle`/`remove`/
`clearFacet`) that `LocationPane` consumes, and `JobsView` already fetches facet
counts for the modal. This change surfaces those facets from the search box by
reusing the existing pane, store, and counts — adding UI, not filter behavior.

## Goals / Non-Goals

**Goals:**
- One-click access to work-format and full location filtering from the header on
  jobs-backed lists.
- Zero new filter logic, backend, API, or facet-count request — reuse the store,
  `LocationPane`, and the view's existing counts.
- Gate visibility to jobs-backed lists without coupling `TopBar` to the feature.

**Non-Goals:**
- The global search launcher (`HeaderSearch`) and the company list (`/companies`).
- A mobile bottom-sheet treatment (MVP is an anchored, scrollable popover).
- Any other `FilterModal` facet (employment type, salary, seniority, etc.).

## Decisions

### Capability-gated bridge, not a `TopBar`/route check
Extend `ListSearchTarget` with an **optional** `filterScope?: { store: FacetStore;
counts(): FacetCounts | null }`. `JobsView` registers an adapter object that bundles
its `filters` store and a reactive `counts()` getter; `CompaniesView` keeps
registering the bare `CompanyFilterStore`. `HeaderListSearch` renders the trigger
**iff** `target.filterScope` is set. The popover therefore appears exactly on
jobs-backed lists and nowhere else, with no `listKind`/pathname branching in the
header.

*Alternative considered:* gate on `listKind === 'jobs'` in `TopBar` and pass a prop.
Rejected — it couples the header shell to this feature and to route shape, and it
would need a second path to obtain counts.

### Reuse `JobsView` counts via a reactive getter
The counts live in `JobsView` as `$state.raw<FacetCounts | null>`, already fetched
for the modal. Passing `counts: () => counts` in the adapter (read through the
getter inside the header template) keeps Svelte 5 reactivity flowing and avoids a
duplicate `api.facetCounts` request. `LocationPane` already degrades gracefully when
counts are null (stable macro-region list; country/city sections thin out), so a
pre-fetch race is harmless.

*Alternative considered:* fetch counts inside the new component. Rejected — a second
request and a second source of truth that can drift from the modal's counts.

### New self-contained component + pure summary helper
`HeaderLocationFilter.svelte` owns the trigger, the popover chrome (title + "Clear
all"), the Work format pill row (reusing `../facets/pill` `pillClass`/`pillTitle`
over `WORK_MODE_OPTIONS` + `store.cycle('work_mode', …)`), and an embedded
`<LocationPane>`. The trigger's label is derived by a **pure** `summarizeScope(store)
→ { icon, label }` helper in `web/src/lib/headerScope.ts`, unit-tested with vitest so
the label logic (none / format-only / geo-only+N / both) is verified without a DOM.

*Alternative considered:* a separate `WorkModePane` file. Rejected as YAGNI — the
pill row is a few lines and only used here.

## Risks / Trade-offs

- **Popover height on small screens** → MVP caps at `max-h-[70vh]` with scroll; a
  bottom-sheet is a noted later refinement, not a blocker.
- **Adapter object identity vs. store instance** → the header reads `target.value.q`
  / `target.setQuery` through the adapter's delegating getters, preserving existing
  proxy behavior; verified by the unchanged text-search path.
- **Counts reactivity across the bridge** → mitigated by reading `counts` through a
  function inside a reactive context; if it ever fails to track, the pane still
  renders (null-safe), so the failure mode is "no country counts," not a crash.

## Migration Plan

Frontend-only, no schema or API change. Ships with a normal `web/` build/deploy;
no migration, no manual prod step. Rollback is reverting the branch.
