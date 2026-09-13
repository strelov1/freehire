## 1. Database

- [x] 1.1 Add migration: `ALTER TABLE mentors ADD COLUMN seniority text NOT NULL DEFAULT
      ''` (additive, no backfill — see design.md's Migration Plan). Check the latest
      filed migration number under `migrations/` and file the next one. Filed as
      `0160_mentors_seniority.sql`.
- [x] 1.2 Update `CreateMentorProfile` and `UpdateMentorProfile`
      (`internal/platform/db/queries/mentorship.sql`) to read/write `seniority`. Added
      the three new predicates to `ListPublishedMentors`:
      `(sqlc.narg(query)::text IS NULL OR m.display_name ILIKE '%' || sqlc.narg(query) ||
      '%' OR m.headline ILIKE '%' || sqlc.narg(query) || '%')`,
      `(sqlc.narg(seniority)::text IS NULL OR m.seniority = sqlc.narg(seniority)::text)`,
      `(NOT sqlc.arg(no_reviews_only)::bool OR COALESCE(r.rating_count, 0) = 0)` —
      matching `companies.sql`'s own unescaped-ILIKE search convention (see design.md).
      Ran `make sqlc`.

## 2. Domain: `internal/engage/mentorship`

- [x] 2.1 Add `Seniority string` to `Profile` and `ProfileInput` (`profile.go`).
- [x] 2.2 Add `Query`, `Seniority`, `NoReviewsOnly bool` to `DirectoryFilter`
      (`profile.go`), each following the struct's existing "empty means unfiltered"
      doc comment.
- [x] 2.3 In `validateProfile`, refuse a non-empty `Seniority` outside
      `vocab.SeniorityValues` via `slices.Contains`, mirroring
      `processreport.ValidKind`'s exact shape. An empty value is always valid (optional
      field).
- [x] 2.4 Wire `Seniority` through `QueriesRepository.CreateProfile`/`UpdateProfile`
      params and `profileFromRow` (`repository.go`). Wire `Query`/`Seniority`/
      `NoReviewsOnly` through `ListPublishedProfiles`'s call into
      `ListPublishedMentorsParams` (`optionalText` for the two string filters, the bool
      passed straight through).
- [x] 2.5 Unit tests: `validateProfile` refuses an unrecognised seniority value and
      accepts every value in `vocab.SeniorityValues` plus the empty string
      (table-driven). Fake-repository test for `Directory` threading the three new
      filter fields into what the fake repo receives, mirroring how `company`/`topic`/
      `language` are already asserted. Also updated `fakeRepo.CreateProfile`/
      `UpdateProfile` to carry `Seniority` through — a gap the RED test caught before
      the SQL-backed path could have hidden it.

## 3. Backend: mentor-profile HTTP surface

- [ ] 3.1 Add `Seniority string` to `mentorResponse` (`json:"seniority,omitempty"`) and
      set it in `toMentorResponse` (`internal/api/handler/mentorship.go`) — present on
      every view (public, owner, moderator) since they all build on top of it.
- [ ] 3.2 Add `Seniority string` to `profileRequest` and thread it into
      `ProfileInput.Seniority` in `toInput` (`internal/api/handler/mentorship_write.go`).
- [ ] 3.3 In `ListMentors`, read `q`, `seniority` and `no_reviews` from the query into
      `DirectoryFilter.Query`/`Seniority`/`NoReviewsOnly` (the bool: `c.QueryBool("no_reviews")`
      or equivalent truthy check). Add all three to `knownMentorParams`
      (`internal/api/handler/mentorship.go`).
- [ ] 3.4 Unit tests: `ListMentors` reports an unlisted param in `meta.ignored_params`
      still works with the three new ones added to the known set (regression — the
      existing test for this must keep passing with the vocabulary extended). A
      dedicated test that `q`/`seniority`/`no_reviews` are NOT reported as ignored.
      Table-driven test for `profileRequest.toInput` carrying `Seniority` through
      unchanged (mirrors existing field-mapping tests in this file, if any exist —
      otherwise add the one field-mapping assertion this file is missing).

## 4. Frontend: filter plumbing

