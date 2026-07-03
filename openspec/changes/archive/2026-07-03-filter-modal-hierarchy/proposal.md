## Why

The job-search filters live in an ever-growing single sidebar (~18 facets stacked
vertically). It scales badly: facets are hard to scan, related facets aren't
grouped, and high-value dimensions (specialization, geography) are flat lists with
no hierarchy. A hiring.cafe-style **filter modal** — a two-pane browser that groups
facets into sections and gives each facet room for hierarchy and search — makes the
full filter surface discoverable, while the sidebar shrinks to a clean summary of
what's actually selected.

## What Changes

- **New filter modal** ("All filters"): a two-pane overlay — a left rail of facet
  categories grouped into sections (`ROLE`, `PAY & BENEFITS`, `REQUIREMENTS &
  ELIGIBILITY`), and a right pane rendering the selected facet's controls.
- **Deferred apply**: selections are staged inside the modal and applied only on
  **Show N jobs** (a primary button carrying a live result-count preview). Applying
  copies staged → the live filter state (URL). The modal opens seeded from the
  current applied filters.
- **Sidebar becomes a summary of selected filters**: chips grouped by facet, plus
  an **All filters** button (with an active-count badge) and **Reset all**. Removing
  a chip applies immediately (the sidebar edits live state directly; the modal is the
  only deferred surface).
- **All option controls are chips/pills** (reusing the existing pill primitive) — no
  checkboxes or radios.
- **Specialization grouped into collapsible sections** (Engineering, Data & AI,
  Quality & Security, Design, Product & Management, Go-to-market & Support) via a
  static category→section map over the existing `CATEGORY_VALUES`.
- **Location becomes a region → country → city chip tree** with drill-down, replacing
  the three independent region/country/city facets in the modal. Requires exporting
  a country→region map and a new city→country map from the backend location
  dictionary to the frontend contracts. Region-level result counts only (per-country
  / per-city scoped counts are out of scope).
- **Salary and Currency merge into one "Salary" pane** (currency pills + minimum-salary
  slider together).
- **Per-facet search inside high-cardinality facets** (Skills): a search box that
  filters the facet's options, with selected values pinned above.
- **Responsive**: on small screens the modal is full-screen; the rail and pane stack.

## Capabilities

### New Capabilities
- `filter-modal`: the two-pane hierarchical filter modal, its deferred-apply
  (staged vs applied) model, the sidebar-as-selected-summary, chip-based controls,
  grouped specialization, the location tree, the merged salary pane, and per-facet
  search.

### Modified Capabilities
- `job-geography`: the location dictionary gains a canonical **city → country**
  association over its beacon-city set, and the existing **country → region** grouping
  plus the new city→country map are exported to the frontend contracts so the client
  can render the region→country→city hierarchy. (Derivation stays dictionary-only and
  never guesses; unmapped cities simply don't nest.)

## Impact

- **Frontend (`web/`)**: new modal components; `FiltersPanel` reshaped into a
  selected-summary; a staged-filter store layered over the existing
  `FilterStore`/`UrlSyncedState`; facet registry gains grouping/hierarchy metadata;
  reuses the existing pill controls and `/api/v1/jobs/facets` counts.
- **Backend (`internal/location`, `cmd/gen-contracts`)**: a city→country map added to
  the location dictionary; `gen-contracts` gains map export (`emitMap`) and emits
  `country→region` + `city→country`. No API, schema, or search-index change.
- **Out of scope (follow-ups)**: skills grouped by themes; a global "search across all
  filters" box; scoped facet counts (countries-within-region, cities-within-country).
