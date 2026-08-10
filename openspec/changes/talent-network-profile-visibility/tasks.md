## 1. Data model

- [x] 1.1 Add migration: `talent_network_visibility` enum column on `users`
      (`off` / `public` / `anonymous`, default `off`, not null) plus a
      `talent_network_public_id uuid DEFAULT gen_random_uuid() NOT NULL
      UNIQUE` column for the opaque public URL. Do NOT touch `users.id` — this
      is a new, feature-scoped column, not a PK swap (see design.md decision
      and `hire-opaque-ids-uuid-swap` precedent for why a PK swap is the wrong
      tool here).
- [x] 1.2 Add sqlc queries: get/set `talent_network_visibility` for a user by
      `users.id` (owner-scoped), and a lookup by `talent_network_public_id`
      returning the fields the public page needs (visibility,
      resume_structured, user_profiles join). Run `make sqlc`.

## 2. Anonymous-mode projection logic

- [x] 2.1 Write the masking function that takes `resumeextract.Structured`
      and returns the anonymous-mode view: apply `.Professional()`, then
      replace the company field of every `experience` entry whose `End`
      reads as "not ended" (empty, or one of
      `present`/`current`/`now`/`ongoing`, case-insensitive — mirror
      `internal/experience/import_resume.go`'s `currentEndLabels`, which
      can't be imported directly due to the package direction) with a
      generic label. REVISED from "mask the newest entry" during task 2's
      review: `Structured.Experience` ordering is undocumented/unenforced,
      so this must be content-based, not positional. Cover: zero entries,
      no entry current, exactly one current, multiple concurrent current
      entries.
- [x] 2.2 Write the public-mode projection: `.Professional()` with name and
      photo retained (i.e. `Professional()` output plus name/photo, still
      no email/phone/links). `Structured` has no photo field — see
      `internal/resumeextract/visibility.go`; a later task composes a photo
      in from elsewhere (e.g. `users.photo_object_key`).

## 3. Owner-facing visibility setting

- [ ] 3.1 Extend the existing `me_profile` handler (or add a sibling) with an
      endpoint to read and update the caller's own
      `talent_network_visibility`. Cookie/key auth only, consistent with the
      rest of `me_profile.go`.
- [ ] 3.2 Handler test: get defaults to `off` for a user who never set it; set
      to `public`/`anonymous` persists and round-trips.

## 4. Public profile page (backend)

- [ ] 4.1 Add an unauthenticated route serving the profile by
      `talent_network_public_id`. `off` and not-found both return 404 with
      the same body shape.
- [ ] 4.2 Handler test: `public` mode response contains name/photo/experience/
      skills, no email/phone/links. `anonymous` mode response contains no
      name/photo, newest employer masked, older employers shown, no
      email/phone/links. Malformed/missing opaque id → 404.
- [ ] 4.3 Handler test: visibility enabled with empty/missing
      `resume_structured` renders 200 with empty sections, not an error.

## 5. Owner-facing toggle (frontend)

- [ ] 5.1 Add the Off/Public/Anonymous control to `web/src/routes/my/profile`,
      wired to the endpoint from 3.1. Show the resulting public URL when a
      non-off mode is selected.

## 6. Public profile page (frontend)

- [ ] 6.1 Add the public route rendering the page for `public` and
      `anonymous` modes, and a 404 state, consistent with the styling of
      other public routes (`companies/[slug]`, `jobs/[slug]`).

## 7. Verification

- [ ] 7.1 `go vet -tags=integration ./...` and `go test ./...` pass.
- [ ] 7.2 Manual check: toggle each mode as a test account, load the public
      URL in a logged-out browser, confirm rendered content matches the mode
      per design.md and specs/talent-network-profile/spec.md.
