## Why

`auto-apply-tailored-resume` built the tailoring/review endpoints and
`auto-apply-inngest-orchestration` built the durable worker that sequences them, both on
the assumption that something else enqueues a `(candidate, job)` row into
`auto_apply_queue` and publishes `auto-apply/submit`. Nothing does: task 2.1 in
`auto-apply-tailored-resume`'s own `tasks.md` was deferred, confirmed during that change's
implementation that no enqueue trigger exists anywhere in the codebase, with product
direction already decided then — a button on the job posting page, for postings from an
ATS this feature supports (Greenhouse today), as its own change. This change is that
button and the endpoint behind it.

## What Changes

- New endpoint `POST /api/v1/jobs/:slug/auto-apply` (`mw.cookie`-only — the same
  "the browser is the only place the candidate can watch and undo it" reasoning
  `/autopilot` already uses, and higher stakes here: this eventually submits a real job
  application, not only an unattended CV rewrite). Enqueues one `auto_apply_queue` row for
  the caller and the named job, after refusing:
  - a job whose `source` is not `greenhouse` (the only ATS this feature resolves a résumé
    upload field for today, per `auto-apply-tailored-resume`'s own scope decision),
  - a caller not on the PRO plan tier (`users.pro_until` via `plan.TierOf`) — a deliberate,
    explicit exception to this repo's own "a plan differs in how much of a feature it
    allows, never whether the feature exists" convention; see design.md,
  - a caller with no base CV yet.

  A second submission for the same `(user, job)` is idempotent — the existing
  `UNIQUE (user_id, job_id)` on `auto_apply_queue` is the dedup key — except a row already
  `review_decision = 'declined'`, which refuses permanently (matches
  `auto-apply-tailored-resume`'s own "a decline is terminal" convention: `Park` has no
  automatic path back).
- On a fresh enqueue, publishes `auto-apply/submit` (`queueId`) to the self-hosted Inngest
  event API — extending the existing `autoApplyEventPublisher` interface
  (`internal/api/handler/auto_apply_review_publish.go`, which already publishes
  `auto-apply/review.decided`) with a second method on the same `inngestEventPublisher`,
  rather than a second publisher.
- New "Auto-apply" button in `web/src/lib/components/JobView.svelte`, beside the existing
  external "Apply" link, rendered only when `job.source === 'greenhouse'`. States: idle →
  disabled+spinner on click → a toast plus a persistent "queued" state, or a permanently
  disabled "declined earlier" state — driven by a new field on the job detail response
  naming the caller's own auto-apply status for that job, so the button renders correctly
  on page load without an extra request.

## Capabilities

### New Capabilities
- `auto-apply-enqueue`: the candidate-facing trigger that creates one durable
  auto-apply attempt for a `(candidate, job)` pair and starts the already-built tailor/
  review sequence — the gate (Greenhouse-only, PRO-only, requires a base CV), the
  dedup/decline-is-terminal rule, and the event publish that hands off to
  `cmd/auto-apply-orchestrate`.

### Modified Capabilities
(none — `auto-apply-cv-tailoring`, `atsapply-resume-upload`, and `auto-apply-orchestration`'s
own requirements are unchanged; this only adds the missing caller those capabilities already
assumed existed.)

## Impact

- **`internal/platform/db/queries/auto_apply_queue.sql`**: one new `:one` insert query
  (`ON CONFLICT (user_id, job_id) DO NOTHING RETURNING id`) and one read for the
  conflicting row — no new migration, `auto_apply_queue` and its unique constraint already
  exist (migration 0116).
- **`internal/api/handler`**: new handler (`PostJobAutoApply`) and route registration; the
  existing `autoApplyEventPublisher`/`inngestEventPublisher` in
  `auto_apply_review_publish.go` gain a `PublishSubmit` method.
- **`internal/job/jobview`**: the job wire shape gains one field naming the caller's own
  auto-apply status for that job (present only for an authenticated caller).
- **`web/src/lib/components/JobView.svelte`** and its types: the new button, its states,
  and the toast copy.
- **No changes** to `cmd/auto-apply-orchestrate`, the tailor/review endpoints, or
  `internal/application/autoapply` — this change is purely the missing trigger those
  already-shipped pieces assumed.
