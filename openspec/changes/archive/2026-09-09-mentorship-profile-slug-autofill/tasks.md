## 1. Backend: derive-and-retry the slug on submission

- [x] 1.1 Add `maxSlugAttempts` constant to `internal/engage/mentorship/profile.go`
      (mirroring `accounts.maxUsernameAttempts`'s rationale) and import
      `internal/identity/username`.
- [x] 1.2 In `Service.SubmitProfile`, when `strings.TrimSpace(in.Slug)` is empty, derive
      the base via `username.Sanitize(strings.TrimSpace(in.DisplayName))` before running
      `validateProfile`, and track that the slug was derived (not caller-supplied).
- [x] 1.3 Loop the create attempt: derived slug first, then `username.Candidate(base, n)`
      for `n` up to `maxSlugAttempts` on `ErrSlugTaken`, but only when the slug was
      derived; an explicitly supplied slug still returns `ErrSlugTaken` on the first
      collision, unchanged.
- [x] 1.4 Update the doc comments on `ProfileInput.Slug` / `validateProfile` to describe
      the new blank-input behavior.

## 2. Backend tests (write first, per TDD)

- [x] 2.1 Table-driven test: blank slug + a display name with latin/digit characters →
      profile created with the sanitized slug.
- [x] 2.2 Test: blank slug whose derived form collides with an existing profile → created
      with `base-2`; a second collision → `base-3`.
- [x] 2.3 Test: blank slug + display name with no latin/digit characters (e.g. all-Cyrillic)
      → falls back to `"user"`, suffixed on collision like any other derived base.
- [x] 2.4 Test: non-empty, invalid-shape slug (e.g. containing `_`) is still refused with
      `ErrInvalidProfile`, not silently sanitized.
- [x] 2.5 Test: non-empty, valid slug that collides is still refused with `ErrSlugTaken`
      (no suffix retry for an explicit value).
- [x] 2.6 Run `go test ./internal/engage/mentorship/...` and confirm green.

## 3. Frontend: MentorProfileEditor.svelte

- [x] 3.1 Add a small local sanitizer mirroring `username.Sanitize`'s four rules
      (lowercase, drop disallowed chars, `.`→`-`, collapse/trim hyphens, truncate to 30),
      with a comment noting it must mirror the Go function it previews.
- [x] 3.2 Derive "Your URL" live from `form.name` while the user hasn't edited the URL
      field directly (a `slugTouched` flag flips true on direct edit); keep the field
      editable in both states.
- [x] 3.3 Add helper text under "Your URL": allowed characters, and that leaving it blank
      generates one from the name.
- [x] 3.4 Add a small inline validation hint when the field is non-empty and doesn't match
      the slug pattern (does not block typing or submission - the backend is still the
      authority).
- [x] 3.5 Replace the "Company slug" raw `<input>` with `<CompanyPicker onSelect={...}>`
      when `!profile` (creating); update `form.company_slug` (and keep the company name
      for display) from `onSelect`.
- [x] 3.6 When `profile` exists (edit mode), render the company name and the profile's
      slug/URL as static read-only text instead of disabled inputs.

## 4. Manual verification

- [x] 4.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`.
- [x] 4.2 Run the app (`make run` / `make up` + `web` dev server) and walk the create-profile
      form: leave URL blank → succeeds; type an invalid character → see the hint; search
      and pick a company via the picker; submit and confirm the profile page shows the
      derived URL and picked company; reload the edit form and confirm both fields render
      as read-only.

## 5. Code review follow-up

- [x] 5.1 **Critical fix**: `MentorProfileEditor.svelte`'s live preview was bound straight
      into `form.slug`, which `save()` submitted verbatim — so leaving "Your URL" untouched
      sent the preview as an EXPLICIT slug, never reaching `SubmitProfile`'s derive/retry
      path at all (two mentors named "Jane Doe" would 409 instead of the second getting
      `jane-doe-2`; a two-character name would fail the length check instead of falling
      back). Added `resolveMentorSlugForSubmit(slug, touched)` in `mentorSlugPreview.ts`
      (submits empty unless the field was deliberately touched) and wired it into `save()`.
- [x] 5.2 Regression tests for 5.1 in `mentorSlugPreview.test.ts`
      (`resolveMentorSlugForSubmit`'s three cases), written first (RED) against the missing
      function, then made to pass.
- [x] 5.3 Minor: `previewMentorSlug`'s truncation could leave a trailing hyphen the pattern
      rejects; added a post-truncate trim (mirroring the Go sanitizer's own) and a test.
- [x] 5.4 Re-verified live in the browser: two accounts both named "Jane Doe", both leaving
      "Your URL" untouched — first got `jane-doe`, second got `jane-doe-2` silently, no
      error either time (this was the broken path before 5.1).
