## 1. Database

- [x] 1.1 Add migration `0152_mentors_show_photo.sql`: `mentors.show_photo boolean NOT
      NULL DEFAULT false`
- [x] 1.2 Update `CreateMentorProfile` and `UpdateMentorProfile` in
      `internal/platform/db/queries/mentorship.sql` to read/write `show_photo`; run
      `make sqlc`

## 2. Domain: `internal/engage/mentorship`

- [x] 2.1 Add `ShowPhoto bool` to `Profile` and `ProfileInput` (`profile.go`)
- [x] 2.2 Wire `ShowPhoto` through `QueriesRepository.CreateProfile`/`UpdateProfile`
      params and `profileFromRow` (`repository.go`)
- [x] 2.3 Unit test: create/update round-trips `show_photo` both ways (default false,
      explicit true), matching the `mentor-profile` spec's new requirement scenarios

## 3. Backend: mentor-profile HTTP surface

- [x] 3.1 Add `show_photo` to `profileRequest`/`toInput` and to `mentorResponse`/
      `toMentorResponse`/`toOwnMentorResponse`/`toModeratorMentorResponse`
      (`internal/api/handler/mentorship_write.go`, `internal/api/handler/mentorship.go`)
      — present on directory, profile-read and owner/moderator views alike
- [x] 3.2 Add `GET /mentors/:slug/photo` to `registerPublic`: resolve via the same
      lookup `GetMentor` uses, require `status == approved && !paused && ShowPhoto`,
      then serve `headshot.Store.Get(ctx, profile.UserID)`; map every non-serving
      reason (no photo, not approved/paused, opted out, storage unconfigured) to the
      same 404, per the spec's "same 404" requirement
- [x] 3.3 Wire a `headshot.Store` dependency into `mentorshipHandlers`
      (`newMentorshipHandlers`, `internal/api/handler/handler.go`)
- [x] 3.4 Unit tests for the photo route: approved+opted-in+has-photo (200, image
      bytes, `image/jpeg` content type), approved+opted-in+no-photo (404),
      approved+opted-out (404) regardless of stored photo, pending/paused/withdrawn
      (404) regardless of opt-in, storage unconfigured (404). The pending/paused/
      withdrawn case is exercised at the `mentorPhoto`/`GetMentorPhoto` seam by
      construction — it reuses `PublicProfile`'s existing predicate rather than a
      second copy, and that predicate's own scenarios are already covered by
      `internal/platform/db`'s mentorship integration tests

## 4. Backend: mentor-profile-prefill suggestions

