## 1. Database

- [x] 1.1 Add migration `migrations/0161_mentors_company_optional.sql`:
      `ALTER TABLE mentors ALTER COLUMN company_slug DROP NOT NULL` — additive, no
      backfill (see design.md's Migration Plan). `node scripts/check-migrations.mjs`: 0
      issues (needed a `squawk-ignore ban-drop-not-null` — deliberate, audited in design.md).
- [x] 1.2 Ran `make sqlc`: regenerated `CompanySlug` as `pgtype.Text` on `Mentor`,
      `CreateMentorProfileParams`, and two unrelated-but-affected booking read rows
      (`GetMentorBookingRow`, `ListBookingsBySeekerRow` — neither has a Go call site that
      reads `.CompanySlug`, confirmed via grep, so no follow-up needed there). No query
      text changes, as expected.

## 2. Domain: `internal/engage/mentorship`

- [x] 2.1 Removed the "a company is required" check from `validateProfile`'s `creating`
      branch (`profile.go`) — a submission naming an unknown company is still refused via
      the existing FK-violation → `ErrCompanyNotFound` mapping; only the "must supply
      something" gate goes away.
- [x] 2.2 Updated `repository.go`'s `CreateProfile` to pass `CompanySlug` through
      `optionalText` (the same helper `DirectoryFilter.CompanySlug` already uses) instead
      of the raw string, matching the new `pgtype.Text` param type.
- [x] 2.3 Updated `repository.go`'s `profileFromRow` to read `CompanySlug` through
      `pgconv.TextString`, matching how `CompanyName` is already read on the same line.
- [x] 2.4 Checked `fakeRepo` (`fake_repo_test.go`): no change needed. Its
      `CreateProfile`/`profileFromRow`-equivalent and the directory filter predicate
      (`f.CompanySlug != "" && p.CompanySlug != f.CompanySlug`) already operate on the
      plain-string domain type and already treat an empty `CompanySlug` as "never matches
      a filter" by construction — the same pattern seniority already established.
- [x] 2.5 Unit tests: `TestSubmitProfileAcceptsNoCompany` (RED before 2.1-2.3, GREEN
      after — a profile submitted with an empty `CompanySlug` succeeds and is created with
      no company); removed the now-obsolete "no company" case from
      `TestSubmitProfileRefusesWhatCannotYieldASchedule`'s refusal table; confirmed the
      existing "unknown company" refusal test (`TestSubmitProfileReportsWhatTheDatabaseRefuses`)
      still passes unchanged; added `TestDirectoryIncludesACompanyLessMentorButNeverMatchesACompanyFilter`
      confirming a company-less mentor appears unfiltered and never matches a company
      filter (mirrors the existing seniority/no-reviews filter tests).

## 3. Backend: mentor-profile HTTP surface

- [x] 3.1 Confirmed (no code change needed, as design.md's Impact predicted):
      `mentorResponse`/`profileRequest`'s `CompanySlug`/`CompanyName` fields are plain,
      non-`omitempty` strings that already tolerate empty on the wire in both directions.
      Added `TestCompanyFieldsAreEmptyForACompanyLessMentor` as the regression test that
      didn't exist before.

## 4. Frontend: `MentorProfileEditor.svelte`

- [x] 4.1 Relabeled the create-mode company field to "Your company — optional" with a hint
      ("Leave blank if you're independent or your employer isn't listed"). No component
      logic change — `CompanyPicker` already emits `onSelect(null)` when cleared/unfilled.
- [x] 4.2 Fixed the existing-profile company display to use the new `companyLabel` helper
      (shows "Independent" when both `company_name`/`company_slug` are empty, unchanged
      otherwise).
- [x] 4.3 Extracted `companyLabel(companyName, companySlug)` into `mentorship.ts` (mirrors
      `seniorityLabel`'s pattern) and added `describe('companyLabel', ...)` in
      `mentorship.test.ts` (RED before the helper existed, GREEN after) covering all three
      cases: name present, slug-only fallback, and the "Independent" case. Reused
      verbatim in both `MentorsView.svelte` and `MentorProfileEditor.svelte`, so this one
      test suite covers both render sites.

## 5. Frontend: `MentorsView.svelte`

- [x] 5.1 Fixed the directory card to use `companyLabel(mentor.company_name,
      mentor.company_slug)` in place of the bare `mentor.company_name` — no dangling
      separator, "Independent" shown for a company-less mentor.
- [x] 5.2 Covered by the same `companyLabel` test suite added in task 4.3 — no separate
      component test needed since the helper is used verbatim in the template and this
      repo's convention keeps pure logic tested in `.ts` files rather than duplicating
      coverage at the markup layer.

## 6. Verification

- [x] 6.1 `gofmt -l .` clean, `go vet ./...` clean, `go test ./...` green except
      `cmd/billing-sync`'s `TestTheStoreProviderAloneKeepsTheWorkerRunning`, confirmed
      pre-existing and failing identically on a pristine `origin/main` checkout (unrelated
      to this change — a throwaway worktree at `origin/main` reproduced the same failure).
- [x] 6.2 `go vet -tags=integration ./...` clean — needed fixing several more direct
      `CreateMentorProfileParams{CompanySlug: "..."}`/`.CompanySlug` call sites in
      integration test files that the domain-layer survey didn't cover (it only checked
      non-test production code paths): `internal/platform/db/mentorship_integration_test.go`
      (multiple), `internal/engage/mentorship/repository_integration_test.go`,
      `internal/engage/mentorship/busysync/dbstore_integration_test.go`. Ran the full
      tagged integration suite for `internal/platform/db`, `internal/api/handler` and
      `internal/engage/mentorship` — all green.
- [x] 6.3 `node scripts/check-migrations.mjs`: 0 issues. `make sqlc` re-run: idempotent
      (only my own hand-written test edits show as modified, no generated-file diff).
      `golangci-lint run --new-from-merge-base=origin/main` (v2.12.2, matching CI's pin):
      0 issues.
- [x] 6.4 `svelte-check` (0 errors, 39 pre-existing unrelated warnings), `eslint` (clean),
      full frontend `vitest run` (163 files, 1905 passing). Design-system adoption:
      unaffected — the new markup reuses the exact plain `<p>` primitive already present
      on the same lines being edited, no new raw-HTML pattern introduced.
- [x] 6.5 Manual check (Playwright against a live `go run ./cmd/server` + `vite dev`, an
      isolated Postgres container on an alternate host port): submitted a mentor profile
      with no company (via curl, mirroring the API a real form submit hits) — created
      successfully, approved, confirmed it appears in the unfiltered `/mentors` directory
      as "Independent" (both the directory card and the owner's own profile view),
      confirmed filtering by `?company=acme` excludes it while including a second,
      company-having mentor. Confirmed submitting WITH a company still works unchanged,
      and submitting an unknown company is still refused with 404. Loaded the create form
      itself in a real browser and confirmed the relabeled "Your company — optional"
      field and its hint render correctly. Zero network errors throughout.
- [x] 6.6 `/code-review` pass on the full diff (rebased onto latest `origin/main` first —
      confirmed not stale). No competitor-name leak, no stale-base risk found. Fixed both
      Important findings: `MentorReviewView.svelte`'s two company render sites (the queue
      row, which also shows the raw slug in parens for a company-haver, and the approval
      preview panel) didn't use `companyLabel` and would have shown a dangling `" ()"` /
      blank segment for a company-less pending submission — fixed both, verified live via
      Playwright as a moderator (screenshot: "Aspiring Mentor · Independent", no dangling
      parens, in both the row and its preview). Added a paragraph to design.md's Migration
      Plan naming the code-rollback exposure (an old binary's non-nullable `CompanySlug`
      Go type would fail to scan a NULL row, breaking any listing query that includes a
      company-less mentor) that the review flagged as missing. Tightened design.md's
      Non-Goals wording per the review's Minor note, since it had claimed the moderation
      queue's display "need[ed] no new code" moments before the review found it did.
      Re-verified: `svelte-check` (0 errors), `eslint` (clean), full frontend `vitest run`
      (1905 passing).
