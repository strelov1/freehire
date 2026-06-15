## Context

The catalogue grows only through automated sources and moderator hand-curation
(`POST /api/v1/jobs`, gated by `RequireRole("moderator")`, implemented in
`internal/moderation`). An ordinary signed-in user has no way to contribute a vacancy.
We want open contribution with a human gate: any authenticated user submits a vacancy,
and it goes live only after a moderator approves it. Both the web SPA and the `freehire`
CLI must drive the flow.

The codebase already has the pieces this leans on:
- `internal/moderation.Service.Create(ctx, actorID, CreateInput)` mints a live `manual`
  vacancy: it validates, derives geo/skills/slugs via `jobderive`, sanitizes the
  description, upserts on `(source, external_id=url)`, and enqueues enrichment — all in
  one transaction. It is **idempotent** on the URL.
- `internal/auth.RequireRole(queries, "moderator")` authorizes by a DB-loaded role.
- The "two-stage queue" idiom (`telegram_posts → extract → jobs`,
  `enrichment_outbox → jobs`): raw rows live in a staging table that an action drains
  into the canonical `jobs` table.

## Goals / Non-Goals

**Goals:**
- Any authenticated user can submit a vacancy; it is invisible publicly until approved.
- A moderator can review a queue and approve (→ live job) or reject (with a reason).
- A submitter can see the status of their own submissions.
- Reach the flow from both the web UI and the CLI.
- Reuse the existing moderator-create machinery for the approve step — no second
  derivation/dedup/enrichment path.

**Non-Goals:**
- Anonymous submission (submission requires a logged-in account).
- Self-service role management (roles stay granted out of band, as today).
- Notifying submitters out of band (email/push) — they poll "My submissions".
- Editing a submission in place before approval (moderators edit the live job after
  approval via the existing `PATCH /api/v1/jobs/:slug`).
- Rate-limiting / anti-abuse beyond the one-pending-row-per-URL guard.
- De-duplicating a submission against an already-live job at submit time.

## Decisions

### Decision: A separate `job_submissions` staging table, not a status column on `jobs`

A submission lives in its own table and only becomes a `jobs` row on approval.

*Why:* every public read surface (`ListJobs`, `/jobs/search`, company pages, sitemap) and
the Meilisearch index already filter `closed_at IS NULL`. Putting pending submissions into
`jobs` behind a new `status='pending'` gate would force every one of those surfaces and the
index pipeline to learn the new filter; missing one leaks an unreviewed vacancy into public
listings. A staging table keeps `jobs` canonical and the read surfaces untouched. It also
matches the existing `telegram_posts` pattern.

*Alternative considered:* a `jobs.status` column (reuses derivation immediately but spreads
a fragile filter across many surfaces) — rejected.

### Decision: Approve reuses `moderation.Service.Create`

`internal/submission` depends on `internal/moderation`. On approve, the submission service
loads the pending row and calls `moderation.Create(ctx, submittedBy, CreateInput{…})`,
then marks the submission approved with the returned `job_id`.

*Why:* derivation, sanitization, dedup, and the enrichment enqueue already live there and
are exactly what an approved job needs. Duplicating them would be two code paths to keep in
sync. `created_by = submittedBy` attributes authorship to the contributor; the approving
moderator is recorded on `job_submissions.reviewed_by`.

*Alternative considered:* a shared lower-level "mint job" function called by both
moderator-create and approve — more indirection than the dependency buys today; the
`moderation.Service` is already the right seam. Revisit if a third caller appears.

### Decision: Validate once, at one source of truth

`moderation.CreateInput.Validate()` is promoted from unexported to exported. Submit-time
validation (in `submission.Submit`) and approve-time validation (inside
`moderation.Create`) call the same function, so "what is a valid vacancy" cannot drift
between the two surfaces.

### Decision: Non-atomic mint-then-mark, relying on URL idempotency

Approve does two writes that are not wrapped in one outer transaction: (1)
`moderation.Create` (its own transaction: upsert + enrichment enqueue), then (2)
`MarkSubmissionApproved`. If (2) fails after (1) commits, the submission stays `pending`
and the moderator retries approve; because `moderation.Create` is idempotent on the URL,
the retry upserts the same job and then marks the submission. No duplicate job, no lost
work.

*Why:* threading one transaction through `moderation.Create` would require it to accept an
external `tx`, coupling the two packages more tightly for a failure window that is
self-healing. Noted as a seam to revisit if approve grows more side effects.

### Decision: One pending submission per URL via a partial unique index

`CREATE UNIQUE INDEX ... ON job_submissions (lower(url)) WHERE status='pending'`. A second
submission of a URL already awaiting review hits the constraint; the repository maps the
unique-violation to `ErrDuplicatePending` (→ `409`). Decided submissions (approved/rejected)
do not block resubmission.

### Decision: `role` on the user wire shape, server still authorizes

`toUserResponse` gains `role` so the SPA can show/hide the `/moderation` entry. This is a
UI affordance only — every moderator endpoint is still gated by `RequireRole`, which loads
the role from the DB per request and never trusts the client.

### Decision: Top-level `/api/v1/submissions` namespace

Submissions are their own resource, not nested under `/jobs`, avoiding any route-ordering
interplay with `/jobs/:slug` and reading cleanly: `POST /submissions`,
`GET /me/submissions`, `GET /submissions` (queue), `POST /submissions/:id/{approve,reject}`.

## Risks / Trade-offs

- **Spam / low-quality submissions flood the queue** → only authenticated users can
  submit (every submission is attributable), and the one-pending-row-per-URL index caps
  trivial duplication. Heavier rate-limiting is a noted follow-up, not in this change.
- **A submission duplicates an already-live job** → harmless: approve upserts on the URL,
  so it updates the existing job rather than creating a second one. Submit-time dedup
  against live jobs is deferred.
- **Mint succeeds but mark-approved fails** → self-healing via URL idempotency (see the
  decision above); worst case is a re-approve.
- **`ON DELETE` of a referenced user/job** → `submitted_by` is `ON DELETE CASCADE` (a
  deleted account's submissions go with it); `reviewed_by` and `job_id` are
  `ON DELETE SET NULL` (history survives a deleted moderator or a deleted job).

## Migration Plan

1. Ship migration `0019_job_submissions.sql` (new table + partial unique index). It only
   adds a table, so it is safe on a live DB and needs no backfill. Per project ops, run the
   migration manually against prod after deploy (no versioned runner yet).
2. Deploy the server (new endpoints + `role` on the user shape) and the web build.
3. Release the `freehire-cli` `submit`/`submissions` commands from the sibling repo.
4. Rollback: the endpoints are additive and the table is independent; reverting the server
   build disables the flow, and the table can be dropped if abandoned. No data migration to
   reverse.

## Open Questions

- None blocking. Future work (out of scope): submission rate-limiting, submit-time dedup
  against live jobs, and submitter notifications on approve/reject.
