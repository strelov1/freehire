## Why

The profile feature carries collection-shaped complexity — up to 50 named
profiles per user, addressed by id — that no user needs. A person has one
professional identity. Collapsing to a single per-user profile removes whole
axes of incidental complexity (profile ids in URLs, per-user name uniqueness,
list endpoints, a client-side array store) and makes the header able to surface
that one profile directly. Doing it now, while the feature is MVP-stage with
negligible production data, avoids paying migration cost later.

## What Changes

- **BREAKING** Collapse search profiles from a per-user collection to a single
  per-user profile. Drop the `name` field and the 50-profile cap; a profile is
  now uniquely `(user_id)`.
- **BREAKING** Replace the collection API (`GET/POST /me/profiles`,
  `PATCH/DELETE /me/profiles/:id`) with a singleton API: `GET /me/profile`
  (profile or `null`), `PUT /me/profile` (upsert), `DELETE /me/profile`. A
  profile still only exists once validly filled (specializations 1–5, skills
  non-empty); no empty placeholder row is created.
- **BREAKING** Move the market-coverage verdict endpoint from
  `/me/profiles/:id/verdict` to `/me/profile/verdict`, scoped by session instead
  of by profile id.
- Rename the DB table `search_profiles` → `user_profiles` (drop `name`, add
  `UNIQUE (user_id)`), migrating each user's most-recently-updated row and
  discarding the rest. Rename the Go package `searchprofile` → `userprofile` and
  handler file accordingly; regenerate sqlc.
- Web: replace the `/my/profiles`, `/my/profiles/new`, `/my/profiles/[id]/edit`
  routes with a single `/my/profile`; verdict moves to `/my/profile/verdict`.
  The client store holds one profile object, not an array. The profile form
  drops the `name` field and the create-vs-edit distinction. The profiles-list
  view is removed.
- Header menu: add an avatar placeholder (initials-in-a-circle, deterministic
  colour from the email) and turn the email row into a clickable
  avatar-plus-email affordance that links to `/my/profile`. Rename the account
  item "Search profiles" → "Profile" targeting `/my/profile`.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `search-profiles`: reshaped from a named, per-user *collection* (create/list/
  update/delete by id, 50-cap, per-user unique name) to a single per-user
  *profile* (singleton GET/PUT/DELETE, no `name`, no id). The resume-skills
  merge, market skill-gap, and management-UI requirements are restated in
  singular terms.
- `header-navigation`: the signed-in menu gains an avatar placeholder and a
  clickable identity row (avatar + email) linking to the profile; the account
  item is renamed "Profile" and points at `/my/profile`.
- `resume-verdict`: the verdict endpoint is restated as the singleton
  `GET /me/profile/verdict`, owned by session rather than by profile id.

## Impact

- **DB**: new migration `0041` (rename table, drop `name`, add `UNIQUE
  (user_id)`, dedup to newest per user). Applied manually before deploy — no
  versioned runner.
- **Backend**: `internal/searchprofile` → `internal/userprofile`;
  `internal/handler/me_profiles.go` → singleton handlers; `internal/db/queries`
  search-profile SQL rewritten (singleton upsert/get/delete) + `make sqlc`;
  verdict handler path/ownership.
- **Frontend**: `web/src` routes under `/my/profile`, `profile.svelte.ts` store,
  `ProfileForm.svelte`, new `ProfileView`, removal of `SearchProfilesView`,
  `HeaderMenu.svelte` + new `Avatar.svelte`.
- **No change** to enrichment vocab, job search, or auth primitives.
