## Why

Starting a CV for a vacancy is a clear intent to pursue that job, but the tailor
bootstrap only created a vacancy-bound CV — it never wrote tracking state. The
Tracking **Kanban** only shows jobs that have a stage (or `applied_at`); a bare
bookmark lives under Activity → Saved and never appears as a board card. Opening
an existing tailored CV via `?cv=` also skipped the bootstrap entirely, so even
a bookmark never ran on resume.

## What Changes

- On a successful tailor bootstrap (and on resume paths that reopen a tailored
  vacancy), the vacancy is placed on the caller's Tracking Kanban: bookmarked
  and staged as `applied` when no stage exists yet.
- `applied_at` is **not** set — preparing a CV is not submitting an application,
  so silence clocks must not start.
- An existing non-empty stage (e.g. interview) is left alone.
- Failures to write tracking must not fail or roll back an already-created
  tailored CV / session.
- The SPA resume path (`?cv=` with an existing session) re-runs the idempotent
  bootstrap for the board side-effect.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `cv-tailoring`: Bootstrap / resume places the vacancy on the Tracking Kanban.
- `user-job-tracking`: Tailoring is an implicit save + stage=`applied` (when
  unset) path for the vacancy.

## Impact

- `internal/handler/cv_tailor.go`, `cv.go`, `handler.go` wiring, and
  `web/src/routes/tailor/[slug]/+page.svelte` resume path.
- Integration coverage on create, heal, stage preservation, and fail-open.
- No migration; no change to the tailor response envelope.
