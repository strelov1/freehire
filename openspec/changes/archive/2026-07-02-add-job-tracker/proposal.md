## Why

Signed-in users already leave a per-job trail (`user_jobs`: viewed/applied), but
they can't see it — there is no way to revisit "jobs I looked at", no list of
applications, and no way to bookmark a job for later. The data model was built
as the first slice of a personal application tracker; this change adds the
missing read surface and the one write that users ask for first: saving a job.

## What Changes

- **Save/unsave a job**: a `saved_at` column on `user_jobs` plus
  `POST /api/v1/jobs/:slug/save` and `DELETE /api/v1/jobs/:slug/save`
  (idempotent, behind auth). Unsaving clears `saved_at` but keeps the row —
  view history survives.
- **"My jobs" listing**: `GET /api/v1/me/jobs?filter=all|saved|applied` returns
  the user's interactions joined with the job view shape, newest interaction
  first, with limit/offset pagination and per-tab counts in `meta`.
- **Interaction record carries `saved_at`**: the existing view/apply responses
  gain the field, so the job page knows the saved state from the view it
  already records.
- **SPA "My jobs" page**: a `/my/jobs` route with All / Saved / Applied tabs
  (reusing the job-row component), linked from the user menu; a Save toggle on
  the job detail page.

## Capabilities

### Modified Capabilities

- `user-job-tracking`: save/unsave, the my-jobs listing endpoint, `saved_at` in
  the interaction record.
- `web-frontend`: the My jobs page, user-menu entry, and the Save toggle on the
  job page.

## Impact

- **Schema**: `ALTER TABLE user_jobs ADD saved_at timestamptz` (new migration
  file; dev volumes need recreation — the versioned-migration-runner seam from
  AGENT.md remains open, prod gets a manual apply).
- **Code**: new queries in `internal/db/queries/user_jobs.sql` (save, unsave,
  list-with-jobs, counts) + regenerated sqlc; handlers in
  `internal/handler/user_jobs.go` + routes; `saved_at` in `interactionResponse`;
  SPA: route, page component, api client functions, Save button, user-menu link.
- **Out of scope**: application stages (HR Interview → Offer pipeline),
  statistics/response rate, export, manually added applications, save buttons
  and interaction badges on the public jobs list (needs a bulk interaction
  lookup — a later change).
