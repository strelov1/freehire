## Context

`HeaderLocationFilter` (shipped in `header-location-workmode-filter`) renders a
trigger + popover driven by a page's `FacetStore`, gated behind the optional
`ListSearchTarget.filterScope`. It is currently jobs-only. Companies already carry
`regions`/`remote_regions` facets on `CompanyFilterStore` (which also satisfies
`FacetStore`), and listless pages render the `HeaderSearch` launcher with no store.

## Goals / Non-Goals

**Goals:** one menu, three context modes (jobs / companies / launcher); reuse the
existing stores and facet counts; no text-search, backend, or extra-request change.

**Non-Goals:** an ephemeral store or an "Apply" step for the launcher; company work
format / city facets (companies don't have them); the full location accordion on the
launcher (it appears live once you land on `/jobs`).

## Decisions

### One component, a `variant` discriminator
`HeaderLocationFilter` gains `variant: 'jobs' | 'companies' | 'launcher'`. The shell
(trigger, open/close, summary label) is shared; the body switches:
- **jobs** — unchanged: work-format pills + `<LocationPane>`, drives the store.
- **companies** — Region pills + Remote-hiring pills (both `REGION` options; drive
  `store.cycle('regions'|'remote_regions', code)`), counts from the company facet
  distribution. No work format / LocationPane.
- **launcher** — work-format pills + Region pills; **no store**. Each pick calls
  `goto('/jobs?<param>=<value>')` and lands on the live feed.

*Alternative for launcher:* an ephemeral non-URL store + Apply button. Rejected as
over-built — "pick jumps to the feed, refine live there" needs zero new state.

### Bridge carries the variant
`ListSearchTarget.filterScope` gains `variant: 'jobs' | 'companies'`. `JobsView`
registers `'jobs'` (unchanged shape otherwise); `CompaniesView` registers
`'companies'` with its store + company facet counts. The launcher is **not** a
bridge target — `HeaderSearch` renders the trigger directly in `'launcher'` mode.

### `summarizeScope` generalized over a facet spec
`summarizeScope(store, spec?)` takes a `{ format?: string; geo: string[] }` spec
(default = the jobs spec, so existing callers/tests are unchanged). Companies pass
`{ geo: ['regions', 'remote_regions'] }`. A per-param labeler maps `countries`→
`countryLabel`, `cities`→raw, everything else (`regions`/`remote_regions`)→
`REGION_LABELS`. Launcher uses a static neutral label (no persisted selection).

## Risks / Trade-offs

- **Launcher single-pick navigation loses multi-select before the jump** → mitigated
  by live refinement on `/jobs` after landing; the first pick is the entry point.
- **Company facet counts shape** → `CompaniesView` already fetches `FacetCounts`;
  Region/Remote-hiring pills read `counts.facets.regions`/`.remote_regions`, degrade
  to countless pills if absent (same null-safety as jobs mode).

## Migration Plan

Frontend-only, no schema/API change, no migration. Ships with a normal web deploy.
