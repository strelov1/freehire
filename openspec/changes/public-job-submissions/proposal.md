## Why

Today only a `moderator` can add a vacancy (`POST /api/v1/jobs`), and an automated source
supplies everything else. There is no way for an ordinary signed-in user to contribute a
job they found — the catalogue can only grow as fast as the boards we crawl and the
moderators who hand-curate. We want to open contribution to any authenticated user while
keeping a human gate: anyone can *submit* a vacancy, but it only goes live after a
moderator approves it. The work must be reachable both from the web UI and the
`freehire` CLI.

## What Changes

- Introduce a **submission queue**: a staging table `job_submissions` that holds
  user-contributed vacancies awaiting review. A submission is never a public job until a
  moderator approves it, so the canonical `jobs` table and every read surface
  (list/search/company/sitemap/Meilisearch) stay untouched — no new "is it approved"
  filter to thread everywhere.
- Add a **submit** endpoint for any authenticated user:
  - `POST /api/v1/submissions` — create a `pending` submission. Body is the same content
    contract as a moderator create (`url`/`title`/`company` required; `location`/`remote`/
    `description`/`posted_at`/`source` optional). Re-submitting a URL already pending is a
    `409` (one pending row per URL).
  - `GET /api/v1/me/submissions` — the caller's own submissions with their status and any
    rejection reason ("My submissions").
- Add **moderator review** endpoints (`RequireRole("moderator")`):
  - `GET /api/v1/submissions` — the pending queue, including each submitter's email.
  - `POST /api/v1/submissions/:id/approve` — mint a live vacancy from the submission's
    fields by **reusing `moderation.Create`** (same derivation, dedup, and enrichment
    enqueue), then mark the submission `approved` and record the minted `job_id`. The
    minted job's `created_by` is the **submitter** (content author); the approving
    moderator is recorded on the submission's `reviewed_by`. Post-approval edits use the
    existing `PATCH /api/v1/jobs/:slug`.
  - `POST /api/v1/submissions/:id/reject` — mark the submission `rejected` with an
    optional reason.
- Expose the user **`role`** on the `/auth` user response so the SPA can decide whether to
  show the moderation queue (the server still authorizes independently).
- **Web** (SvelteKit): a `/submit` form, a `/my/submissions` list, and a role-gated
  `/moderation` review queue.
- **CLI** (`freehire-cli`, separate repo): `freehire submit`, `freehire submissions`
  (mine), and the moderator `submissions pending|approve|reject` commands.

## Capabilities

### New Capabilities
- `job-submission`: the user-facing submission queue — the `job_submissions` staging
  table, the authenticated `POST /submissions` + `GET /me/submissions` endpoints, the
  controlled `pending`/`approved`/`rejected` status, the one-pending-row-per-URL
  invariant, and the moderator review flow (`GET /submissions`, `approve`, `reject`) that
  mints a live job through the existing `moderation` use case on approval.

### Modified Capabilities
- `user-auth`: the user read shape now includes `role` so clients can gate
  moderator-only UI (authorization itself is unchanged — `RequireRole` still loads the
  role from the database per request).

## Impact

- **Schema**: migration `0019_job_submissions.sql` — the `job_submissions` table (FKs to
  `users` and `jobs`, a `status` CHECK, and a partial unique index on `lower(url) WHERE
  status='pending'`). Next free local number is `0019` (last applied is `0018`).
- **DB access** (sqlc): new queries `CreateSubmission`, `GetSubmissionForUpdate`,
  `ListPendingSubmissions`, `ListSubmissionsByUser`, `MarkSubmissionApproved`,
  `MarkSubmissionRejected`; `users.role` added to the user read queries' projection.
  The ingest and moderator-create write paths are untouched.
- **New package** `internal/submission` (Service + Repository, mirroring
  `internal/moderation`). It depends on `internal/moderation` for the approve-time mint.
- **Reuse**: `moderation.CreateInput.Validate()` is made exported so submit-time and
  approve-time validation share one source of truth.
- **Handler** `internal/handler`: new `submissions.go` (submit/list-mine/list-pending/
  approve/reject) + routes wired in `Register`; `role` added to `userResponse`.
- **Web** `web/`: new `/submit`, `/my/submissions`, and `/moderation` routes, a submissions
  API client, and a moderation menu entry gated on `user.role`.
- **CLI** (`freehire-cli`, separate repo): new `submit` and `submissions` command group.
- **Search**: approved jobs reach Meilisearch via `make reindex` (same as every source),
  not synchronously.
