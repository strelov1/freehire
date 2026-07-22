## 1. Database & sqlc

- [ ] 1.1 Add migration `migrations/00NN_user_profile_excluded_skills.sql`: `ALTER TABLE user_profiles ADD COLUMN excluded_skills text[] NOT NULL DEFAULT '{}'::text[];` with the standard header noting manual prod apply (mirror `0010_user_profile_location.sql`).
- [ ] 1.2 Add `excluded_skills` to the `UpsertUserProfile` INSERT columns/values in `internal/db/queries/user_profiles.sql` (SELECT `*` already picks it up for GET).
- [ ] 1.3 Regenerate sqlc (`make sqlc`) and confirm `UpsertUserProfileParams` and the `UserProfile` model gain `ExcludedSkills`.

## 2. Backend domain — userprofile (TDD)

- [ ] 2.1 RED: add a test for `normalizeExcludedSkills` (trim/lower/dedup, empty allowed) in `internal/userprofile`; write the normalizer to green.
- [ ] 2.2 RED: add a `Service.Save` test asserting the overlap rule — a skill present in both `skills` and `excluded_skills` is dropped from `excluded_skills` (wanted wins), empty excluded set is valid. GREEN: implement the subtraction in `Save`.
- [ ] 2.3 Add `ExcludedSkills []string` to the `Profile` struct and thread it through the `Repository.Upsert` signature.
- [ ] 2.4 Map `ExcludedSkills` in `QueriesRepository.Upsert` (→ sqlc params) and in `profileFromRow` (`internal/userprofile/repository.go`).

## 3. Backend handler — me_profile

- [ ] 3.1 Add `ExcludedSkills` to `profileResponse` + `toProfileResponse` and to `saveProfileRequest`, and pass it into `Service.Save` (`internal/handler/me_profile.go`). Response key `excluded_skills`.
- [ ] 3.2 Add/extend a handler-level test (or integration test) covering PUT-then-GET round-trip of `excluded_skills`, including the overlap-drop case.

## 4. Frontend — types & API

- [ ] 4.1 Add `excluded_skills: string[]` to `UserProfile` in `web/src/lib/types.ts`.
- [ ] 4.2 Include `excluded_skills` in the `saveProfile` PUT body in `web/src/lib/api.ts`, and thread it through `ProfileStore.save(...)` in `web/src/lib/profile.svelte.ts`.

## 5. Frontend — apply-profile seeding (TDD)

- [ ] 5.1 RED: add a vitest for `filtersFromProfile` asserting `profile.excluded_skills` seeds the `skills` facet's `exclude` set (and `filtersToParams` emits `skills_exclude=…`). GREEN: seed the exclude side in `filtersFromProfile` (`web/src/lib/facetModel.ts`).

## 6. Frontend — profile form UI

- [ ] 6.1 Add a "Skills to avoid" control to `web/src/lib/components/ProfileForm.svelte` mirroring the existing skills control (same canonical-skill typeahead source): `excludedSkills` state, toggle handler, pre-seed from the loaded profile, and include it in the `submit()` save call. Exclude each control's current selections from the other's options.
- [ ] 6.2 Visual-verify the profile form (throwaway route + headless Chrome per repo convention): add/remove an excluded skill, save, reload, confirm persistence.

## 7. Verify

- [ ] 7.1 `go build ./... && go vet ./... && go test ./...` green; web `svelte-check` + vitest green.
- [ ] 7.2 End-to-end: save a profile with an excluded skill, open the jobs filter, click **Apply my profile**, and confirm the excluded skill lands in the `skills` exclude set and the committed URL carries `?skills_exclude=…`.
