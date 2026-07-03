## Context

Search profiles today are a per-user collection: table `search_profiles`
(`id`, `user_id`, `name`, `specializations[]`, `skills[]`, timestamps) with a
per-user unique `name`, a 50-row cap, and REST addressed by id (`GET/POST
/me/profiles`, `PATCH/DELETE /me/profiles/:id`). Market coverage hangs off a
profile id (`GET /me/profiles/:id/verdict`). The web app has three routes
(`/my/profiles`, `.../new`, `.../[id]/edit`) plus a verdict page, an array store
`searchProfiles.svelte.ts`, a list view `SearchProfilesView.svelte`, and a
create/edit `ProfileForm.svelte`.

Nobody needs many profiles; a user has one professional identity. The feature is
MVP-stage with negligible production data, so we can migrate destructively. The
repo has no versioned migration runner (migrations apply via Postgres initdb on
first volume init; prod applies them manually before deploy).

## Goals / Non-Goals

**Goals:**
- Exactly one profile per user, enforced at the DB level (`UNIQUE (user_id)`).
- A singleton REST surface with no id and no `name`.
- Header surfaces the profile: an avatar placeholder + a clickable identity row
  linking to `/my/profile`.
- Preserve the resume-skills merge, the market skill-gap block, and the verdict
  feature — only their addressing changes.

**Non-Goals:**
- Real avatar images / uploads (placeholder initials only).
- Any change to the enrichment vocabulary, job search, or auth primitives.
- Backfilling or reconciling discarded extra profiles (we keep newest, drop
  rest).

## Decisions

**D1 — Real 1:1 collapse, not a UI cap.** Migrate the schema to a genuine
singleton rather than keeping the collection and capping it at one. Rationale:
the MVP-fluid principle favours reshaping over bolting on a special case; a cap
would leave dead multi-infrastructure (name uniqueness, list endpoints, id in
URLs) that future readers must still reason about. _Alternative considered:_
fold `specializations[]`/`skills[]` as columns onto `users` — rejected because
it conflates the account with the search profile and complicates future profile
growth.

**D2 — Rename `search_profiles` → `user_profiles`.** The concept is no longer a
"search profile" collection but the user's single profile. New shape:
`user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE`,
`specializations TEXT[]`, `skills TEXT[]`, `created_at`, `updated_at`. Using
`user_id` as the primary key is the cleanest expression of the 1:1 invariant.
The Go package `internal/searchprofile` → `internal/userprofile` and
`me_profiles.go` → `me_profile.go` follow, keeping code and concept aligned.

**D3 — Singleton REST semantics.**
- `GET /api/v1/me/profile` → `200 {"data": <profile>}` or `200 {"data": null}`
  when the user has no profile yet.
- `PUT /api/v1/me/profile` → upsert (create-or-replace) with the existing
  validation (specializations 1–5 from `enrich.CategoryValues`; skills
  normalized, non-empty). `200` with the stored profile.
- `DELETE /api/v1/me/profile` → clear the profile; `204` (idempotent — `204`
  even if none existed).
- `GET /api/v1/me/profile/verdict` → market coverage for the caller's profile;
  `404` when the caller has no profile.
All remain cookie-only (`RequireAuth`), owner scoping is implicit (the session
user id is the key). Choosing `PUT` upsert over `PATCH` because the form always
edits the whole profile — there is no partial-field use case once `name` is
gone.

**D4 — A profile exists only when validly filled.** We do not create an empty
placeholder row on first visit. `GET` returns `null` until the user saves a
valid profile; the form renders empty in that state. This preserves the verdict
and skill-gap semantics (both require ≥1 specialization) without introducing an
"incomplete profile" state.

**D5 — Avatar placeholder as a small reusable component.** New
`Avatar.svelte` renders a circle with the email's first character; background
colour is a deterministic hash of the email (a small palette indexed by a
char-sum), so it is stable per user with no external dependency. It lives inside
the menu (the header spec already forbids a standalone avatar dropdown outside
the menu — this stays compliant).

## Risks / Trade-offs

- [Destructive migration discards extra profiles] → Acceptable: negligible prod
  data, and the design was approved with "data doesn't matter". The migration
  keeps each user's most-recently-updated row deterministically.
- [Manual migration ordering] → As with prior schema changes, apply `0041`
  before deploying the new binary, or reads hit `column/table does not exist`.
  Documented in tasks + the known deploy seam.
- [Breaking API surface] → The old `/me/profiles*` paths are removed, not
  aliased. Only our own SPA consumes them, updated in the same change; no
  external clients.

## Migration Plan

1. Ship migration `0041_collapse_search_profiles.sql`: within a transaction —
   delete non-newest rows per `user_id`, drop the `name` column and its unique
   index, rename table to `user_profiles`, add `PRIMARY KEY (user_id)` (or a
   `UNIQUE (user_id)` if keeping a surrogate is simpler for sqlc).
2. Regenerate sqlc, land backend + frontend together.
3. Prod: apply `0041` manually, then deploy. Rollback: the change is
   forward-only; a rollback would require restoring the old table shape from
   backup (low stakes given data volume).

## Open Questions

_None — design approved in brainstorming._
