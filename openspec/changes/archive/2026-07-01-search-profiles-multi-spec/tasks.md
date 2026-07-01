## 1. Root-cause the inactive Create button

- [x] 1.1 Reproduce the "Create profile never enables" dead-end (run the SPA signed-in, or
  drive it headless) and confirm the mechanism: skills are select-only from the facet pill
  wall, so an unfound skill leaves `skills.length === 0` and `canSubmit` false. Record the
  finding in the change notes (systematic-debugging Phase 1).

## 2. Data model (DB + sqlc)

- [x] 2.1 Add `migrations/0029_search_profiles_specializations.sql`: add
  `specializations TEXT[]`, backfill `ARRAY[specialization]`, add CHECK
  `cardinality(specializations) BETWEEN 1 AND 5`, drop `specialization`. Include the
  inverse SQL as a comment for rollback.
- [x] 2.2 Update `internal/db/queries/search_profiles.sql`: `CreateSearchProfile` inserts
  `specializations`; `UpdateSearchProfile` COALESCEs `specializations` (nullable arg).
- [x] 2.3 Run `make sqlc` and commit regenerated `internal/db/*` (models, querier,
  search_profiles.sql.go).

## 3. Service layer (`internal/searchprofile`) — TDD

- [x] 3.1 RED: extend `searchprofile_test.go` for `normalizeSpecializations` and
  Create/Update — valid set, dedupe, trim, unknown category → error, empty → error, >5 →
  error, nil-on-update → unchanged.
- [x] 3.2 GREEN: add `normalizeSpecializations`, `ErrEmptySpecializations`,
  `ErrTooManySpecializations`; retire the single-value `validSpecialization`/
  `ErrInvalidSpecialization` semantics; change `Create`/`Update` signatures
  (`specialization string` → `specializations []string`); wire the new
  `CreateSearchProfileParams`/`UpdateSearchProfileParams` fields.
- [x] 3.3 REFACTOR + re-run `go test ./internal/searchprofile/`; keep symmetry with
  `normalizeSkills`.

## 4. HTTP handler (`internal/handler/me_profiles.go`)

- [x] 4.1 Change request/response shapes: `Specialization string` → `Specializations
  []string` in create/update bodies and `searchProfileResponse` (+ `toSearchProfileResponse`).
- [x] 4.2 Map the new sentinels in `searchProfileError` (empty/too-many specializations →
  400); update the create/update handler calls to pass the slice.
- [x] 4.3 Extend the handler integration tests (`//go:build integration`) for the new
  wire shape and validation statuses; run `go test -tags=integration ./internal/handler/`.

## 5. Frontend contracts & store

- [x] 5.1 `web/src/lib/types.ts`: `SearchProfile.specialization: string` →
  `specializations: string[]`.
- [x] 5.2 `web/src/lib/api.ts`: `createSearchProfile`/`updateSearchProfile` take/serialize
  `specializations`.
- [x] 5.3 `web/src/lib/searchProfiles.svelte.ts`: `create`/`update` signatures use
  `specializations`.

## 6. Profile form UI (`SearchProfilesView.svelte`)

- [x] 6.1 Specialization input → `SearchSelect` multi-select over `CATEGORY_OPTIONS`
  (array state + toggle, client-side cap of 5); `canSubmit` requires ≥1 specialization.
- [x] 6.2 Skills input → `RemoteSearchSelect` whose `search(query)` filters the loaded
  skill distribution locally (dictionary-only, counts shown, chips removable); seed
  selected skills on edit so they stay visible/removable.
- [x] 6.3 Profile list renders all specialization labels (via `categoryLabel`) per profile.

## 7. Verify & finish

- [x] 7.1 `go build ./... && go vet ./...`; `cd web && npx svelte-check`.
- [x] 7.2 Verified the full backend round-trip against a real Postgres (fresh volume ran
  migration 0029): register → create multi-spec profile → list → rename-only PATCH leaves
  specializations/skills unchanged → change-specs-only leaves skills unchanged → empty / >5
  / unknown specialization all 400. UI is covered by svelte-check (0 errors) + a production
  build; the never-enabling-button dead-end is removed by the typeahead + the
  ≥1-specialization/≥1-skill canSubmit gate.
- [x] 7.3 Note the manual prod migration step (apply `0029` before deploying the binary).
