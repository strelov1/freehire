## Context

Filters today live in `web/src/lib/components/FiltersPanel.svelte`: a single sidebar
that iterates the `FACETS` registry (`web/src/lib/facets.ts`) and renders each facet
via `FacetSection` over a `FilterStore` (`web/src/lib/filters.ts`), which wraps
`UrlSyncedState` and applies every change live to the URL. Facet options and counts
come from `GET /api/v1/jobs/facets`. Geography is served as three independent flat
facets (`regions`, `countries`, `cities`) — the backend stores them as unassociated
arrays and has no city→country link. Region grouping (`country→region`) exists in
`internal/location` but is not exported to the client. `cmd/gen-contracts` exports
value arrays to `web/src/lib/generated/contracts.ts` but has no map export.

This change adds a two-pane filter **modal** as the primary editing surface, turns the
sidebar into a summary of what's selected, and introduces hierarchy (grouped
specialization, region→country→city geography) and a merged salary facet — all in the
existing neutral freehire design language (chips, black primary, `rounded-xl` cards).
A throwaway prototype validated the layout and interactions.

## Goals / Non-Goals

**Goals:**
- A discoverable, sectioned filter modal that scales past ~18 flat facets.
- Deferred apply with a live result-count preview; the sidebar reflects selection.
- Hierarchy where it pays off: grouped specialization and a region→country→city tree.
- Reuse the existing pill controls, facet registry, and `/jobs/facets` counts.
- No API, DB schema, or search-index change.

**Non-Goals:**
- Skills grouped by themes (needs a new dictionary — separate change).
- A global "search across all filters" box (deferred).
- Scoped facet counts (countries-within-region, cities-within-country) — region-level
  counts only.
- Changing the search/filter query semantics or params.

## Decisions

### Staged vs applied state — a staging store layered over FilterStore
The modal edits a **staged** snapshot; the sidebar and URL stay the live truth. Model
this as a thin staging store that clones the current `JobFilters` on open, exposes the
same facet mutators as `FilterStore`, and on **Show results** writes the snapshot back
through `FilterStore.apply()` (the existing "replace whole filter state" path used by
saved searches). Chip removal in the sidebar calls the live `FilterStore` directly.
*Alternative rejected:* adding a "pending" mode to `FilterStore` — it would blur the
live-URL invariant that the rest of the app relies on.

### Sidebar becomes a summary component
`FiltersPanel` is reshaped: instead of rendering facet controls it renders the applied
values as grouped chips plus the **All filters** button and **Reset all**. The
per-facet control rendering (`FacetSection`, pill/select/token controls) moves into the
modal's right pane. `activeFilterCount` already exists for the badge.

### Facet registry gains grouping + hierarchy metadata
Extend `FacetDef` with a `section` (rail grouping) and, where needed, a hierarchy
descriptor. Specialization gets a static `category → section` map covering the whole
`CATEGORY_VALUES` vocabulary (a test asserts exhaustiveness so a newly added category
can't silently fall out of every section). Location becomes a dedicated hierarchical
control rather than a flat select.

### Geography hierarchy is built client-side from two exported maps
Add `emitMap` to `cmd/gen-contracts` and export `country→region` (inverse of
`internal/location`'s region grouping) and a new `city→country` map. The city→country
map is curated in `internal/location` over the **beacon-city** set the parser can emit
(`nameToCity` / `nameToCountry`), so coverage matches the actual `cities` facet values;
a test asserts every beacon city resolves to a country. The client composes
region→country→city from these maps + the live `regions`/`countries`/`cities` facet
distributions. Selection still uses the existing three params, so the backend filter is
unchanged. *Alternative rejected:* a backend hierarchical/scoped facet endpoint — larger
surface, not needed when the client already has the full mappings.

### Live count preview reuses the facets endpoint
**Show results** shows the total for the staged filters via `api.facetCounts(staged
params).total`, debounced per toggle. No new endpoint.

## Risks / Trade-offs

- **Incomplete city→country coverage** → a city with no beacon mapping won't nest under
  a country. *Mitigation:* the spec makes non-nesting explicit; unmapped-but-present
  cities are surfaced in a flat fallback list within the Location pane so they stay
  selectable.
- **Count query volume** (a count per staged toggle) → *Mitigation:* debounce the
  staged-count request; it's the same cheap facet call the panel already makes.
- **Skills tail beyond Meili's `maxValuesPerFacet` (300)** → the facet-local search only
  filters the returned distribution. *Mitigation:* acceptable for phase 1 (matches
  today's skills select); a server-backed tail search is a later enhancement.
- **Mobile two-pane** → *Mitigation:* full-screen modal with the rail collapsing to a
  facet selector; validated as a requirement, not an afterthought.
- **Sidebar reshape touches a load-bearing component** → *Mitigation:* the modal reuses
  the extracted `FacetSection`, so control behavior is unchanged; only the container
  moves.

## Resolved During Implementation

- **City nesting dropped.** The `cities` facet turned out to be an LLM-open vocabulary
  (jobview backfills city names far beyond the location dictionary), so the beacon-only
  `city→country` map couldn't nest most cities and produced a confusing "some nested,
  the rest in Other cities" split. Resolved to a **region→country tree + one flat,
  searchable Cities section** (busiest-first + type-to-search). The `city→country`
  map and its contract export were **removed** (YAGNI — no consumer); a future
  gazetteer can reintroduce them.
- **Work format + Employment merged** into one "Work & employment" rail entry (one
  pane, two chip groups), mirroring the salary/currency merge.

## Migration Plan

Frontend-only deploy plus a `gen-contracts` regeneration at build time (the two new
maps). No DB migration, no search reindex, no API change. Rollback is a straight revert
of the frontend + generated contracts.

## Open Questions

- Final section assignment for a few cross-cutting categories (e.g. `architecture`,
  `blockchain`, `embedded`) — settle during implementation against the exhaustiveness
  test.
- Whether unmapped cities get a flat "Other cities" fallback in the Location pane or are
  reachable only via the facet-local search — decide when wiring the tree.
