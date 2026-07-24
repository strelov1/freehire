# Per-user job tracking conventions

## Scope
Per-user job interactions: view/apply/save/track endpoints backed by the `user_jobs` table.

## Always true
- **One row per (user, job).** The composite PK `PRIMARY KEY (user_id, job_id)` is the dedup key — the invariant is "at most one interaction per (user, job)".
- **All writes are idempotent upserts** behind `RequireAuthOrKey` (session cookie or API key) and addressed by the job's public `:slug` (resolved to internal id before the write).
- **`stage` is a controlled vocabulary** (`userjob.Stages`/`ValidStage` in `internal/userjob/stages.go`): applied/screening/responded/interview/offer/accepted/rejected/withdrawn. The SPA mirrors it; an unknown stage is a 400 before any DB touch.
- Handlers return `{"data": interaction}` with `user_id` omitted; public job reads stay unauthenticated.
- `internal/db/user_jobs_stage_integration_test.go` covers the stage vocabulary and interactions.

## How it works

The per-user job-tracking use cases — **`RecordView`** (touches `viewed_at`), **`MarkApplied`** (sets `applied_at`), **`SaveJob`/`UnsaveJob`** (toggles the saved mark), **`TrackJob`** (sets application `stage` and/or `notes`) — live in **`internal/jobtracking`** (Fiber/pgx-free service; the HTTP handlers in `internal/handler/user_jobs.go` translate the wire format). The SPA records views silently — failures are swallowed and must not break the page.

`internal/userjob` itself keeps the shared tracking vocabulary: `stages.go` defines the controlled stage vocabulary with validation (`ValidStage`), and `buckets.go` provides the job-status buckets (saved, viewed, applied, etc.) used by the tracking UI and the pipeline aggregation.

The `/me/tracking` read joins the caller's interactions with the jobs they touch. View history = all rows; applications = `applied_at IS NOT NULL`.

## Limitations
- No bulk operations (e.g. "mark all viewed"); each interaction is an individual per-(user, job) upsert.
