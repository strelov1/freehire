## Context

The catalogue already has two moderation building blocks this change reuses rather than
re-invents:

- **The submission queue** (`internal/submission` + `job_submissions`, migration 0019):
  a staging table, a `Service`+`Repository` pair where the repository maps Postgres
  conditions (unique violation, no-row) onto package sentinels, a controlled status
  vocabulary in a `CHECK`, a partial unique index for the dedup invariant, and a
  role-gated review flow. `internal/report` is a structural copy of this.
- **The soft-close lifecycle** (`job-lifecycle`): a job is open while `closed_at IS NULL`;
  closing is a soft state written today by the ingest sweep (`CloseUnseenJobs`) and the
  liveness probe (`MarkLivenessExpired`). A resolve-with-close is a third writer of the
  same column.

Authorization is already in place: `auth.RequireAuthOrKey` (cookie **or** API key) for the
filing path, `auth.RequireRole(a.queries, "moderator")` for the review path, and
`auth.UserID(c)` to read the acting user from `c.Locals`. The user `role` is already on the
`/auth` user response (added by public-job-submissions), so the SPA can gate the queue.

Work is based on `origin/main` (the local checkout is 10 commits behind). Last applied
migration is `0019`, so this change adds `0020`.

## Goals / Non-Goals

**Goals:**
- A signed-in user can flag a live vacancy with a reason from a fixed vocabulary, required
  details, and an optional Telegram contact, addressed by the job's public slug.
- A moderator can review the pending queue and either resolve a report (optionally closing
  the job through the existing soft-close) or dismiss it.
- The dedup invariant "one open report per (user, job)" holds at the database level.
- Reuse the submission/lifecycle patterns so reports behave consistently and add no new
  cross-cutting concepts.

**Non-Goals:**
- No auto-close on a report threshold — every close is a moderator decision (the
  threshold is a clean later seam, not built now).
- No public/anonymous reporting — authentication is required (decided in brainstorming;
  the Telegram field is optional extra contact, not an identity).
- No reporter-facing "my reports" list — a report is fire-and-forget for the user
  (unlike submissions, the reporter has no live artifact to track). Seam noted.
- No CLI command and no new Meilisearch field — a closed job leaves search via the
  existing `closed_at IS NULL` filter on the next reindex.
- No notification/email to moderators — the queue is pull, same as submissions.

## Decisions

### Reports are scoped under the job, addressed by slug

The filing endpoint is `POST /api/v1/jobs/:slug/reports`, mirroring the existing per-job
interaction routes (`/jobs/:slug/view|apply|save|track`). The handler resolves the slug to
the internal id with the existing `GetJobIDBySlug` query (a missing slug → `404` before any
write), then passes the id to the service. The report row stores `job_id`, not the slug, so
it survives a reslug — consistent with `user_jobs`.

*Alternative considered:* a flat `POST /api/v1/reports` with the job in the body (as
submissions do with `url`). Rejected: a report is always *about an existing catalogue job*,
so the job belongs in the path like every other per-job action; the slug also gives a clean
404 for a bad target without a body round-trip.

### `internal/report` mirrors `internal/submission`, minus the Minter

A new package `internal/report` with `Service` + `Repository` (and `QueriesRepository`
adapting `*db.Queries`). The repository maps a unique violation → `ErrDuplicateOpen`, a
missing row → `ErrReportNotFound`, and a no-row on a status-scoped mark → `ErrAlreadyDecided`
— the same sentinel-mapping shape as submissions. The service validates the `reason`
vocabulary and non-empty `details` before any write.

Unlike submissions, there is **no Minter**: a report never creates a job. Resolve *may*
close a job, so the service takes a narrow `JobCloser` seam (`Close(ctx, jobID) error`)
instead, satisfied by a repository method over a new `CloseJobByID` query. This keeps the
service testable without a database and keeps the close path a single, named SQL write.

*Alternative considered:* fold reports into `internal/submission`. Rejected: different
domain object, different status machine (resolve/dismiss vs approve/reject), different
side effect (close vs mint). A sibling package is clearer than an overloaded one.

### Closing on resolve is a new single-row `CloseJobByID`, not a reuse of existing closers

