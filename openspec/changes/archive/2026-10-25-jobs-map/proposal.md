## Why

The job list says how many postings match but not where they are. A visitor searching for "backend, remote LATAM" cannot see that most of the hits sit in São Paulo, or narrow to a city without knowing its name. The AI search already draws a small, read-only world map of a plan's countries; a spike on 2026-10-24 (branch `spike/jobs-map`) put an interactive version of it on `/jobs` against production data and the owner accepted the result.

## What Changes

- A **Map** button at the left edge of the `/jobs` list toolbar opens a wide dialog showing the current search on a world map. There is no list/map toggle: the list stays where it is, behind the dialog.
- The map draws one bubble per country, sized by job count, and zooms to fit what it draws. With one country selected it draws that country's **cities** instead.
- **Clicking narrows the same filters the list reads**, so the list behind the dialog updates: a country click selects that country (replacing any other), a click on the selected country deselects it, a city click selects only that city (the location filters combine with OR on the server, so a city beside its country would select the whole country). **World view** clears both the country and city filters.
- **Remote worldwide** — postings open to anyone anywhere have no place to draw; a button with their count toggles that filter, and turning it on clears the country and city filters (the location filters combine with OR, so keeping a place beside it would widen the list instead of switching it).
- **Show N jobs** — always visible without scrolling — closes the dialog onto the updated list. The dialog never scrolls, on a laptop or a phone.
- The posted/sort selects in the same toolbar get their own chevron with padding: the native arrow overlapped the selected text.
- The AI search's read-only map and the new interactive one share one bubble-map component and one tested geometry module instead of two copies.

## Capabilities

### New Capabilities
- `jobs-map`: the `/jobs` map dialog — what it draws for the current filters, how clicks change the filters, the worldwide toggle, and its layout guarantees.

### Modified Capabilities
<!-- none: the AI search map's behaviour is unchanged; only its implementation is shared -->

## Impact

- **Web**: `web/src/lib/components/JobsView.svelte` (Map button, dialog), `ListToolbar.svelte` (a left-edge `leading` slot), a new `JobsMapDialog`/map component, the shared bubble map (refactoring `filters/JobsWorldMap.svelte`), `web/src/lib/worldMap.ts` (city lookup, view fitting), `.messages.ts` files, specs.
- **Static data**: `web/static/geo-cities.json` (~200 KB, Natural Earth populated places, public domain), fetched only when one country is selected.
- **API**: none — the map reads `GET /api/v1/jobs/facets` with `facets=countries,cities,regions`, which already exists.
