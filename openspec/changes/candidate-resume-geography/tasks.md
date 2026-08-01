## 1. Location dictionary integrity

- [x] 1.1 Add a guard test asserting every country code in `countryToRegion` resolves from at least one name in `nameToCountry` (currently fails for 25 codes)
- [x] 1.2 Add the 25 missing country names to `nameToCountry` (`ad ao bn ci cm hn kh la li ly mm mn mo mz ni ps rw sm sn sv tz ug ye zm zw`) — names only, never codes
- [x] 1.3 Add regression tests that the two-letter subdivision collisions still win (`LA`→Louisiana, `MN`→Minnesota, `MO`→Missouri) and that `San Pedro Sula, Honduras` now resolves to `hn`/`latam`

## 2. Candidate geography rule

- [x] 2.1 Write failing tests for `location.ParseResidence`: `Valencia, Spain` → `es`/`eu`/`Valencia`; empty input → empty
- [x] 2.2 Write failing tests that `global` is excluded via BOTH paths — the bare-remote fallback (`Remote (GMT+3)`) and the dictionary entry (`REMOTE · WORLDWIDE`, `anywhere`)
- [x] 2.3 Write failing tests that a real region without a country survives (`EU / Remote` → `eu`), that a place beside a remote marker survives, and that no work-mode value is returned
- [x] 2.4 Implement `Residence` and `ParseResidence` in `internal/location`; document why it is a distinct type rather than a flag on `Parse`
- [x] 2.5 Re-run the 100 production location strings through `ParseResidence` and record the country-coverage number before and after the dictionary fix

## 3. Schema

- [x] 3.1 Add migration `0068_user_resume_geography.sql` adding nullable `resume_countries`, `resume_regions`, `resume_cities` (`text[]`, no default) to `users`, with a comment stating the NULL-vs-`'{}'` distinction
- [x] 3.2 Update `SetUserResumeStructured` and `ClearUserResume` in `internal/db/queries/users.sql` to write and clear the three columns; add a `GetUserResumeGeography`-style read for the profile surface
- [x] 3.3 Run `make sqlc` and confirm `internal/db` regenerates cleanly

## 4. Write path

- [x] 4.1 Write failing tests in `internal/resume` that `SetStructured` persists the derived geography alongside the structure, under the same stamp
- [x] 4.2 Write a failing test that a superseded stamp writes neither the structure nor the geography (the monotonic guard covers both)
- [x] 4.3 Write a failing test that clearing the résumé clears the three columns
- [x] 4.4 Implement the derivation inside `resume.Store.SetStructured`; confirm both producers (upload path and `cmd/backfill-resume-structured`) inherit it without changes

## 5. Reconciler worker

- [x] 5.1 Write failing tests for the worker's selection rule: users with a current structure are processed, users with a superseded structure are skipped
- [x] 5.2 Write a failing idempotency test: a second run over unchanged data writes identical values
- [x] 5.3 Implement `cmd/backfill-resume-geo` with `--user` / `--dry-run`, requiring only `DATABASE_URL` (no LLM, no S3, no PII detector)

## 6. Read path

- [x] 6.1 Write failing tests for country precedence in `buildHardConstraintInputs`: asserted base wins; derived fills an unstated base; two derived countries yield none; neither source yields none
- [x] 6.2 Implement the precedence in `internal/handler/hardconstraint_inputs.go`
- [x] 6.3 Write a failing test that `GET /api/v1/me/profile` returns the derived geography in its own block, `null` when absent, and that a profile write cannot set it
- [x] 6.4 Implement the read-only derived-geography block on the profile response and regenerate the TS contract (`cmd/gen-contracts`)

## 7. Profile form

- [x] 7.1 Extract the profile→filters location seeding into a pure `.ts` module and write a failing test that a remote-only user's `base` does NOT seed the `countries`/`cities` facets
- [x] 7.2 Implement the seeding rule in `facetModel.ts` (`base` contributes only when the user accepts on-site or hybrid); confirm the existing mixed-mode scenario still passes
- [x] 7.3 Un-gate the "where you're based" control in `ProfileForm.svelte` — visible for every user, and reaching `buildLocation()` regardless of accepted work modes
- [x] 7.4 Pre-fill the base control from the derived geography when the user has stated no base; verify a stated base is never overwritten
- [ ] 7.5 Visually verify the profile form in a headless browser at a real viewport width (the work-format section no longer gates two sub-forms)

## 8. Documentation

- [x] 8.1 Update `internal/location/AGENTS.md` with the residence-vs-job-geography rule and the two-map guard
- [x] 8.2 Update `internal/resumeextract/AGENTS.md` (or `internal/resume`'s notes) to record that the structure write now also carries geography
- [x] 8.3 Note migration `0068` in the deploy-order documentation as apply-before-deploy

## 9. Production rollout

- [x] 9.1 Apply migration `0068` on production before deploying
- [x] 9.2 Run `cmd/backfill-resume-structured --dry-run`, review the 40 users, then run it for real
- [x] 9.3 Run `cmd/backfill-resume-geo --dry-run`, then run it for real
- [x] 9.4 Verify: produce the country distribution in one query, and confirm a second `backfill-resume-geo` run changes nothing
- [x] 9.5 Record the final coverage number (resolved / consciously empty) across all users with a stored CV
