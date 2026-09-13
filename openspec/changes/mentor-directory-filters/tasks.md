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

- [x] 3.1 Add `Seniority string` to `mentorResponse` (`json:"seniority,omitempty"`) and
      set it in `toMentorResponse` (`internal/api/handler/mentorship.go`) — present on
      every view (public, owner, moderator) since they all build on top of it.
- [x] 3.2 Add `Seniority string` to `profileRequest` and thread it into
      `ProfileInput.Seniority` in `toInput` (`internal/api/handler/mentorship_write.go`).
- [x] 3.3 In `ListMentors`, read `q`, `seniority` and `no_reviews` from the query into
      `DirectoryFilter.Query`/`Seniority`/`NoReviewsOnly` (`c.QueryBool("no_reviews")`).
      Added all three to `knownMentorParams` (`internal/api/handler/mentorship.go`).
- [x] 3.4 Unit tests: `TestSeniorityIsOnEveryView` (all three response views carry it),
      `TestNewDirectoryFiltersAreKnownParams` (the three new params are not reported as
      ignored), `TestProfileRequestToInputCarriesSeniority` (this file had no
      `toInput` field-mapping test at all before this change — added the one
      assertion needed to catch a field landing in the request struct but never
      reaching `ProfileInput`).

## 4. Frontend: filter plumbing

- [x] 4.1 Extend `MentorFilters`/`MENTOR_FILTER_KEYS` in `web/src/lib/mentorship.ts`
      with `q`, `seniority` (folded into the existing single-loop
      `MENTOR_FILTER_KEYS`) and `noReviews: boolean` (kept OUT of that loop — handled by
      two explicit lines in `mentorFiltersToParams`/`mentorFiltersFromParams`, since a
      flag's "present means true" shape does not fit the "empty string means unfiltered"
      loop the five string filters share).
- [x] 4.2 Extend `MentorFilterOptions`/`mentorFilterOptions` with a `seniorities: string[]`
      list, derived from the unfiltered directory response exactly as `topics`/
      `languages` already are.