- [x] 4.1 Define narrow, single-method `mentorSuggestion*` interfaces (mirroring
      `structuredResumeReader` in `me_profile.go`) covering: résumé (name, headline,
      bio, languages), user profile (`Specializations`), account (`Timezone`),
      experience bank (`ListEmployments`, filtered in the aggregator to
      `Kind == job && Current`), and `db.Queries.CompanyExists`. (Split into five
      one-method interfaces rather than one `mentorSuggestionSources` — each is
      trivially satisfied by the existing concrete service with no adapter, and each is
      faked with one line in the test.) **Corrected during the manual `run`-skill
      verification below**: the résumé interface first exposed `Structured` +
      `CandidateOwned` (mirroring `structuredCV`'s own composition), but
      `Owned.ApplyBody` only ever merges the five BODY fields (headline/summary/
      languages/certifications/education) — never identity (`FullName` included). A
      candidate who set their name only via the owned-contacts overlay, with no CV
      upload, would have gotten no name suggestion at all. Switched to
      `StructureForSeed` — the same identity-plus-body composition `cv_seed.go`
      already uses to seed a new document from a candidate's data — which is the
      actually-correct primitive for "seed a new form," not `structuredCV`'s.
- [x] 4.2 Implement the composition in a new
      `internal/api/handler/mentorship_suggestions.go`: each field independently
      best-effort (a failing or absent source omits its field, never fails the
      request); company suggestion included only when
      `normalize.CompanySlug(currentEmployer)` exactly matches via `CompanyExists`
- [x] 4.3 Add `GET /me/mentorship/profile/suggestions` (auth required, `mw.key`);
      response omits absent fields rather than emitting empty strings/arrays for them.
      Reads only résumé/user-profile/account/experience-bank, never the mentor profile
      itself, so it answers the same way whether or not the caller already has one — see
      the corrected `mentor-profile-prefill` spec (the original wording gated the
      endpoint on "no profile yet", which contradicted its own no-side-effect scenario;
      fixed during implementation, both proposal.md and the spec updated)
- [x] 4.4 Unit tests, table-driven: full data → all fields present; no data → success
      with every field absent; unmatched employer → company field absent. (The "caller
      already has a profile" case needs no separate test: the aggregator has no
      dependency on the mentor profile at all, so it structurally cannot see or be
      affected by one.)

## 5. Frontend: create-form prefill

- [x] 5.1 Add a `MentorProfileSuggestions` type and an
      `api.mentorProfileSuggestions()` call (`web/src/lib/types.ts`,
      `web/src/lib/api.ts`)
- [x] 5.2 In `MentorProfileEditor.svelte`, when `profile === null`, fetch suggestions
      once on mount and seed `blank()` from them; each field stays independently
      editable afterward, and a failed fetch falls back to today's empty defaults
      (never blocks rendering the form). The merge itself is a pure, unit-tested
      function (`seedFormFromSuggestions` in `mentorship.ts`) — the component only
      wires the fetch and applies the result, following this codebase's convention of
      keeping testable logic out of `.svelte` files
- [x] 5.3 Add the `show_photo` opt-in checkbox to the form: default unchecked for a
      new profile, reflects the stored value when editing an existing one, with copy
      stating it publishes the account's existing CV photo

## 6. Frontend: public avatar

- [x] 6.1 Add `show_photo: boolean` to the `Mentor` type (`web/src/lib/types.ts`)
- [x] 6.2 Render a circular avatar `<img src="/api/v1/mentors/{slug}/photo">` in
      `MentorsView.svelte` (directory card) and
      `web/src/routes/mentors/[slug]/+page.svelte` (profile page) only when
      `mentor.show_photo` is true; `onerror`-hide as a backstop so a 404 never shows a
      broken-image icon

## 7. Verification

- [x] 7.1 `gofmt -w`, `go vet ./...`, `go test ./...` on touched Go packages;
      `golangci-lint run` for the layering guard (composition stays in
      `internal/api/handler`, `internal/engage/mentorship` gains no new imports beyond
      the `show_photo` field). Also ran, all clean: `go vet -tags=integration ./...`,
      the layering test with `-tags=integration,llmlive`, `golangci-lint run
      --new-from-rev=origin/main ./...` (0 new issues), `deadcode -test
      -tags=integration,llmlive ./...` (nothing new), `node scripts/check-migrations.mjs`
      on the new migration (0 issues), `make sqlc` (idempotent, no drift), `svelte-check`
      (0 errors), `eslint` on every touched frontend file (clean), and the full frontend
      `vitest run` (1781 passed)
- [x] 7.2 Manual check via the `run` skill, driven live in a real Chromium browser
      (Playwright) against `make up`-equivalent local Postgres/MinIO + `go run
      ./cmd/server` + `vite dev`: registered a test account, seeded résumé contacts
      (`PUT /me/resume/contacts`), a user-profile specialization (`PUT /me/profile`),
      an account timezone (`PATCH /me/timezone`), and a current job "Acme Inc" in the
      experience bank (`POST /me/experience/employments`), with a matching `acme`
      company row inserted for the exact-match check.
      - Visited `/my/mentorship/profile` with no existing profile: the create form
        came back seeded with name "Jane Doe", headline, bio, languages, topic
        "backend", timezone "Europe/Berlin" and company slug "acme" — confirmed by
        DOM inspection, screenshot, and `GET .../suggestions` called directly
        (returned all seven fields). "Your URL" and "Meeting link" correctly stayed
        blank (never suggested); filled both and submitted — profile created
        `pending`, form switched to edit mode with the same values retained.
      - Approved the profile (`status='approved'`) directly in Postgres (no
        moderator UI in this change's scope), checked `show_photo` in the edit form
        and saved, then visited the public `/mentors` directory and
        `/mentors/jane-doe-test`: both rendered the circular avatar
        (`GET /api/v1/mentors/jane-doe-test/photo` → 200), zero console errors.
      - Unchecked `show_photo` and saved: the avatar disappeared from the public
        profile page with zero `<img src*="/photo">` elements and zero console
        errors — no broken-image icon.
      - Found and fixed a real bug in this pass (see task 4.1's note): the
        suggestions aggregator originally used `Structured`+`CandidateOwned`, which
        never merges identity fields, so `name` would have stayed unsuggested for a
        candidate who set it only via the contacts overlay. Fixed before this
        check's first successful run.
      - Two infrastructure hiccups along the way, neither a code defect: OrbStack's
        Docker daemon crashed mid-setup (waited for the user to restart it, then
        recreated the compose containers against the same volumes); and the Vite
        dev server's proxy briefly 502'd after that restart until relaunched fresh.
