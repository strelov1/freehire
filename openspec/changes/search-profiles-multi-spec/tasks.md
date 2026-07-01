## 1. Root-cause the inactive Create button

- [ ] 1.1 Reproduce the "Create profile never enables" dead-end (run the SPA signed-in, or
  drive it headless) and confirm the mechanism: skills are select-only from the facet pill
  wall, so an unfound skill leaves `skills.length === 0` and `canSubmit` false. Record the
  finding in the change notes (systematic-debugging Phase 1).

## 2. Data model (DB + sqlc)

- [ ] 2.1 Add `migrations/0029_search_profiles_specializations.sql`: add
  `specializations TEXT[]`, backfill `ARRAY[specialization]`, add CHECK
  `cardinality(specializations) BETWEEN 1 AND 5`, drop `specialization`. Include the
  inverse SQL as a comment for rollback.
- [ ] 2.2 Update `internal/db/queries/search_profiles.sql`: `CreateSearchProfile` inserts
  `specializations`; `UpdateSearchProfile` COALESCEs `specializations` (nullable arg).
- [ ] 2.3 Run `make sqlc` and commit regenerated `internal/db/*` (models, querier,
  search_profiles.sql.go).

## 3. Service layer (`internal/searchprofile`) — TDD

- [ ] 3.1 RED: extend `searchprofile_test.go` for `normalizeSpecializations` and
  Create/Update — valid set, dedupe, trim, unknown category → error, empty → error, >5 →
  error, nil-on-update → unchanged.
- [ ] 3.2 GREEN: add `normalizeSpecializations`, `ErrEmptySpecializations`,
  `ErrTooManySpecializations`; retire the single-value `validSpecialization`/
  `ErrInvalidSpecialization` semantics; change `Create`/`Update` signatures
  (`specialization string` → `specializations []string`); wire the new
  `CreateSearchProfileParams`/`UpdateSearchProfileParams` fields.
- [ ] 3.3 REFACTOR + re-run `go test ./internal/searchprofile/`; keep symmetry with
  `normalizeSkills`.

## 4. HTTP handler (`internal/handler/me_profiles.go`)

- [ ] 4.1 Change request/response shapes: `Specialization string` → `Specializations
  []string` in create/update bodies and `searchProfileResponse` (+ `toSearchProfileResponse`).
- [ ] 4.2 Map the new sentinels in `searchProfileError` (empty/too-many specializations →
  400); update the create/update handler calls to pass the slice.
- [ ] 4.3 Extend the handler integration tests (`//go:build integration`) for the new
  wire shape and validation statuses; run `go test -tags=integration ./internal/handler/`.

## 5. Frontend contracts & store

- [ ] 5.1 `web/src/lib/types.ts`: `SearchProfile.specialization: string` →
  `specializations: string[]`.
- [ ] 5.2 `web/src/lib/api.ts`: `createSearchProfile`/`updateSearchProfile` take/serialize
  `specializations`.
- [ ] 5.3 `web/src/lib/searchProfiles.svelte.ts`: `create`/`update` signatures use
  `specializations`.

## 6. Profile form UI (`SearchProfilesView.svelte`)

- [ ] 6.1 Specialization input → `SearchSelect` multi-select over `CATEGORY_OPTIONS`
  (array state + toggle, client-side cap of 5); `canSubmit` requires ≥1 specialization.
- [ ] 6.2 Skills input → `RemoteSearchSelect` whose `search(query)` filters the loaded
  skill distribution locally (dictionary-only, counts shown, chips removable); seed
  selected skills on edit so they stay visible/removable.
- [ ] 6.3 Profile list renders all specialization labels (via `categoryLabel`) per profile.

## 7. Verify & finish

- [ ] 7.1 `go build ./... && go vet ./...`; `cd web && npx svelte-check`.
- [ ] 7.2 Manually verify (signed-in, headless if needed): the form enables with name +
  specialization(s) + skill, the skill typeahead suggests/adds chips, multi-specialization
  round-trips through create → list → edit; confirm the dead-end from 1.1 is gone.
- [ ] 7.3 Note the manual prod migration step (apply `0029` before deploying the binary).
