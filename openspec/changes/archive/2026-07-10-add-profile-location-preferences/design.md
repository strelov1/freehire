## Context

The singleton `user_profiles` table today holds only `specializations text[]` and `skills text[]` (one row per user, keyed by `user_id`). The domain lives in `internal/userprofile` (Service + Repository + validation), served over `GET/PUT/DELETE /api/v1/me/profile` (`internal/handler/me_profile.go`), and consumed on the frontend by `ProfileForm.svelte`, the `profileStore`, and the jobs filter modal's **Apply my profile** action (`filtersFromProfile` in `web/src/lib/facetModel.ts` → `stagedFilters.applyProfile`).

The controlled vocabularies already exist and are reused, not reinvented: `enrich.WorkModeValues` (`remote`/`hybrid`/`onsite`), `enrich.RegionValues` (9 macro-regions incl. `global`/`latam`), and the ISO alpha-2 country dictionary in `internal/location`. The jobs filter already exposes `work_mode`, `regions`, `countries`, `cities`, and `relocation` facets, so the profile stores facet-compatible values.

## Goals / Non-Goals

**Goals:**

- Let a user record compound, directional location/work preferences ("remote for LATAM" + "on-site in Florianópolis" + "willing to relocate to Berlin") in one profile.
- Reuse existing vocabularies so the same values flow into the jobs filter with no new taxonomy.
- Make "Apply my profile" seed the location facets in addition to category/skills.

**Non-Goals:**

- Matching / recommendations / notifications consuming these preferences (future; the shape already supports them).
- Multi-base support, per-mode reach modelling beyond the three blocks, or a separate "worldwide" toggle.
- Geocoding or validating cities against a dictionary (cities stay free-text, mirroring the jobs `cities` facet).

## Decisions

**1. One nullable `location_preferences` JSONB column, not typed columns.**
The preference is a single nested value read and written whole (form → API → form) and is not filtered in SQL anywhere yet. Typed columns (≈9 arrays/scalars) would be premature normalization with unwieldy CHECK constraints. JSONB + a typed Go struct (`userprofile.LocationPreferences`) gives type safety at the boundary — the same pattern as the `jobs.enrichment` blob — and lets the shape evolve during MVP without a migration per tweak. *Alternative considered:* typed columns, better for a future SQL `WHERE`. *Seam:* when matching needs to index a field (e.g. `remote.regions`), denormalize that one field to a column then.

**2. Three willingness blocks with relocation as a modifier of the physical block.**
`work_modes` is the arrangement axis; `remote{regions,countries}` is remote reach; `base{country,city}` is the current single location; `relocation{open,regions,countries,cities}` is a directional modifier (destinations distinct from base). This captures the Florianópolis example directly. *Alternative considered:* a flat facet mirror (one set of regions/countries/cities) — rejected because it collapses "where I am now" and "where I'd move", which is the whole point.

**3. `relocation.open` boolean + empty targets = anywhere.**
No separate "worldwide" checkbox. `open=false` → not willing; `open=true` with empty targets → anywhere; `open=true` with targets → only those. One field, no redundant state.

**4. Apply-to-filter is a lossy flatten, done frontend-side in `filtersFromProfile`.**
`work_mode ← work_modes`; `regions ← remote.regions ∪ relocation.regions`; `countries ← remote.countries ∪ base.country ∪ relocation.countries`; `cities ← base.city ∪ relocation.cities`; `relocation ← [supported, required]` iff `relocation.open`. The existing `seed()`/`facetAdd` reducer already trims and dedupes, so unions are free. The flatten intentionally loses the base-vs-relocation distinction (the filter is an AND-narrowing of "places relevant to me"); the structured block is retained for future matching.

**5. Validation in `userprofile.Service.Save`, mirroring the existing pattern.**
New `LocationPreferences` (with nested `GeoSet`, `BaseLocation`, `Relocation`) validated: work modes ⊆ `enrich.WorkModeValues` and regions ⊆ `enrich.RegionValues`, both matched case-insensitively (lowercased); countries lowercased + shape-checked (two ASCII letters — see the Risks note on why not membership); cities trimmed/deduped/capped; everything optional. New sentinels `ErrInvalidWorkMode`, `ErrInvalidRegion`, `ErrInvalidCountry`, `ErrTooManyCountries`, `ErrTooManyCities` alongside the existing `ErrInvalidSpecialization` family. An invalid block rejects the whole save.

## Risks / Trade-offs

- **JSONB is opaque to SQL queries** → acceptable now (nothing queries it); denormalize per the seam in Decision 1 when matching arrives.
- **Lossy apply-to-filter** (base vs relocation merged, relocation → job-relocation facet) → the filter is a convenience narrowing, not an exact encoding; the structured profile remains the source of truth for future consumers.
- **Cities are free-text** → the profile form must reuse the jobs city search UX (raw strings) rather than a curated list, so a user can only usefully type cities that exist in the catalogue's distribution; acceptable and consistent with the jobs filter.
- **Country codes are shape-validated, not membership-validated** → the `internal/location` dictionary is a curated subset (it omits real countries like `jm`/`cu`) built to *derive* job geography, so validating a user's picked country against it would reject legitimate countries the frontend offers. Instead the country is validated for shape (two ASCII letters) and lowercased. A well-formed but job-less code is harmless — it simply matches nothing, like an unknown city or skill. No coupling to `location` for validation.

## Migration Plan

1. Add `migrations/0009_user_profile_location.sql`: `ALTER TABLE public.user_profiles ADD COLUMN location_preferences jsonb;` (nullable, no default; existing rows read as `NULL` = no preferences).
2. Apply it to prod **manually before deploy** — the repo has no versioned migration runner and initdb migrations only run on first volume init (per the migrations gotcha; an unapplied column would 500 all profile reads).
3. Regenerate `internal/db` via `make sqlc` after updating `internal/db/queries/user_profiles.sql`.
4. Rollback: the column is additive and nullable; a rollback binary ignores it, and dropping the column loses only the new preferences.

## Open Questions

- Cap for cities count and for country-code list length — pick sane constants during implementation (e.g. cities ≤ 10, countries ≤ 20) and encode as validation, matching the `maxSpecializations = 5` precedent.
