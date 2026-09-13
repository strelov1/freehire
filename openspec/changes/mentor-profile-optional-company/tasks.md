## 1. Database

- [ ] 1.1 Add migration `migrations/0161_mentors_company_optional.sql`:
      `ALTER TABLE mentors ALTER COLUMN company_slug DROP NOT NULL` — additive, no
      backfill (see design.md's Migration Plan). Run `node scripts/check-migrations.mjs`.
- [ ] 1.2 Run `make sqlc` to regenerate `CompanySlug` as `pgtype.Text` on every affected
      generated struct (`Mentor`, `CreateMentorProfileParams`). Confirm no query text
      changes are needed (every read already `LEFT JOIN`s `companies`; the `company`
      filter is already `sqlc.narg`-based and NULL-safe) — this task is regen-only.

## 2. Domain: `internal/engage/mentorship`

- [ ] 2.1 Remove the "a company is required" check from `validateProfile`'s `creating`
      branch (`profile.go`) — a submission naming an unknown company is still refused via
      the existing FK-violation → `ErrCompanyNotFound` mapping; only the "must supply
      something" gate goes away.
- [ ] 2.2 Update `repository.go`'s `CreateProfile` to pass `CompanySlug` through
      `optionalText` (the same helper `DirectoryFilter.CompanySlug` already uses) instead
      of the raw string, matching the new `pgtype.Text` param type.
- [ ] 2.3 Update `repository.go`'s `profileFromRow` to read `CompanySlug` through
      `pgconv.TextString`, matching how `CompanyName` is already read on the same line.
- [ ] 2.4 Update `fakeRepo` (`fake_repo_test.go`) if it does any company-slug-specific
      handling that assumed a non-empty value (check `CreateProfile`, `ListPublishedProfiles`
      filter logic).
- [ ] 2.5 Unit tests: `TestSubmitProfileAcceptsNoCompany` (a profile submitted with an
      empty `CompanySlug` succeeds and is created with no company); confirm the existing
      "unknown company" refusal test still passes unchanged; a fake-repo `Directory` test
      confirming a company-less mentor appears unfiltered and never matches a company
      filter (mirrors the existing seniority/no-reviews filter tests).

## 3. Backend: mentor-profile HTTP surface

- [ ] 3.1 Confirm (no code change expected per design.md's Impact) that
      `mentorResponse`/`profileRequest`'s `CompanySlug`/`CompanyName` fields already
      tolerate an empty string on the wire in both directions — add a regression test if
      none already covers a company-less profile round-tripping through `toMentorResponse`.

## 4. Frontend: `MentorProfileEditor.svelte`

- [ ] 4.1 Relabel the create-mode company field to "Your company — optional" with a hint
      ("Leave blank if you're independent or your employer isn't listed"). No component
      logic change — `CompanyPicker` already emits `onSelect(null)` when cleared/unfilled.
- [ ] 4.2 Fix the existing-profile company display (`{profile.company_name ||
      profile.company_slug}`) to show "Independent" when both are empty, instead of a
      blank paragraph.
- [ ] 4.3 Unit/component test (or a `mentorship.ts` helper + test, if the label logic is
      extracted) confirming the "Independent" fallback renders for a company-less
      profile and the existing behavior is unchanged for one with a company.

## 5. Frontend: `MentorsView.svelte`

- [ ] 5.1 Fix the directory card's `{mentor.headline} · {mentor.company_name}` to omit
      the dangling separator and show "Independent" in place of the company segment when
      `company_name` is empty.
- [ ] 5.2 Unit test confirming the card's rendered text for a company-less mentor.

## 6. Verification

- [ ] 6.1 `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` all green.
- [ ] 6.2 `go vet -tags=integration ./...` clean. Run the full tagged integration suite for
      `internal/platform/db`, `internal/api/handler` and `internal/engage/mentorship` (the
      `.sql` query signatures and generated types changed).
- [ ] 6.3 `node scripts/check-migrations.mjs`: 0 issues on the new migration. `make sqlc`
      re-run: idempotent, no diff. `golangci-lint run --new-from-merge-base=origin/main`:
      0 issues.
- [ ] 6.4 `svelte-check` (0 errors), `eslint` (clean), full frontend `vitest run`, design-
      system adoption ratchet — all clean/unchanged.
- [ ] 6.5 Manual check via the `run` skill: submit a mentor profile with no company via
      the UI, confirm it's created, appears in the unfiltered directory as "Independent",
      never appears when filtering by any company, and does not show up as a mentorship
      entry point on any vacancy or company page. Also confirm submitting WITH a company
      still works exactly as before.
- [ ] 6.6 `/code-review` pass on the full diff; fix Critical + Important findings with a
      regression test each.
