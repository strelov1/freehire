## Why

The profile editor's two city inputs ("where you're based" and "where you'd relocate") are
plain free-text fields — unlike every other geography field nearby (the base-country
`<select>`, the remote/relocation region and country pickers), they offer no suggestions and
no validation against a real place, so a user can save a typo or an ambiguous fragment with
no feedback. The app already carries a global, population-ranked GeoNames city dictionary
(`internal/location`, ~34k cities) for resolving CV/job location text, but nothing exposes it
for interactive lookup — this proposal adds that lookup and wires the two profile fields to it.

## What Changes

- Add `location.SearchCities(query, countryCode string, limit int) []CityMatch`: a
  population-ranked, case-insensitive prefix search over the existing embedded GeoNames
  dictionary, optionally narrowed to one ISO country code.
- Add a new read-only HTTP endpoint, `GET /geo/cities?q=&country=`, exposing that search as
  a facet-shaped list response (`{"value", "label"}` pairs), following the project's existing
  remote-search facet pattern.
- Add `searchCities(query, country?)` to the web client's facet-search helpers, calling the
  new endpoint.
- Rework `ProfileForm.svelte`'s two city inputs to use the search endpoint instead of raw
  text entry:
  - "Where you're based" → city becomes a single-value search-and-pick control (via the
    already-present, previously unused `RemoteSearchSelect` component), narrowed by the
    selected base country.
  - "Where you'd relocate" → cities becomes a multi-value search-and-pick control (the same
    component's native chip mode), replacing the manual Enter-to-add text field.
- No storage or wire-contract change: `location_preferences.base.city` and
  `.relocation.cities` keep being saved and validated exactly as today (trimmed, deduplicated
  free-text strings) — the UI now helps the user pick a real, well-formed value, but the
  server-side contract is unchanged, and an existing user's previously saved free-text city
  keeps rendering and saving correctly (a picker shows an unrecognized saved value via its
  fallback label rather than blanking it).

## Capabilities

### New Capabilities
- `city-search`: population-ranked prefix search over the existing GeoNames city dictionary,
  exposed as an HTTP endpoint and used to back autocomplete UI for city entry.

### Modified Capabilities
_None._ `search-profiles`' `location_preferences` save/read contract (city as validated,
trimmed free text) is unchanged — only the editor's input mechanism changes, not what is
accepted or stored.

## Impact

- New code: `internal/location` (search function + tests), a new handler + route
  (`internal/handler`), `web/src/lib/facets.ts` (search helper). Not documented in
  `web/static/openapi.yaml` — that file is the curated public ChatGPT Actions schema, not a
  general API reference, and this is an internal helper for the SPA's own profile form.
- Changed code: `web/src/lib/components/ProfileForm.svelte` (two fields rewired; the
  now-unused `cityDraft`/`addCity` free-text-chip code is removed).
- No new external dependency, no new database table or column, no migration.
