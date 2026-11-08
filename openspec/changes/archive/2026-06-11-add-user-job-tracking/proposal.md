## Why

Signed-in users have no way to keep track of which jobs they have already looked
at or applied to — every visit starts from scratch. Recording a lightweight
per-user history of views and applies is the first step toward a personal
application tracker, and it immediately improves the browsing experience by
showing an "already applied" badge on a job they return to.

## What Changes

- Introduce a per-user job interaction record: one row per `(user, job)` pair
  capturing when the user **viewed** the job and (optionally) when they marked it
  **applied**.
- Add two authenticated endpoints under the existing `/api/v1/jobs/:id` surface:
  - `POST /api/v1/jobs/:id/view` — silently record (or refresh) a view.
  - `POST /api/v1/jobs/:id/apply` — mark the job as applied.
  Both are idempotent and return the interaction record. `GET /api/v1/jobs/:id`
  stays public and unchanged.
- Wire the SPA `JobView`: for a signed-in user, opening a job records a view; an
  inline "Did you apply? Yes, save / No" prompt (revealed after clicking Apply)
  marks the job applied; a "You applied" badge shows on a job already applied to.
  Choosing **No** writes nothing — the job does not enter the tracker. Signed-out
  behavior is unchanged.

Out of scope this iteration (design seam only): an application **stage** pipeline
on applied rows, an "Applications / My jobs" listing page, and bulk status badges
in the job list.

## Capabilities

### New Capabilities

- `user-job-tracking`: per-user recording of job views and applies, the
  authenticated endpoints that write them, and the rule that the public job read
  path is unaffected.

### Modified Capabilities
<!-- None: no existing spec's requirements change. -->

## Impact

- **Schema**: new `user_jobs` table (`migrations/0006_user_jobs.sql`), FK to
  `users` and `jobs`, composite PK `(user_id, job_id)`.
- **DB access**: new `internal/db/queries/user_jobs.sql` (sqlc) → regenerated
  `internal/db`.
- **API**: new `internal/handler/user_jobs.go`; two routes registered under
  `RequireAuth` in `handler.Register`.
- **Web**: `web/src/lib/api.ts` (+`types.ts`) gain `recordJobView`/`markJobApplied`
  and a `UserJob` type; `web/src/lib/components/JobView.svelte` gains the badge +
  apply prompt.
- **Auth**: reuses the existing httpOnly-cookie JWT + `RequireAuth`; no new auth
  primitives.
- **Deploy**: the new migration applies via Postgres initdb only — an existing
  dev volume needs `docker compose down -v && make up` to pick it up.
