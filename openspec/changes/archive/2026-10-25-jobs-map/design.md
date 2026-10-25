## Context

The spike (`/Users/i_strelov/Projects/hire-jobs-map`, branch `spike/jobs-map`, uncommitted) is the accepted layout. Its two-axis review found: no i18n, arbitrary Tailwind values the token-coverage hook rejects, a hand-rolled button, no specs, an unhandled city fetch, Enter-only keys, duplicated map code with the AI search's `JobsWorldMap.svelte`, a response race, World view leaving cities set, no way to deselect, and worldwide AND-ing with a country. All are in scope here.

## Goals / Non-Goals

**Goals:** the spec's behaviour; one shared map; tested geometry; no new API.

**Non-Goals:** remembering the dialog in the URL; regions/metros as map layers; panning or manual zoom; a standalone `/map` page.

## Decisions

### D1. One bubble map, two hosts
`web/src/lib/components/map/BubbleMap.svelte` renders land, bubbles, hover/focus tooltip and keyboard activation from a list of `{ key, label, count, lat, lng }` places and an optional `onpick`. Without `onpick` it is read-only. `filters/JobsWorldMap.svelte` (AI search) becomes a thin host on it; the new `JobsMapDialog.svelte` is the interactive host. Bubble sizing, view fitting and city matching live in `web/src/lib/worldMap.ts` as pure, tested functions (`fitView`, `bubbleRadius`, `placeCities`).

### D2. Clicks map to the filter store's own operations
The server combines the location filters (countries, cities, regions) with OR, so a click REPLACES places rather than adding to them. Country click: clear countries and cities, then add the country (or clear alone when it was the selected one). City click: clear countries, select only that city; the dialog remembers the focus country so it keeps drawing its cities. World view: clear countries and cities. Worldwide on: add `regions=global`, clear countries and cities (with OR, a place kept beside it would widen the list); off: remove it. Each is one or two store writes; the list's existing debounced reload follows. A small store method for "set exactly this value" is acceptable if two writes would reload twice — check how the store batches before adding one.

### D3. Counts are the dialog's own, ordered
The dialog fetches `facetCounts(store.params, { facets: ['countries','cities','regions'] })`, debounced 150 ms, with a request sequence number: a response whose number is not the latest is dropped and does not clear the loading state. **Show N jobs** reads this count, not the list's `listTotal`, so it is never one click behind.

### D4. City coordinates are a static file loaded through a helper
`web/static/geo-cities.json` (Natural Earth 10m populated places, `{cc: {name: [lat,lng]}}`, names lowercased NFC, also ASCII variants). Loaded once, lazily, by a module-level memoised loader with an error state; a failure shows the countries without cities and does not retry in a loop. Matching normalises case and strips accents on both sides.

### D5. Layout without arbitrary values
The design-system `Dialog` (`sm:max-w-6xl`). The map's height is bounded with existing scale utilities and the dialog body is a flex column whose map area takes the remaining height, so nothing scrolls; the busiest-places row is one line that scrolls sideways. Show N jobs is in the header row on every width.

### D6. Select chevrons
The posted/sort selects get `appearance-none`, right padding and a lucide `ChevronDown`, a fix the token check allows.

## Risks / Trade-offs

- [City names in the facet are free text, some junk ("Brazil")] → only matched names are drawn; the dialog states the placed share.
- [200 KB static file] → fetched only with one country selected, cached by the browser.
- [Refactoring the AI search map] → its existing specs must stay green; its behaviour is unchanged.
