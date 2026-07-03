## Why

Today a facet is either all-include or all-exclude: the filter model carries a
single `exclude` flag for the whole facet, so a user cannot say "include nodejs
**and** exclude php/.net" in one facet. The backend already accepts both
`?skills=nodejs` and `?skills_exclude=php` in the same request — the limitation is
purely in the web client's filter model and controls.

## What Changes

- **BREAKING (internal model):** Replace the per-facet `FacetState { values,
  exclude, matchAll }` with `FacetState { include[], exclude[], matchAll }`, so a
  single facet can hold included and excluded values at the same time. `matchAll`
  applies to the include set only (exclude values are always ANDed).
- Filter store gains per-value sign operations (`setSign`, `cycle`, `pick`,
  `toggleSign`) and drops the whole-facet `setExclude` toggle.
- **Select facets** (skills, countries, cities, domains, category, company,
  source, language): the dropdown pick adds to include; selected values render as
  chips, each with an include↔exclude toggle; excluded values group below in the
  destructive (red) style.
- **Pills facets** (regions, work format, seniority, employment, …): each pill is
  three-state — off → include → exclude → off — replacing the section-wide Exclude
  toggle.
- The `FilterSummary` and staged-filter surfaces render include and exclude
  values distinctly.
- URL serialization is unchanged in shape (`?param` / `?param_exclude` /
  `?param_mode=and`), so existing shared links and saved searches keep working.
- Add **vitest** to the web project to unit-test the pure filter logic (model
  transitions + URL round-trip); Svelte controls are verified visually.

## Capabilities

### New Capabilities

<!-- none: this modifies existing filter behavior -->

### Modified Capabilities

- `filter-modal`: the "per-facet Exclude and Clear" requirement changes from a
  whole-facet Exclude toggle to per-value include/exclude — three-state pills and a
  per-chip include↔exclude toggle — so include and exclude values coexist within one
  facet.

## Impact

- **Web only.** No backend, Meilisearch, or search-API change (the API already
  accepts simultaneous `param` / `param_exclude`; `query_filter.go` and its tests
  are untouched).
- Touched web files: `filters.ts` (model, store, serialization),
  `stagedFilters.svelte.ts`, `facets.ts` (`FacetSelection`/`FacetStore`
  interfaces), the facet controls (`PillGroup`, `SearchSelect`,
  `RemoteSearchSelect`, `FacetSection`, `TokenInput`), and the modal surfaces
  (`FacetHeader`, `ChipFacet`, `FilterSummary`).
- New dev dependency: `vitest` + a `test` script in `web/package.json`.
- Old URLs / saved searches remain valid (parse path is backward-compatible).
