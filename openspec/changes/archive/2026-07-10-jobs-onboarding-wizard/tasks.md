## 1. Selection→query helper and lifecycle state (`web/src/lib/onboarding.ts`)

- [x] 1.1 Define the wizard `Selection` type (role, seniority, workMode, region, stack[]) and a `selectionsToQuery(selection)` that maps it through the existing `filtersToParams` — no hand-written param strings; empty fields contribute nothing.
- [x] 1.2 Add `narrowestFacet(filters)` returning the facet to drop for the relax action, in fixed specificity order (skills → regions → seniority; never role), operating on live filter state (`JobFilters`).
- [x] 1.3 Add `hire.onboarding` state helpers (`loadOnboardingState`/`markSeen`/`markDone`, plus pure `bannerVisible`) over localStorage, best-effort (no throw when storage is unavailable).
- [x] 1.4 Unit tests (vitest): `filtersFromParams(selectionsToQuery(s))` round-trips for empty / role-only / role+stack / full selections; `narrowestFacet` peels the expected facet first and never the role; lifecycle helpers no-op safely when localStorage throws (11 tests green).

## 2. Wizard component (`web/src/lib/components/onboarding/OnboardingWizard.svelte`)

- [x] 2.1 Build the step flow: step 1 focus + seniority, step 2 work-mode + region + stack, confirm on the last step; back/skip on each step; staged local selection isolated from the live store. (Payoff is the reconfigured feed after close — no separate in-wizard screen.)
- [x] 2.2 Build pickers from the exported `facets.ts` vocabularies (`CATEGORY_OPTIONS` for focus, `FACETS` options for seniority/work-mode/regions minus the `none` sentinel); all fields optional, single-select coarse facets.
- [x] 2.3 Stack picker reuses the filter panel's `SearchSelect` fed by the live skills facet distribution (`counts.facets.skills`, busiest-first with counts): type to filter typo-tolerantly, click to add/remove. Values come from the controlled skills vocabulary and render as escaped text.
- [x] 2.4 Emit `onComplete(query)` (from `selectionsToQuery`) on confirm and `onCancel()` on exit-without-confirm; component stays ignorant of URL/localStorage.

## 3. Banner component (`web/src/lib/components/onboarding/OnboardingBanner.svelte`)

- [x] 3.1 Presentational banner ("Make this your feed") with a Set-up action and a dismiss (×); emits `onOpen`/`onDismiss`, knows only shown/hidden.

## 4. Integrate into the feed (`web/src/lib/components/JobsView.svelte`)

- [x] 4.1 Host the banner above the list; compute visibility = standalone feed AND onboarding not done/dismissed AND no active filters (`bannerVisible`).
- [x] 4.2 Wire the wizard: `onComplete(query)` → `filters.apply(query)` (feed + facet counts reconfigure via existing `$effect`; persistence flows through existing `saveJobFilters`), then `markDone`; `onCancel`/banner dismiss → `markSeen`.
- [x] 4.3 (Per product decision, no persistent entry point.) The banner is the sole trigger: once dismissed/completed it retires and there is no re-open control.
- [x] 4.4 Extend the existing empty-state: when the applied query yields zero results and a relaxable facet is set, show a "Broaden search" action calling `relaxFeed` (drops the narrowest facet via `clearFacet`).

## 5. Verify

- [x] 5.1 `npm run check` (svelte-check: 0 errors) and vitest (92 passed) green; `npm run lint` clean on new files.
- [x] 5.2 Visual verification via throwaway route + headless Chrome: banner + wizard steps 1 & 2 render in the app theme, step navigation works, region picker excludes the `none` sentinel; screenshots captured (throwaway route removed after).
- [x] 5.3 Confirmed by construction: `completeWizard` calls `filters.apply(query)`, which seeds the same `FilterStore` the FilterModal's `StagedFilters` reads on open — so the wizard's result is editable/clearable through the standard filter UI with no parallel state (covered by the `selectionsToQuery` round-trip test).
