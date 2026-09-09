## 1. Geography resolution + facet composition (pure logic)

- [x] 1.1 Add a small pure module (e.g. `web/src/lib/eligibleForMe.ts`) exporting the
      `GeoSource` precedence function (profile country → edge-derived region → null)
      per `specs/eligible-for-me-filter` "The country/region source follows a fixed
      precedence and degrades gracefully".
- [x] 1.2 In the same module, export the function from `GeoSource` to the facet
      values to stage (`countries=[code]` or `regions=[code,'global','none']`), per
      "The filter composes existing location facet values" and "The safety values
      are always included."
- [x] 1.3 Export the "is currently on" predicate that reports whether the currently
      included `regions`/`countries` values already contain everything the `GeoSource`
      would stage, per "Turning the filter off removes exactly what it staged." (Takes
      the include arrays directly rather than a `FacetStore`, so it stays a pure
      function callers can test without constructing a store — see design.md.)
- [x] 1.4 Unit tests for 1.1-1.3 (precedence order, both `GeoSource` kinds, the null
      case, the on/off predicate against various staged states), following the
      existing pattern in `web/src/lib/profileFilters.test.ts`.

## 2. LocationPane integration

- [x] 2.1 Fetch `/geo/region` lazily in `LocationPane.svelte` (only when
      `profileStore` has loaded and carries no `location_preferences.base.country`),
      reusing `web/src/lib/geoScope.ts`'s existing response shape — no changes to
      `geoScope.ts` or the `/geo/region` endpoint itself.
- [x] 2.2 Render the "Eligible for me" pill alongside the existing `Global` /
      `Not specified` flat pills, disabled when the resolved `GeoSource` is null, per
      `specs/filter-modal` "The Location pane offers an 'Eligible for me' pill."
- [x] 2.3 Wire the pill's click handler to stage/unstage via `store.add`/`store.remove`
      using the 1.2 value set, and its pressed state via the 1.3 predicate.
- [x] 2.4 Confirm the pane's existing `selectedChips` row already picks up the staged
      values with no changes needed (per design.md, chips are derived from
      `regionF.include`/`countryF.include`) — adjust only if a staged value doesn't
      render there today. (Confirmed: `selectedChips` reads `regionF.include`/
      `countryF.include` directly, so it picks up the toggle's staged values with no
      changes.)

## 3. Verification

- [x] 3.1 `pnpm --filter web check` / relevant lint+typecheck for the touched files.
      (Ran `pnpm run check` (`svelte-kit sync && svelte-check`) and `pnpm run lint`
      inside `web/` — no `.openspec.yaml`-managed workspace filter exists, each
      package is installed/run standalone. 0 errors from svelte-check; eslint clean
      on the touched files; all pre-existing warnings elsewhere unrelated.)
- [x] 3.2 `pnpm --filter web test` (or the workspace's actual vitest invocation) for
      the new unit tests. (`pnpm test` in `web/`: 155 files / 1791 tests passed,
      including the 13 new `eligibleForMe.test.ts` cases.)
- [x] 3.3 Manual browser verification, via a real Vite dev server + Playwright
      (Chromium) against a small local JSON stub standing in for the Go API — no
      Docker/Postgres/Meilisearch available in this environment, so sign-in and real
      job fixtures were out of reach; documented as a residual gap below.
      - **Disabled state** (no CF-IPCountry header, unauthenticated): pill renders
        disabled, title "We couldn't determine your location", `aria-pressed=false`.
        Screenshot confirmed visually.
      - **Region-level enabled state** (`cf-ipcountry: ID` header, simulating an
        Indonesian visitor via the existing `/geo/region` resolution): pill enabled,
        title "Click to include".
      - **Toggle on**: `aria-pressed` flips to `true`; the pane's selected-location
        chips show exactly `APAC`, `Worldwide`, `Not specified` — confirming
        `selectedChips` picks up the staged values with no code changes, per 2.4.
      - **Toggle off**: `aria-pressed` flips back to `false`.
      - Zero browser console errors across all of the above.
      - **Not verified live** (no backend available): the profile-country precedence
        path (needs a signed-in account with `location_preferences.base.country`
        set) and a real job whose geography is genuinely unresolved staying visible
        with the pill on (needs seeded job fixtures) — both are covered by the
        `eligibleForMe.test.ts` unit tests (1.4) at the logic level, but not observed
        rendering real job cards end-to-end.
