## 1. Backend — the upper-bound filter

- [x] 1.1 Add a `query_filter_test.go` case asserting `experience_years_max=3`
  produces `enrichment.experience_years_min <= 3`, that it composes with
  `experience_years_min` as a closed range, and that an absent/empty/non-numeric
  value produces no group. Test fails.
- [x] 1.2 Allow `experience_years_max` in `internal/search/query_params.go` and emit
  the `Lte` group in `internal/search/query_filter.go`, next to the existing floor.
  Test passes.
- [x] 1.3 Add a `query_params_test.go` case covering the new param's pass-through,
  matching how `experience_years_min` is already covered.

## 2. Backend — the no-experience phrase detector

- [x] 2.1 Add `jobfacts_test.go` cases: an explicit "no prior experience required"
  yields `0`; "no experience necessary" and "no previous experience needed" yield `0`;
  a description silent about experience still yields nil; "5+ years of experience"
  still yields `5`; a description carrying both an explicit no-experience statement
  and a "3 years ... is a plus" figure yields `0`. Tests fail.
- [x] 2.2 Add the boundary-matched phrase list to `internal/jobfacts/jobfacts.go` and
  wire it into `ExperienceYearsMin` so an explicit statement contributes `0` to the
  existing smallest-value walk. Tests pass.
- [x] 2.3 Add the known false-positive case as a test — "no prior experience with
  Kubernetes is required" must NOT yield `0` — and tighten the phrases until it holds.
- [x] 2.4 `gofmt -w` the touched Go files, then `go vet ./...` and `go test ./...`.

## 3. Contract and docs

- [x] 3.1 Declare the `ExperienceYearsMax` parameter in `web/static/openapi.yaml` and
  reference it from every endpoint that already references `ExperienceYearsMin`. Its
  description must state which attribute it bounds and that either bound excludes
  postings with no stated requirement.
- [x] 3.2 Add the param to `web/src/lib/docs/filters.ts` and the numeric-filter list
  in `web/src/lib/docs/api-spec.ts`, then regenerate `docs/API.md` via `gen:api-docs`.

## 4. SPA — filter state

- [x] 4.1 Add a `facetModel` test asserting `experienceYearsMax` round-trips through
  `filtersToParams`/`filtersFromParams` as `experience_years_max`, that `null` emits
  no param, that `0` DOES emit `experience_years_max=0` (the falsy-zero trap), and
  that a non-numeric URL value parses back to `null`. Test fails.
- [x] 4.2 Add `experienceYearsMax` to `JobFilters`, `emptyFilters`, `filtersToParams`,
  `filtersFromParams`, and `activeFilterCount` in `web/src/lib/facetModel.ts`. Test
  passes.
- [x] 4.3 Add the `setExperienceYearsMax` transition to both `web/src/lib/filters.ts`
  (live) and `web/src/lib/stagedFilters.svelte.ts` (deferred), following `setSalaryMin`.

## 5. SPA — the Experience pane

- [x] 5.1 Add `'experience'` to `RailKind` and the `Experience` entry to `RAIL` in
  `web/src/lib/filterSections.ts`, placed in the `ROLE` section after `category`.
  Update the rail test that asserts every facet param is reachable.
- [x] 5.2 Define the preset-stop array (no-experience, 1, 2, 3, 5, 8, 10, Any) with a
  label per stop, alongside `FRESHNESS_PRESETS`. Add a unit test that the index→value
  mapping is total and that the Any stop maps to `null`.
- [x] 5.3 Render the `kind: 'experience'` pane in `FilterModal.svelte`: the seniority
  `ChipFacet`, then the preset slider driven by index, then the permanent coverage
  note. Model the slider on the `kind: 'posted'` branch.
- [x] 5.4 Remove the seniority group from the `kind: 'category'` pane and update the
  filter-modal tests that assert its contents.
- [x] 5.5 Add the pane's staged count to the `selCount` switch in `FilterModal.svelte`
  so the rail entry counts seniority values plus a set years bound.
- [x] 5.6 Add the years bound to `FilterSummary.svelte` as a removable chip, following
  the `salaryMin` chip.

## 6. Verification

- [x] 6.1 Run the web test suite and `eslint` on the touched files; both clean.
- [x] 6.2 Run `go vet -tags=integration ./...` — the push-time guard.
- [x] 6.3 Drive the app: open the filter modal, confirm the `Experience` pane renders
  both controls, that moving the slider changes the result count and the URL, that the
  leftmost stop applies `experience_years_max=0`, and that an old `?seniority=senior`
  URL still selects the pill in its new home.
