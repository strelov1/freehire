## 1. Backend: geography mappings + contract export

- [x] 1.1 Add a canonical `city → country` map in `internal/location` over the beacon-city set (`nameToCity` / `nameToCountry`), plus a test asserting every emittable beacon city resolves to exactly one ISO country code
- [x] 1.2 Expose the `country → region` grouping (inverse of `regionCountries`) as a package-accessible map with a test asserting every grouped country maps to exactly one controlled region value
- [x] 1.3 Add an `emitMap` helper to `cmd/gen-contracts` (emit a frozen TS `Record<string,string>` / `Record<string,readonly string[]>`) with a unit test on its output shape
- [x] 1.4 Wire `gen-contracts` to emit `COUNTRY_REGION_MAP` (country→region) and `CITY_COUNTRY_MAP` (city→country); regenerate `web/src/lib/generated/contracts.ts` and commit

## 2. Frontend: staging store + facet registry metadata

- [ ] 2.1 Add a `StagedFilterStore` (clone-on-open over `JobFilters`, same facet mutators as `FilterStore`, `commit()` → `FilterStore.apply()`, `discard()`), with unit-testable pure logic for clone/mutate/commit
- [ ] 2.2 Extend `FacetDef` with a `section` (rail grouping) field and assign every facet to `ROLE` / `PAY & BENEFITS` / `REQUIREMENTS & ELIGIBILITY`
- [ ] 2.3 Add the static `category → section` map (Engineering; Data & AI; Quality & Security; Design; Product & Management; Go-to-market & Support) with a test asserting it covers every `CATEGORY_VALUES` value exactly once
- [ ] 2.4 Merge `salary_currency` + the salary-min control into one `salary` facet definition; drop the standalone Currency facet from the registry

## 3. Frontend: modal shell (two-pane, deferred apply)

- [ ] 3.1 Build the modal container (backdrop, Escape/close/backdrop dismiss, focus trap) opened from an **All filters** control
- [ ] 3.2 Render the left rail: facets grouped under section headings, each with its staged-count badge; selecting an entry switches the active pane
- [ ] 3.3 Render the right pane host that dispatches to the active facet's control
- [ ] 3.4 Wire deferred apply: seed staged from applied on open, **Show results** commits + closes, dismiss discards
- [ ] 3.5 Add the live count on **Show results** via debounced `api.facetCounts(stagedParams).total`

## 4. Frontend: sidebar becomes a selected-summary

- [ ] 4.1 Reshape `FiltersPanel` into a summary: applied values as chips grouped by facet, the **All filters** button with active-count badge, and **Reset all**
- [ ] 4.2 Chip removal calls the live `FilterStore` directly (applies immediately); empty state when nothing is applied
- [ ] 4.3 Verify the modal reuses the extracted per-facet controls so control behavior is unchanged

## 5. Frontend: facet panes (chips, grouping, search, salary)

- [ ] 5.1 Ensure all modal option controls render as chips (pill primitive), selected = active style
- [ ] 5.2 Specialization pane: collapsible sections from the category→section map + facet-local option search
- [ ] 5.3 High-cardinality pane (Skills): facet-local search filtering the distribution, selected values pinned above
- [ ] 5.4 Salary pane: currency chips + minimum-salary slider together; rail count = currencies + non-zero minimum

## 6. Frontend: location region → country → city tree

- [ ] 6.1 Build the tree from `COUNTRY_REGION_MAP` + `CITY_COUNTRY_MAP` + live `regions`/`countries`/`cities` distributions (pure builder function, unit-testable)
- [ ] 6.2 Render the chip tree with distinct expand vs select affordances; region shows its count, country/city chips have none
- [ ] 6.3 Selection at each level stages the correct `regions` / `countries` / `cities` param independently
- [ ] 6.4 Surface unmapped-but-present cities in a flat fallback within the Location pane so they stay selectable

## 7. Responsive / mobile

- [ ] 7.1 Full-screen modal on small viewports with the rail collapsing to a facet selector; reconcile with the existing mobile filter access so every facet is reachable and **Show results** applies + closes

## 8. Verification

- [ ] 8.1 `go build ./... && go vet ./... && go test ./...` (backend maps + gen-contracts) green
- [ ] 8.2 `svelte-check` clean for the touched frontend; visual-verify the five states (modal→specialization, →location tree, →skills search, →salary, sidebar summary) against the approved prototype
- [ ] 8.3 Manual pass: deferred apply, chip removal applies immediately, URL round-trips, back/forward, reset all
