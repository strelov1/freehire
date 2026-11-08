## Context

The backend already has stateless-JWT auth (httpOnly cookie, `RequireAuth`
middleware in `internal/auth`) and a public jobs read surface
(`GET /api/v1/jobs[/:id]`). There is no per-user state yet. This change adds the
first user-scoped data: a record of which jobs a user has viewed and applied to,
surfaced in the SPA `JobView`. It is deliberately the thin first slice of a
larger personal application tracker (stage pipeline, dedicated page) that is
designed-for but not built here.

Constraints that shape the design:

- sqlc is the only DB layer — schema lives in `migrations/`, access in
  `internal/db/queries/*.sql`, generated code committed.
- Response envelopes are `{"data": ...}` / `{"error": msg}`; handlers signal
  failure by returning errors to the central `handler.ErrorHandler`.
- Migrations apply via Postgres initdb only (no versioned runner yet).

## Goals / Non-Goals

**Goals:**

- Record a view automatically when a signed-in user opens a job.
- Let a signed-in user mark a job applied via an explicit, idempotent action.
- Show an "already applied" badge on a job the user returns to.
- Keep the public job read path (`GET /jobs/:id`) untouched and
  auth-independent.
- Shape the schema so the future stage pipeline slots in without reshaping
  existing rows.

**Non-Goals:**

- No application **stage** column/pipeline (No Response → … → Offer) yet.
- No "Applications / My jobs" listing page or listing endpoints.
- No bulk interaction status for the job list (`JobRow`) — the badge is on the
  opened `JobView` only.
- No optional-auth middleware for read endpoints.

## Decisions

### One table `user_jobs`, composite PK `(user_id, job_id)`

A single row per `(user, job)` pair holds both `viewed_at` (NOT NULL, defaulted)
and `applied_at` (NULL until applied). View history is "all rows"; the
application tracker is "rows where `applied_at IS NOT NULL`".

- *Why over two tables (views + applications):* the two facts describe the same
  pair and the apply action would otherwise have to guarantee a separate view
  row. One table avoids duplicate `(user, job)` bookkeeping while still letting
  the two states be queried independently.
- *Why over an append-only event log:* the badge needs current state, which a log
  forces you to aggregate. A log is the wrong shape for a single-row lookup and
  is overkill at this stage.
- The composite PK is also the dedup key and encodes the invariant "at most one
  application per (user, job)" — the future stage pipeline cannot create a
  duplicate application.
- `ON DELETE CASCADE` on both FKs keeps the table consistent when a user or job
  is removed.

### Two idempotent upsert endpoints, both `RETURNING *`

`RecordJobView` and `MarkJobApplied` are `INSERT … ON CONFLICT (user_id, job_id)
DO UPDATE …` returning the row. Idempotency means the SPA can call view on every
open and apply on every confirm without special-casing "already exists", and the
returned row always carries the current `applied_at` so the client learns the
applied state from the same call that records the view.

### Reads stay public; only writes require auth

`GET /jobs/:id` is unchanged and needs no auth. The two new write endpoints sit
behind the existing `RequireAuth`. The SPA only calls them when a session exists
(`auth.user` set). This avoids building optional-auth middleware (YAGNI) — the
client, which already knows the auth state, gates the calls.

### "No" writes nothing

The apply prompt's **No** is a pure client-side dismissal: the job must not enter
the tracker. Only **Yes, save** hits `MarkJobApplied`. This keeps the tracker a
record of real applications, not of prompts seen.

## Risks / Trade-offs

- [A view is recorded on every open, including accidental opens] → Acceptable:
  views are passive history with no user-visible weight this iteration; only
  applies (explicit) carry meaning. Refine later if a history page surfaces noise.
- [`viewed_at` is "last viewed", not "first viewed" (the upsert touches it)] →
  Acceptable for history; if first-view ordering ever matters, add a separate
  `first_viewed_at` then. Not needed now.
- [New migration won't apply to an existing dev volume] → Known initdb-only
  limitation; documented: recreate the volume with `docker compose down -v &&
  make up`. Same seam as prior migrations; out of scope to fix here.
- [Client gates the write calls, so a misbehaving client could spam them] →
  Bounded by `RequireAuth` (must be a real session) and idempotent upserts (no
  row growth); rate-limiting is a separate, already-noted auth seam.

## Migration Plan

1. Add `migrations/0006_user_jobs.sql`; it applies on fresh initdb. For an
   existing dev DB: `docker compose down -v && make up`.
2. Ship backend (queries + handlers + routes) and SPA together — the endpoints
   are additive and the SPA degrades gracefully when signed out.
3. Rollback: drop the two routes and the table; no existing data or endpoint
   depends on `user_jobs`, so removal is clean.

## Open Questions

None — scope and model are settled. The stage pipeline, listing page, and list
badges are explicitly deferred and noted as seams, not open questions.
