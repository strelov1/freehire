## 1. Cap the skill sets

- [x] 1.1 Add failing tests to `internal/userprofile/userprofile_test.go`: a set past the cap returns `ErrTooManySkills` and does not reach the repository (for both the wanted and the avoided set); a set exactly at the cap is persisted whole; an over-long value is dropped while its valid siblings survive.
- [x] 1.2 Add `maxSkills`, `maxSkillLen` and `ErrTooManySkills`; rename `normalizeExcludedSkills` to `normalizeSkillList` (it is the shared core of both sets and now returns an error, so the old name no longer described it) and apply both bounds there.
- [x] 1.3 Map `ErrTooManySkills` to `400` in `profileError` (`internal/handler/me_profile.go`).
- [x] 1.4 Add `migrations/0056_user_profile_skills_cap.sql` — both constraints `NOT VALID`, so the bound guards every future write without a validation scan or a lengthy lock.
- [x] 1.5 Run `go test ./internal/userprofile/ ./internal/handler/`, `go build ./...`, `go vet ./...`, `gofmt -l`.

## 2. Raise the bound to what real profiles hold

- [x] 2.1 Measure prod: 131 profiles, 89% at 50 skills or fewer, maximum 90. A ceiling of 100 leaves a live user 10 of headroom while CV autofill unions more skills into the form.
- [x] 2.2 Change the failing tests to the new bound: 200 accepted whole, 201 rejected.
- [x] 2.3 Raise `maxSkills` to 200 and rewrite its comment to justify the number by the measurement rather than by symmetry with the coverage endpoint's 100. Update the `400` message.
- [x] 2.4 Add `migrations/0057_user_profile_skills_cap_200.sql` — drop and re-add both constraints (no ALTER CONSTRAINT exists for a CHECK expression), still `NOT VALID`.
- [x] 2.5 Verify the migration sequence on a scratch Postgres: every file applies in order, both constraints end at `<= 200` and `convalidated = f`, 200 skills accepted, 201 rejected.

## 3. Rollout

- [x] 3.1 `0056` applied to prod by `release.sh` during the release of the change that introduced it (`migrate: 72 file(s) on disk, 0 baselined, 1 applied`).
- [ ] 3.2 `0057` applies on the next release — `release.sh` runs the migration runner before starting the new colour, and widening a bound cannot fail on existing data.
