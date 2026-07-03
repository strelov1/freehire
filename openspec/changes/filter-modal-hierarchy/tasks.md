## 1. Backend: geography mappings + contract export

- [x] 1.1 Add a canonical `city → country` map in `internal/location` over the beacon-city set (`nameToCity` / `nameToCountry`), plus a test asserting every emittable beacon city resolves to exactly one ISO country code
- [x] 1.2 Expose the `country → region` grouping (inverse of `regionCountries`) as a package-accessible map with a test asserting every grouped country maps to exactly one controlled region value
- [x] 1.3 Add an `emitMap` helper to `cmd/gen-contracts` (emit a frozen TS `Record<string,string>` / `Record<string,readonly string[]>`) with a unit test on its output shape
- [x] 1.4 Wire `gen-contracts` to emit `COUNTRY_REGION_MAP` (country→region) and `CITY_COUNTRY_MAP` (city→country); regenerate `web/src/lib/generated/contracts.ts` and commit

## 2. Frontend: staging store + facet registry metadata

- [x] 2.1 Add a `StagedFilters` store (`stagedFilters.svelte.ts`): clone-on-open (`seed`) over `JobFilters`, same `FacetStore` mutators as `FilterStore`, `commit(live)` → `FilterStore.apply()`; type-checked
- [x] 2.2 Add the modal rail registry (`filterSections.ts`: `RAIL` entries + `RAIL_SECTIONS`) grouping facets under `ROLE` / `PAY & BENEFITS` / `REQUIREMENTS & ELIGIBILITY`, with composite `location`/`salary` entries (a rail model rather than a per-facet `section`, since Location/Salary fold several params)
- [x] 2.3 Add the static `category → section` map (`CATEGORY_GROUP`, a `Record<Category,…>` so svelte-check fails if a category is unassigned) + ordered `CATEGORY_GROUPS`
- [x] 2.4 Realize the salary/currency merge via the rail: a single `salary` entry renders currency + min-salary; no standalone Currency entry (no `FACETS`/search-param change)

## 3. Frontend: modal shell (two-pane, deferred apply)

- [x] 3.1 Build the modal container (`FilterModal.svelte`: backdrop, Escape/close/backdrop dismiss) opened from an **All filters** control
- [x] 3.2 Render the left rail: facets grouped under section headings, each with its staged-count badge; selecting an entry switches the active pane
- [x] 3.3 Render the right pane host that dispatches on rail-entry kind (facet / category / location / salary / visa / posted)
- [x] 3.4 Wire deferred apply: seed staged from applied on open, **Show results** commits + closes, dismiss discards
- [x] 3.5 Add the live count on **Show results** via debounced `previewCount` (`api.facetCounts(stagedParams).total`, scope-merged)

## 4. Frontend: sidebar becomes a selected-summary

- [x] 4.1 Add `FilterSummary.svelte`: applied values as chips grouped by facet, the **All filters** button with active-count badge, and **Reset all** (wired into JobsView; FiltersPanel kept for other views)
- [x] 4.2 Chip removal calls the live `FilterStore` directly (applies immediately); empty state when nothing is applied
- [x] 4.3 Modal reuses `FacetSection` for generic facets so control behavior is unchanged

## 5. Frontend: facet panes (chips, grouping, search, salary)

- [x] 5.1 All modal option controls render as chips (pill primitive), selected = active style
- [x] 5.2 `CategoryPane.svelte`: collapsible sections from the category→section map + facet-local option search (labels via `categoryLabel`)
- [x] 5.3 High-cardinality facets (Skills) reuse `SearchSelect` (facet-local search, selected pinned to front)
- [x] 5.4 Salary pane: currency chips + minimum-salary slider together; rail count = currencies + non-zero minimum

## 6. Frontend: location region → country → city tree

- [x] 6.1 Build the tree from `COUNTRY_REGION_MAP` + `CITY_COUNTRY_MAP` + live `regions`/`countries`/`cities` distributions, scoped to what has jobs (inline `$derived` in `LocationPane.svelte`)
- [x] 6.2 Render the chip tree (region heading expands to country pills; country pill has a distinct expand-to-cities chevron); region shows its count
- [x] 6.3 Selection at each level stages the correct `regions` / `countries` / `cities` param independently
- [x] 6.4 Surface unmapped cities in a flat, count-capped, searchable "Other cities" fallback. NOTE: the `cities` facet is an LLM-open vocabulary (jobview backfills beyond the dictionary), so city-under-country nesting only fires for the ~60 beacon cities — in practice this lands as **region→country tree + flat searchable cities** (see design Open Questions)

## 7. Responsive / mobile

- [x] 7.1 Full-screen modal on small viewports (rail + pane compressed two-pane); reachable via the existing mobile edge tab; **Show results** applies + closes. (Rail is cramped on narrow phones — candidate for a follow-up top-selector refinement.)

## 8. Verification

- [x] 8.1 `go build ./... && go vet ./... && go test ./...` (backend maps + gen-contracts) green — 50 packages
- [x] 8.2 `svelte-check` clean for the touched frontend; visual-verified specialization / location tree / salary / mobile / sidebar summary in the running app against the approved prototype
- [x] 8.3 Manual pass (in-app, automated): modal toggle stays deferred (URL unchanged), **Show results** applies to the URL (`?category=backend&seniority=senior`) + sidebar chips, chip removal applies immediately (URL back to `?seniority=senior`). URL round-trip via the unchanged FilterStore/UrlSyncedState. Minor polish noted: the Show button shows no number until the first debounced count resolves.
