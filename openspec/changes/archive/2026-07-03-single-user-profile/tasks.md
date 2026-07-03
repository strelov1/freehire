## 1. Database schema & generated access

- [x] 1.1 Add migration `migrations/0043_collapse_search_profiles.sql` (0042 is the latest existing): in a transaction, delete all but each user's most-recently-updated row, drop the `name` column and its unique index, rename `search_profiles` → `user_profiles`, and make `user_id` the primary key (`UNIQUE (user_id)` invariant)
- [x] 1.2 Rewrite `internal/db/queries/search_profiles.sql` → `user_profiles.sql` as singleton queries: `GetUserProfile` (by user_id), `UpsertUserProfile` (insert … on conflict (user_id) do update), `DeleteUserProfile`; remove list/create-by-id/update-by-id queries
- [x] 1.3 Run `make sqlc` and commit regenerated `internal/db`

## 2. Backend singleton profile API

- [x] 2.1 Rename package `internal/searchprofile` → `internal/userprofile`; collapse `Create`/`Update` into a single `Save`/`Upsert` (validate specializations 1–5 against `enrich.CategoryValues`, normalize skills, drop `name` and `maxPerUser`); keep `Get`/`Delete`
- [x] 2.2 Rename `internal/handler/me_profiles.go` → `me_profile.go`: implement `GET /me/profile` (profile or `{"data": null}`), `PUT /me/profile` (upsert), `DELETE /me/profile` (204, idempotent); drop list/create/update/delete-by-id handlers
- [x] 2.3 Update `internal/handler/handler.go` route wiring to the singleton paths (remove `/me/profiles` and `/me/profiles/:id`)

## 3. Backend profile sub-resource endpoints (verdict + ATS report)

- [x] 3.1 Move the verdict handler to `GET /me/profile/verdict`: resolve the caller's single profile from the session (no `:id`), return 404 when the user has no profile
- [x] 3.2 Move the ATS-report handlers to `GET`/`POST /me/profile/ats-report` (`internal/handler/ats_report.go`, `atsContext`): resolve the profile from the session, drop `pathID`, 404 when the user has no profile

## 4. Frontend data layer

- [x] 4.1 Update `web/src/lib/types.ts`: `SearchProfile`/profile type drops `id` and `name` (keep `specializations`, `skills`, timestamps)
- [x] 4.2 Replace `web/src/lib/searchProfiles.svelte.ts` with a single-profile store `profile.svelte.ts`: holds one `profile | null`, `ensureLoaded()` (GET), `save()` (PUT), `clear()` (DELETE), `reset()` on sign-out
- [x] 4.3 Update `web/src/lib/api.ts` profile/verdict/ats-report helpers to the singleton paths (`/me/profile`, `/me/profile/verdict`, `/me/profile/ats-report`); drop id params

## 5. Frontend profile route, view & edit modal

- [x] 5.1 Replace routes `/my/profiles`, `/my/profiles/new`, `/my/profiles/[id]/edit` with a single `/my/profile` page: shows the profile (specialization + skill chips, skill-gap block, verdict link) or an empty state, with an Edit button; anonymous → sign-in prompt
- [x] 5.2 Build the profile edit modal reusing the jobs facet machinery: render `FacetSection` for `category` (cap 5) and `skills` (dynamic, live `counts.facets.skills`) via profile-scoped `FacetDef`s with `excludable:false`/`hasAndOr:false`, backed by a staging store seeded from the profile; Save (enabled when ≥1 specialization and ≥1 skill) calls store `save()` → `PUT /me/profile`. Keep CV upload (merges into the skills facet). Remove the bespoke `SearchSelect`/`RemoteSearchSelect` pickers
- [x] 5.3 Remove `ProfileForm.svelte` + `SearchProfilesView.svelte` (superseded by the page + modal); render the skill-gap block for the single profile in the profile view
- [x] 5.4 Move the verdict page (which also hosts the ATS report via `ATSReportView.svelte`) to `/my/profile/verdict` (no `[id]`), pointing at `GET /me/profile/verdict` and `/me/profile/ats-report`

## 6. Header avatar & menu

- [x] 6.1 Add `web/src/lib/components/Avatar.svelte`: circle with the email's first character on a colour deterministically derived from the email
- [x] 6.2 Update `HeaderMenu.svelte`: render the avatar + email as a single clickable identity row linking to `/my/profile` (signed-in only); rename the account item "Search profiles" → "Profile" targeting `/my/profile`

## 7. Verification

- [x] 7.1 Backend: `go build ./... && go vet ./... && go test ./...`
- [x] 7.2 Frontend: `svelte-check` clean for touched files (no unit runner in repo)
- [x] 7.3 Confirm no dangling references to `/me/profiles`, profile `id`, or `name` across `internal/` and `web/src/`
