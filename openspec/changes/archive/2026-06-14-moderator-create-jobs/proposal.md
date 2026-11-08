## Why

Every job in the catalogue today arrives through an automated source (ATS boards,
Telegram, link-following). There is no way for a trusted human to add or correct a
vacancy directly. Moderators need a write API — driven from the CLI tool — to create
hand-curated vacancies and fix existing ones, without opening the write path to every
authenticated user.

## What Changes

- Introduce a **`role`** on users (`user` / `moderator` / `admin`, default `user`) and a
  `RequireRole` middleware so privileged endpoints can gate on it. Role is granted out of
  band (`psql`) — no self-service role management.
- Add two moderator-only endpoints:
  - `POST /api/v1/jobs` — create a hand-curated vacancy under `source='manual'`,
    `external_id=URL` (idempotent: re-POSTing the same URL updates rather than duplicates).
    Enqueues the new job for AI enrichment like every other source.
  - `PATCH /api/v1/jobs/:slug` — partial-update a manual vacancy's content fields
    (manual-source rows only; ATS/Telegram jobs are not editable through this path).
- Record **authorship audit** on jobs: `created_by` / `updated_by` (FK `users`, NULL for
  every non-manual source).
- Both endpoints authenticate via `RequireAuthOrKey` (the CLI sends a Bearer API key) and
  then `RequireRole("moderator")`.

## Capabilities

### New Capabilities

- `job-authoring`: moderator-driven creation and editing of hand-curated vacancies — the
  manual source identity (`source='manual'`, `external_id=URL`), the `POST`/`PATCH`
  endpoints, authorship audit (`created_by`/`updated_by`), enrichment enqueue on create,
  and the manual-source-only edit invariant.

### Modified Capabilities

- `user-auth`: adds a user `role` attribute and role-based authorization (`RequireRole`)
  layered over the existing cookie/API-key authentication.

## Impact

- **Schema**: migration `0017_jobs_moderation.sql` — `users.role` (+CHECK), `jobs.created_by`,
  `jobs.updated_by`. (Next free local number is `0017`; an unmerged classification change may
  also claim `0017` — reconcile/bump the number at merge time.)
- **DB access** (sqlc): new `UpsertManualJob` / `UpdateManualJob` queries; `users.role`
  surfaced on the user read queries. The ingest `UpsertJob` path is left untouched.
- **New package** `internal/moderation` (Service + Repository, mirroring `internal/jobtracking`).
- **Auth** `internal/auth`: new `RequireRole` middleware.
- **Handler** `internal/handler`: new `jobs_moderation.go` + two routes wired in `Register`.
- **Shared derivation**: a small helper factoring the geo/skills/slug derivation currently
  inline in `pipeline.normalizeJob`, reused by the manual write path.
- **CLI** (`freehire-cli`, separate repo): `freehire jobs add` / `jobs edit` commands —
  out of scope for this change, tracked as follow-up work.
- **Search**: manual jobs reach Meilisearch via `make reindex`, not synchronously (same as
  ingest).