- [ ] 4.1 Extend `MentorFilters`/`MENTOR_FILTER_KEYS` in `web/src/lib/mentorship.ts`
      with `q`, `seniority`, `noReviews` (serialized as `no_reviews`, matching the
      backend param name — `MENTOR_FILTER_KEYS` entries and their wire keys may need to
      stop being the same string if `noReviews`'s camelCase key differs from
      `no_reviews`'s wire form; check whether `mentorFiltersToParams`/
      `mentorFiltersFromParams`'s single loop still fits or needs a small key-mapping
      table, and keep whichever is less code).
- [ ] 4.2 Extend `MentorFilterOptions`/`mentorFilterOptions` with a `seniorities: string[]`
      list, derived from the unfiltered directory response exactly as `topics`/
      `languages` already are.
- [ ] 4.3 Add `seniority?: string` to the `Mentor` type and `seniority: string` to
      `MentorProfileInput` (`web/src/lib/types.ts`).
- [ ] 4.4 Unit tests (`mentorship.test.ts` or wherever the existing filter-serialization
      tests for `mentorFiltersToParams`/`mentorFiltersFromParams`/`mentorFilterOptions`
      live): round-trip the three new keys through params, and `mentorFilterOptions`
      derives `seniorities` from the unfiltered mentor list the same way it derives
      `topics`.

## 5. Frontend: `MentorsView.svelte`

- [ ] 5.1 Add a text `<input>` for `q`, alongside the three existing `<select>`s, using
      the same `apply({ ...filters, q: e.currentTarget.value })` pattern — consider
      whether free text needs a debounce the existing selects don't (they fire on
      `onchange`, not on every keystroke); decide based on whether `apply`'s
      `goto`-per-change is cheap enough here too (the directory is small and
      hand-onboarded per this file's own existing comment) or whether `oninput` would
      fire a navigation per keystroke and needs `onchange` instead.
- [ ] 5.2 Add a seniority `<select>` mirroring the Company/Topic/Language ones exactly
      (including `withSelected` for a filter value the current options no longer carry).
- [ ] 5.3 Add a "No reviews yet" checkbox toggle wired to `filters.noReviews`.
- [ ] 5.4 Extend the `active` derived flag (currently `company || topic || language`) to
      include the three new filters, so "Clear filters" appears whenever any of the six
      is set.
- [ ] 5.5 Verify via `svelte-check`, `eslint`, full frontend `vitest run`, design-system
      adoption ratchet.

## 6. Frontend: `MentorProfileEditor.svelte`

- [ ] 6.1 Add an optional seniority `<select>` to the create/edit form (plain `<select>`,
      matching this form's existing convention — no design-system dropdown component is
      used here today), with an explicit "Prefer not to say" / blank option since the
      field is optional.
- [ ] 6.2 `blank()`'s defaults and `profileInputFromProfile` both carry `seniority`
      (empty string default, read back from an existing profile on edit — the same
      whole-object-save reasoning `show_photo`/session params already follow in this
      file, so editing one field never silently resets seniority to blank).
- [ ] 6.3 Verify via `svelte-check`, `eslint`, full frontend `vitest run`.

## 7. Verification

- [ ] 7.1 `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` all green.
- [ ] 7.2 `go vet -tags=integration ./...` clean. Run the full tagged integration suite
      for `internal/platform/db` and `internal/api/handler` (the `.sql` query signatures
      changed).
- [ ] 7.3 `node scripts/check-migrations.mjs`: 0 issues on the new migration. `make sqlc`
      re-run: idempotent, no diff.
- [ ] 7.4 `svelte-check` (0 errors), `eslint` (clean), full frontend `vitest run`,
      design-system adoption ratchet — all clean/unchanged.
- [ ] 7.5 Manual check via the `run` skill: create a profile with a seniority level,
      confirm it appears on the directory card/filter and round-trips through an edit;
      confirm the search box narrows by name/headline; confirm "no reviews yet" excludes
      a mentor with at least one review.
- [ ] 7.6 `/code-review` pass on the full diff; fix Critical + Important findings with a
      regression test each.
