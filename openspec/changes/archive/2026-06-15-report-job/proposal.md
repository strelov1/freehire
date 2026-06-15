## Why

The catalogue carries stale, spam, and fraudulent vacancies that no automated signal
catches: a board posting that quietly stopped responding, a "vacancy" that is really an
ad, or an outright scam. Today a user who spots one has no way to tell us — the only
correction paths are the ingest sweep and the liveness probe, neither of which sees these
cases. We want signed-in users to flag a problem vacancy so a moderator can review it and
close it, reusing the moderation queue pattern we already built for submissions.

## What Changes

- Introduce a **report queue**: a staging table `job_reports` that records a user's
  complaint about a specific live job. A report never changes the job on its own — the
  canonical `jobs` table and every read surface stay untouched until a moderator acts.
- Add a **report** endpoint for any authenticated user:
  - `POST /api/v1/jobs/:slug/reports` — file a `pending` report against the job resolved
    from the slug. Body: `reason` (required, from a fixed vocabulary), `details` (required
    free text), `contact_telegram` (optional). The acting user is recorded.
  - At most **one open report per (user, job)**: while a user's report is awaiting review,
    a second report of the same job by that user is a `409` (a partial unique index). A
    different user reporting the same job is always allowed — that overlap is the signal.
- Add **moderator review** endpoints (`RequireRole("moderator")`):
  - `GET /api/v1/reports` — the pending queue, including each reporter's email and the
    reported job's slug/title so the moderator can judge it.
  - `POST /api/v1/reports/:id/resolve` — mark the report `resolved`; optionally close the
    reported job in the same action (reusing the existing soft-close lifecycle).
  - `POST /api/v1/reports/:id/dismiss` — mark the report `dismissed` (not a real problem),
    with an optional reason. No change to the job.
- **Reason vocabulary** (closed set, mirrored by the SPA): `no_response`, `not_relevant`,
  `spam`, `fraud`, `other`.
- **Web** (SvelteKit): a "Пожаловаться на вакансию" button in the job view that opens a
  two-step modal (pick a reason → add details + optional Telegram), and the report queue
  added to the role-gated `/moderation` page.

## Capabilities

### New Capabilities
- `job-report`: the user-facing report queue — the `job_reports` staging table, the
  authenticated `POST /jobs/:slug/reports` endpoint, the controlled `reason` and
  `pending`/`resolved`/`dismissed` status vocabularies, the one-open-report-per-(user,job)
  invariant, and the moderator review flow (`GET /reports`, `resolve`, `dismiss`) where
  resolve may close the reported job through the existing job-lifecycle soft-close.

### Modified Capabilities
<!-- No spec-level requirement changes to existing capabilities. The moderator role and
     soft-close behavior are reused as-is; web changes are implementation, see Impact. -->

## Impact

- **Schema**: migration `0020_job_reports.sql` — the `job_reports` table (FKs to `users`
  and `jobs`, `reason` and `status` CHECKs, a partial unique index on
  `(reported_by, job_id) WHERE status = 'pending'`). Next free local number is `0020`
  (last applied is `0019`); base on `origin/main`.
- **DB access** (sqlc): new queries `CreateReport`, `GetReport`, `ListPendingReports`,
  `MarkReportResolved`, `MarkReportDismissed`. Ingest and moderator-create write paths are
  untouched.
- **New package** `internal/report` (Service + Repository), mirroring `internal/submission`.
  Resolve-with-close delegates to the existing job-lifecycle close path.
- **Handler** `internal/handler`: new `reports.go` (file / list-pending / resolve / dismiss)
  + routes wired in `Register`. Report filing is `RequireAuthOrKey`; the queue and
  decisions are `RequireRole("moderator")`.
- **Web** `web/`: a report button + two-step modal in `JobView.svelte`, a reports API
  client, and a reports section in the `/moderation` route (gated on `user.role`).
- **Search**: a resolve-with-close removes the job from list/search via the existing
  `closed_at IS NULL` filter on the next reindex — no new index field.