- [x] 4.3 Add `seniority?: string` to the `Mentor` type and `seniority: string` to
      `MentorProfileInput` (`web/src/lib/types.ts`). Also fixed the three call sites
      type-checking caught as a result (`profileInputFromProfile`, `MentorBlock.svelte`'s
      hand-rolled filter literal, `MentorProfileEditor.svelte`'s `blank()`) — part of
      task 6.2's own scope, done here since the type change made them fail to compile.
- [x] 4.4 Unit tests (`mentorship.test.ts`): round-trip the three new keys through params,
      `no_reviews` as a flag (absent vs `=1`, never `=false`), and `mentorFilterOptions`
      derives `seniorities` from the unfiltered mentor list the same way it derives
      `topics`.

## 5. Frontend: `MentorsView.svelte`

- [x] 5.1 Add a text `<input>` for `q`, alongside the three existing `<select>`s, using
      the same `apply({ ...filters, q: e.currentTarget.value })` pattern. Used
      `onchange` (fires on blur/Enter), not `oninput` — matching the existing controls'
      one-navigation-per-change cost, since firing `apply`'s `goto` on every keystroke
      would be a materially different cost than on every dropdown selection.
- [x] 5.2 Add a seniority `<select>` mirroring the Company/Topic/Language ones exactly
      (including `withSelected` for a filter value the current options no longer carry).
      Also added a seniority `Badge` to each mentor card (outline variant, beside the
      topic badges) — filtering by an attribute the card never shows would be a
      confusing feature to use.
- [x] 5.3 Add a "No reviews yet" checkbox toggle wired to `filters.noReviews`.
- [x] 5.4 Extended the `active` derived flag (was `company || topic || language`) to
      include all six filters, so "Clear filters" appears whenever any is set.
- [x] 5.5 Verified via `svelte-check` (0 errors), `eslint` (clean), full frontend
      `vitest run` (1888 passing), design-system adoption ratchet (unchanged).

## 6. Frontend: `MentorProfileEditor.svelte`

- [x] 6.1 Add an optional seniority `<select>` to the create/edit form (plain `<select>`,
      matching this form's existing convention), with an explicit "Prefer not to say"
      option. Options come from `SENIORITY_VALUES` (`$lib/generated/contracts`, already
      generated from the same Go `vocab.SeniorityValues` this change's backend half
      validates against) labelled via a new `seniorityLabel` helper in `mentorship.ts`
      (reuses the existing `SENIORITY_LABELS` map from `$lib/labels`, the same one job
      filters already use, rather than a second label set).
- [x] 6.2 `blank()`'s defaults and `profileInputFromProfile` both carry `seniority`
      (empty string default, read back from an existing profile on edit) — done in task
      4.3 already, once the type change forced it.
- [x] 6.3 Verified via `svelte-check` (0 errors), `eslint` (clean), full frontend
      `vitest run` (1890 passing).

## 7. Verification

- [x] 7.1 `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` all green.
- [x] 7.2 `go vet -tags=integration ./...` clean. Ran the full tagged integration suite
      for `internal/platform/db`, `internal/api/handler` and `internal/engage/mentorship`
      (the `.sql` query signatures changed) — all green. Layering guard
      (`-tags=integration,llmlive ./internal/platform/arch/...`) also green.
- [x] 7.3 `node scripts/check-migrations.mjs`: 0 issues on the new migration. `make sqlc`
      re-run: idempotent, no diff. `golangci-lint run --new-from-merge-base=origin/main`:
      0 issues.
- [x] 7.4 `svelte-check` (0 errors), `eslint` (clean), full frontend `vitest run` (1890
      passing), design-system adoption ratchet — all clean/unchanged.
- [x] 7.5 Manual check via the `run` skill (Playwright against a live `go run ./cmd/server`
      + `vite dev`): created a mentor profile with `seniority=senior`, confirmed the
      Search/Seniority/"No reviews yet" controls render on `/mentors`, the "Senior" badge
      shows on the card, filtering by `seniority=senior` includes the mentor and
      `seniority=junior` excludes it (empty-state message, `withSelected` correctly shows
      "junior" even though no mentor currently has it), the search box narrows by a
      name substring and excludes on a non-match, "no reviews yet" includes a mentor with
      zero reviews, and the Seniority `<select>` on `/my/mentorship/profile` shows the
      saved value and round-trips a change (`senior` → `lead`) across a page reload. Zero
      network errors on the mentor/mentorship endpoints throughout. (Needed manually
      marking the test account's `onboarding_completed_at` via SQL first — the SPA
      redirects an unfinished-onboarding account away from every other route.)
- [x] 7.6 `/code-review` pass on the full diff. Found the branch was stale against
      `origin/main` by two merged PRs (#2766, #2767) — rebased, which removed the
      OAuth-callback-dedup files from this change's diff entirely (they had shown up only
      as a stale-base artifact). Fixed both Important findings with regression tests:
      `seniorityLabel` in `mentorship.ts` now reuses `labels.ts`'s `titleCase` instead of a
      second hand-rolled fallback (matching `insights.ts`'s own `seniorityLabel`); the
      directory's seniority filter options are now ordered by `SENIORITY_VALUES` (career
      order) instead of alphabetically, with the covering test strengthened to use
      `lead`/`middle` so an alpha-sort regression would fail it. Also fixed both Minor
      findings: `Seniority` is now trimmed in `normaliseProfile` and validated on the
      trimmed value (new `TestSubmitProfileTrimsSeniority`, RED before the fix), and a
      misleading subtest name (`"by free-text query matching the name"` → `"...the
      headline"`, since the assertion only ever matched via the headline). Confirmed no
      competitor name/domain anywhere in the diff or OpenSpec docs. Re-ran the full suite
      after fixes: `gofmt -l .` clean, `go vet ./...` and `go vet -tags=integration ./...`
      clean, `go test ./...` (220 packages, all passing under `CGO_ENABLED=0` — a local
      macOS SDK/clang linker issue blocks `CGO_ENABLED=1` builds of several `cmd/`
      binaries, confirmed pre-existing and unrelated to this change), tagged integration
      suites for `internal/platform/db`, `internal/api/handler` and
      `internal/engage/mentorship` all green, layering guard green, `svelte-check` (0
      errors), `eslint` (clean), full frontend `vitest run` (1890 passing).
