## Why

The user profile today captures *what* work a person wants (specializations + skills) but nothing about *where* and *how* they want to work. Real preferences are compound and directional — "I'll work remote for LATAM, on-site where I live in Florianópolis, and I'm willing to relocate to Berlin" — and users have no way to record them. Without this, "Apply my profile" can only seed category/skill filters, and future matching/notifications have no geography signal.

## What Changes

- Extend the singleton user profile with an optional, structured **location & work-mode preferences** block, expressive enough to combine many willingness statements at once.
- Model it as three independent blocks stored in one nullable `location_preferences` JSONB column on `user_profiles`:
  - `work_modes[]` — which arrangements the user accepts (subset of `enrich.WorkModeValues`).
  - `remote{regions[], countries[]}` — remote reach (legal/timezone); empty = worldwide.
  - `base{country, city}` — where the user is now (a single place).
  - `relocation{open, regions[], countries[], cities[]}` — willingness to move and the targets; `open` + empty targets = anywhere.
- Validate on save: regions ⊆ `enrich.RegionValues`, countries = ISO 3166-1 alpha-2 (lowercase) validated against the `internal/location` dictionary, work modes ⊆ `enrich.WorkModeValues`, cities trimmed/deduped/capped. Every field optional; a NULL column means "never set".
- Extend the profile wire shape (`GET`/`PUT /api/v1/me/profile`) to carry `location_preferences`.
- Add a **"Where & how you want to work"** section to the profile edit form, reusing the existing facet option vocabularies.
- Extend the **Apply my profile** action so it flattens the three blocks into the jobs-filter facets: `regions ← remote.regions ∪ relocation.regions`; `countries ← remote.countries ∪ base.country ∪ relocation.countries`; `cities ← base.city ∪ relocation.cities`; `work_mode ← work_modes`; `relocation ← [supported, required]` when `open`.

Not breaking: the block is additive and optional; existing profiles (specializations + skills only) stay valid, and a NULL block applies no location filters.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `search-profiles`: the profile entity gains an optional `location_preferences` block; save-validation and the fetch/save wire shape extend to cover it.
- `filter-modal`: the **Apply my profile** action additionally seeds the jobs-filter location facets (work_mode, regions, countries, cities, relocation) from the profile's location preferences.

## Impact

- **DB**: new migration `0009_user_profile_location.sql` — `ALTER TABLE user_profiles ADD COLUMN location_preferences jsonb`. Must be applied to prod manually before deploy (repo has no versioned migration runner).
- **sqlc**: `internal/db/queries/user_profiles.sql` (`UpsertUserProfile`/`GetUserProfile`) + regenerate `internal/db`.
- **Go**: `internal/userprofile` (new `LocationPreferences` types + validation + `ErrInvalid*` sentinels), `internal/handler/me_profile.go` (wire shape). Reuses `enrich.WorkModeValues`/`RegionValues` and the `internal/location` country dictionary.
- **Frontend**: `web/src/lib/types.ts` (`UserProfile`), `web/src/lib/api.ts` + `web/src/lib/profile.svelte.ts` (`save`), `web/src/lib/components/ProfileForm.svelte` (new section), `web/src/lib/facets.ts` (export the `WORK_MODE`/`REGION`/`RELOCATION` option builders), `web/src/lib/facetModel.ts` + `web/src/lib/stagedFilters.svelte.ts` (`filtersFromProfile`/`applyProfile`).
- **Out of scope (future)**: matching, recommendations, and notifications consuming these preferences — the JSONB shape already supports them; denormalize to typed columns only when a concrete SQL query needs an index.
