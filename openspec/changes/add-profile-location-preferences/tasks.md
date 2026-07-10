## 1. Schema & data access

- [x] 1.1 Add `migrations/0009_user_profile_location.sql` — `ALTER TABLE public.user_profiles ADD COLUMN location_preferences jsonb;` (nullable, no default). (sqlc reads incremental ALTERs cumulatively, so no edit to `0001_init.sql` — that would double-define the column.)
- [x] 1.2 Update `internal/db/queries/user_profiles.sql`: `UpsertUserProfile` sets `location_preferences` (new param) and `GetUserProfile` returns it; add `user_profiles.location_preferences → json.RawMessage` override in `sqlc.yaml`; run `make sqlc` and commit regenerated `internal/db`.

## 2. Domain types & validation (`internal/userprofile`)

- [x] 2.1 Country validation: shape-check (two ASCII letters, lowercased) inside `internal/userprofile`, NOT membership in `internal/location` — the location dictionary is a curated subset that omits real countries (jm/cu), so a membership check would reject countries the frontend legitimately offers (found in review; see design Risks).
- [x] 2.2 Define `LocationPreferences` (+ nested `GeoSet`, `BaseLocation`, `Relocation`) in `internal/userprofile`, and add `ErrInvalidWorkMode`, `ErrInvalidRegion`, `ErrInvalidCountry`, `ErrTooManyCountries`, `ErrTooManyCities` sentinels.
- [x] 2.3 Implement validation + normalization in `Service.Save` (work_modes ⊆ `enrich.WorkModeValues`; regions ⊆ `enrich.RegionValues`, both case-insensitive; countries shape-checked; cities trimmed/deduped/capped; base.country validated; whole block optional, empty→NULL, invalid → reject the save). Unit-test the Florianópolis combined case, the optional/absent case, enum-case normalization, and each rejection.
- [x] 2.4 Thread `location_preferences` (marshal to/from JSONB) through the `Repository`: satisfied by the sqlc-generated `UpsertUserProfileParams`/`UserProfile` carrying `json.RawMessage` — the interface uses `db` types, so it flows through unchanged; round-trip covered by the combined-case test capturing the upsert param.

## 3. HTTP wire shape (`internal/handler/me_profile.go`)

- [x] 3.1 Extend `saveProfileRequest` (`*LocationPreferences`) and `profileResponse` (`json.RawMessage`, echoed verbatim) with the optional `location_preferences` block; map request→service and stored→response; map the new sentinels to 400 in `profileError`. Added unit handler tests (fake repo, no DB) for parse-and-reach-upsert, `400` on invalid block, and GET echo.

## 4. Frontend model & API

- [x] 4.1 Extend the `UserProfile` TS type (`web/src/lib/types.ts`) with `location_preferences` + `LocationGeoSet`/`LocationBase`/`LocationRelocation`; update `api.saveProfile` and `profileStore.save` to carry it.
- [x] 4.2 Export `WORK_MODE_OPTIONS`, `REGION_OPTIONS`, `COUNTRY_OPTIONS` from `web/src/lib/facets.ts` (matching `CATEGORY_OPTIONS`). RELOCATION not exported — the form uses a boolean toggle, not the job vocabulary (YAGNI).

## 5. Apply-my-profile flatten

- [x] 5.1 `filtersFromProfile(profile)` now flattens `location_preferences` into facets (work_mode; regions = remote ∪ relocation; countries = remote ∪ base ∪ relocation; cities = base ∪ relocation; relocation = [supported,required] if open — `open` gates the whole relocation contribution); `stagedFilters.applyProfile(profile)` + FilterModal call widened. Tests extended (combined Florianópolis case, not-open case, no-location case) — 7/7 green.

## 6. Profile edit form

- [x] 6.1 Added "Where & how you want to work" section to `ProfileForm.svelte`: work_mode pills, remote-reach region pills + country SearchSelect, base country `<select>` + city Input, relocation toggle + target region/country/city controls; two-way bound; `buildLocation()` gates relocation targets on the toggle and collapses an all-empty block to null. aria-labels on the select/city inputs (review). svelte-check + eslint clean.

## 7. Verification

- [x] 7.1 Verified: `go build/vet/test ./...` green; `make sqlc` no-diff; web `svelte-check` 0/0, `vitest` 122/122, changed-file `eslint` clean (only a pre-existing `URLSearchParams` lint in stagedFilters remains, unrelated); web prod build succeeds. Live DB JSONB round-trip not run standalone — covered by handler marshaling tests + the identical `companies.company_info`/`jobs.enrichment` `json.RawMessage` precedent. Reviewed by finder agents (backend + frontend); all confirmed findings fixed.