`CloseUnseenJobs` is source-scoped and time-scoped; `MarkLivenessExpired` is strike-driven.
Neither fits "close this one job now." Add `CloseJobByID` — `UPDATE jobs SET closed_at =
now(), updated_at = now() WHERE id = $1 AND closed_at IS NULL` — the minimal idempotent
soft-close (a second resolve-with-close on an already-closed job is a no-op, not an error).
The report's own status guard (`MarkReportResolved` scoped to `status='pending'`) prevents
double resolution; the job close is intentionally idempotent so it never fights that guard.

### Wire shape omits the reporter id; the queue adds reporter email + job slug/title

`reportResponse` follows `submissionResponse`: `reported_by` is internal and never
serialized; the moderator-queue row (`ListPendingReportsRow`) adds `reporter_email`,
`job_slug`, and `job_title` (joined in the query) so a moderator can judge the report and
link to it. `contact_telegram` is returned as-is. Status and review fields
(`status`, `review_reason`, `reviewed_at`) mirror submissions.

### Reason and status are closed vocabularies in the migration `CHECK`

`reason IN ('no_response','not_relevant','spam','fraud','other')` and
`status IN ('pending','resolved','dismissed')` live in the migration, the same way the
enrichment enums and the submission status do. The service re-validates `reason` before the
write (a clean `400` instead of a raw constraint error), and the SPA mirrors the five
reasons. The dedup invariant is a partial unique index:
`UNIQUE (reported_by, job_id) WHERE status = 'pending'`.

### Web: a two-step modal in JobView, the queue in /moderation

`JobView.svelte` gains a "Пожаловаться на вакансию" button among the existing top-right
actions; a signed-out click opens the auth dialog (the established `openAuthDialog` pattern
used by save). The button opens a two-step modal: step 1 picks one of the five reasons,
step 2 collects required details + optional Telegram and posts. The `/moderation` route
gains a reports section listing the pending queue with resolve (with a "close job" toggle)
and dismiss actions, gated on `user.role === 'moderator'` (the server still authorizes).

## Risks / Trade-offs

- **A user re-reports a job after their first report is decided** → allowed by design: the
  partial unique index only blocks `pending` duplicates, so the signal can recur if the
  problem recurs. Acceptable; not spam because it requires a prior resolution.
- **Resolve-with-close races a concurrent reopen (e.g. an ingest upsert)** → `CloseJobByID`
  is `WHERE closed_at IS NULL` and idempotent; a later upsert legitimately reopens a board
  job, which is the lifecycle's existing reopen-on-reappear behavior, not a bug.
- **No abuse ceiling beyond auth** → authenticated-only + one-open-per-(user,job) is the
  brainstormed bound; a global rate limit is a noted seam if abuse appears.
- **`details` required but unbounded** → validate non-empty and trim; a sane max length cap
  in the service guards against a giant payload (mirrors how content fields are treated).

## Migration Plan

1. Add `migrations/0020_job_reports.sql` (table + CHECKs + partial unique index + FKs with
   `ON DELETE CASCADE` for `reported_by`/`job_id`, matching the ownership-FK convention).
2. Add the queries to `internal/db/queries/` and run `make sqlc` (or `sqlc generate` —
   `sqlc@v1.31.1` if Docker is unavailable); commit the generated `internal/db` code.
3. Implement `internal/report`, the handler, and the routes; wire `a.report` in `Register`.
4. Web: modal + API client + `/moderation` section.
5. **Deploy ordering** (per the moderator-create deploy gotcha — sqlc `SELECT *` couples
   schema and generated code): deploy the binary, then apply `0020` immediately via psql on
   prod (no auto-runner). A closed job needs the routine reindex to leave search; no
   special backfill.
6. **Rollback:** the feature is additive — the table and routes can sit unused; to fully
   revert, drop the routes and `DROP TABLE job_reports`. No existing data is mutated except
   intentional soft-closes (reversible by clearing `closed_at`).

## Open Questions

- Should a resolved-with-close action also record *why* on the job (e.g. an audit note),
  or is the report row's own audit trail enough? Leaning: the report row is enough; the job
  already has `updated_by`/`updated_at` for moderator edits, and a close-from-report is
  traceable via the resolved report. Defer unless an audit need surfaces.
