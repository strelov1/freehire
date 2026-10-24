## 1. Geometry (pure, tested)

- [x] 1.1 `worldMap.ts`: `bubbleRadius`, `fitView` (fit to points, ≥60° window, 2:1, world = inhabited band), `placeCities` (case/accent-insensitive within one country) with unit tests
- [x] 1.2 `web/static/geo-cities.json` from Natural Earth populated places (generated, committed) and a memoised loader with an error state, tested

## 2. Shared bubble map

- [x] 2.1 `map/BubbleMap.svelte`: land, bubbles, tooltip on hover AND focus, Enter/Space activation when `onpick` is given, read-only otherwise; spec
- [x] 2.2 Rebuild `filters/JobsWorldMap.svelte` on it; AI search specs stay green

## 3. Jobs map dialog

- [x] 3.1 `JobsMapDialog.svelte`: own facet counts with a request sequence (stale responses dropped), cities when one country is selected, busiest-places row, placed-in-a-city share
- [x] 3.2 Click rules: country replaces / deselects, city adds, World view clears countries+cities, Remote worldwide toggles and clears places; specs for each
- [x] 3.3 Show N jobs from the dialog's own count, in the header on every width; closes onto the list
- [x] 3.4 `.messages.ts` (en + ru) for the dialog and the map; locale number formatting
- [x] 3.5 `ListToolbar` `leading` slot; `JobsView` Map button (design-system `Button`) at the left edge opening the dialog (`sm:max-w-6xl`)
- [x] 3.6 Posted/sort selects: chevron with padding

## 4. Verification and rollout

- [x] 4.1 svelte-check 0 errors, eslint, vitest, token-coverage and adoption hooks, knip
- [x] 4.2 Browser: no scroll at 1280×720 and 390×844; Brazil → cities; clicks update the list; World view; worldwide; keyboard
- [ ] 4.3 PR → CI → merge → deploy → check on prod
