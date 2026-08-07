## 1. Backend: city search over the dictionary

- [x] 1.1 In `internal/location`, add a population-ordered, (Name, Country)-deduplicated
      `[]cityEntry` built once at init alongside `cityDict` (extend `loadCityDict` or add a
      sibling loader over the same TSV — do not re-parse independently).
- [x] 1.2 Add `SearchCities(query, countryCode string, limit int) []CityMatch` in
      `internal/location`: case-insensitive prefix match on canonical name, falling back to
      alias prefix; optional country filter; population-ranked (source order); blank query
      returns nothing.
- [x] 1.3 Unit tests for `SearchCities`: name-prefix match, alias-prefix match, dedup of an
      entry matched by multiple aliases, country filter, cap enforcement on a broad query,
      blank-query returns empty. (Two rounds of review also surfaced and fixed a
      same-alias-different-place override collision — see search.go's `claimed` guard.)

## 2. Backend: HTTP endpoint

- [x] 2.1 Add `internal/handler/geo.go`: `geoHandlers` struct + `newGeoHandlers`, following
      the `companiesHandlers`/`CompanySubindustries` pattern (no auth). `SearchCities`
      handler reads `q` and optional `country` query params, calls `location.SearchCities`
      with a limit of 20, responds `{"data": [{"value", "country"}]}` — a raw ISO code, not
      a pre-composed label (the frontend's existing `countryLabel()` composes the display
      string; see design.md's response-shape decision).
- [x] 2.2 Register `GET /geo/cities` on the `api` group in `internal/handler/handler.go`
      (mirrors the `companiesH` wiring).
- [x] 2.3 Integration test (`//go:build integration`, per `internal/handler`'s existing
      convention) covering: reachable + unauthenticated through the real `Register()` wiring,
      and `country` narrows results — confirmed RED (404) with the route registration
      temporarily disabled, then GREEN restored. Code-reviewed clean (no findings).
- [x] ~~2.4 Document `GET /geo/cities` in `web/static/openapi.yaml`.~~ Dropped: that file is
      not a general API reference — its own `info.description` states it is "the freehire
      ChatGPT Actions API" and "intentionally excludes" everything outside public job/company
      search for agent consumption. `/geo/cities` is an internal helper for the SPA's own
      profile form, not an agent-facing capability; adding it there would be scope creep
      against the file's stated purpose, not a genuine gap. (Planning-time assumption that
      didn't hold up once the file's actual scope was checked.)

## 3. Frontend: search helper

- [x] 3.1 Add `searchCities(query: string, country?: string): Promise<FacetOption[]>` to
      `web/src/lib/facets.ts`, calling `GET /geo/cities`. Composes the label via the
      existing `countryLabel()` (Intl.DisplayNames), matching the endpoint's raw-code
      response. Unit-tested (`cityOption` in facets.test.ts).

## 4. Frontend: wire ProfileForm's city fields

- [x] 4.1 Replace `baseCity`'s plain `<Input>` with `RemoteSearchSelect`, single-value
      semantics (`include = baseCity ? [baseCity] : []`, `onToggle` replaces/clears),
      search narrowed by `baseCountry`, `fallbackLabel={(v) => v}`.
- [x] 4.2 Replace the `relocCities` chip-list + Enter-to-add `<Input>` with
      `RemoteSearchSelect` in its native multi-select mode (`include = relocCities`,
      `onToggle` = existing `toggleIn`, `clearOnSelect`), search not narrowed by country.
      Removed the dead `cityDraft` state and `addCity()` function. Also moved the shared
      `RemoteSearchSelect` component's selected-chip row to render below the search input
      instead of above it (user-requested layout fix, affects every usage of the
      component: skills, base city, relocation cities, company/subindustry facets).
- [x] 4.3 Manual verification in the browser: confirmed end-to-end — real GeoNames
      results render with correct labels ("Florianópolis, Brazil"), picking replaces the
      single base-city value and adds a removable chip for relocation cities, dark theme
      renders correctly (readable text, matches the earlier `color-scheme` fix). The
      earlier-observed intermittent stuck "Searching…" state is not reproducible.

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./...` and `go vet -tags=integration ./...` — clean.
- [x] 5.2 `go test ./...` and `go test -tags=integration ./internal/handler/` — green.
      Frontend: `pnpm check` (0 errors), `pnpm test` (833/833), `pnpm lint` (0 errors,
      baseline-only warnings) all green.
