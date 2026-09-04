## 1. Schema: enqueue query

- [x] 1.1 sqlc queries in `internal/platform/db/queries/auto_apply_queue.sql`:
      `EnqueueAutoApply` (`INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2)
      ON CONFLICT (user_id, job_id) DO NOTHING RETURNING id`) and
      `GetAutoApplyQueueEntryForJob` (by `user_id, job_id`, returning `id`,
      `review_decision`) for the conflict-read path. Run `make sqlc`.

## 2. Job wire shape: the caller's own auto-apply status

- [x] 2.1 RED: `jobview.Job` gains a caller-scoped field (e.g. `AutoApplyStatus *string`,
      values `"queued"`/`"declined"`, absent/nil for no attempt or an anonymous caller) —
      a test asserting it is populated for an authenticated caller with a live entry, with
      a declined entry, and omitted for no entry and for an anonymous caller, mirroring
      `MyVote`'s own caller-scoped shape and test coverage.
- [x] 2.2 GREEN: implement — one query keyed on `(caller_id, job_id)`, read the same way
      `MyVote` already is, wired at the same call site.

## 3. Enqueue endpoint

- [x] 3.1 RED: `POST /api/v1/jobs/:slug/auto-apply` refuses a job whose `source` is not
      `greenhouse` (400/404 — pick and justify the exact code against this repo's existing
      refusal conventions for a job-not-eligible case). Went with 400: the slug itself
      resolves (unlike a 404), the request is simply not valid for this job.
- [x] 3.2 RED: refuses a caller not on the PRO plan tier (402, mirrors
      `refuseNewTailoring`'s own 402 shape).
- [x] 3.3 RED: refuses a caller with no base CV (409, mirrors `TailorCV`'s own
      `ErrNoResume` → 409 mapping).
- [x] 3.4 RED: a fresh request for an eligible `(caller, job)` pair creates exactly one
      `auto_apply_queue` row.
- [x] 3.5 RED: a second request for the same pair, before any decision, succeeds without
      creating a second row (assert row count stays 1).
- [x] 3.6 RED: a request for a pair whose existing row is already
      `review_decision = 'declined'` is refused (409) and creates no new row.
- [x] 3.7 RED: `mw.cookie` only — a request bearing a live `api_keys` Bearer credential
      but no cookie is refused 401 (this route does not accept `mw.key`).
- [x] 3.8 GREEN: implement `PostJobAutoApply` (`internal/api/handler/auto_apply_enqueue.go`)
      — resolve job by slug, run the three eligibility checks in order (source, plan tier,
      base CV), then `EnqueueAutoApply`; on a fresh row (non-nil returned id), call the
      event-publish step (§4); on a conflict, read the existing row via
      `GetAutoApplyQueueEntryForJob` and branch on its `review_decision`. Registered with
      `mw.cookie` in `assistant.go`.

## 4. Publish `auto-apply/submit`

- [x] 4.1 RED: enqueuing a fresh row publishes `auto-apply/submit` (`queueId`) via the
      existing `autoApplyEventPublisher` — extended that interface
      (`internal/api/handler/auto_apply_review_publish.go`) with `PublishSubmit(ctx,
      queueID)`, alongside the existing `PublishReviewDecided`.
- [x] 4.2 RED: a repeat (idempotent, no-new-row) request does NOT publish a second
      `auto-apply/submit` for the same entry.
- [x] 4.3 RED: a publish failure is logged and does NOT change the endpoint's own
      response (still success, the row is already committed) — same best-effort contract
      `PostAutoApplyReview`'s own publish call already has.
- [x] 4.4 GREEN: implement `PublishSubmit` on `inngestEventPublisher` (shares a new `post`
      helper with `PublishReviewDecided` — same POST-to-event-API shape, different event
      name/payload); wired the best-effort call into `PostJobAutoApply` via `publishSubmit`.

## 5. Frontend: the Auto-apply button

- [ ] 5.1 Add `auto_apply_status` to the frontend `Job` type (`web/src/lib/types.ts`).
- [ ] 5.2 A small pure function deciding the button's rendered state from
      `(job.source, job.auto_apply_status)` — kept out of the component so it is testable
      without mounting Svelte, mirroring `notificationTarget.ts`'s own convention. Cover:
      not-greenhouse (button absent), no status (idle/enabled), `queued` (disabled,
      "queued" label), `declined` (disabled, "declined" label).
- [ ] 5.3 Wire the button into `JobView.svelte` beside the existing "Apply" link, calling
      `POST /api/v1/jobs/:slug/auto-apply` on click, showing a toast on success
      ("we're preparing a tailored resume, we'll let you know when it's ready to review")
      and flipping to the queued state; a refusal (402/409) surfaces its own message
      rather than the generic queued toast.
- [ ] 5.4 Manual: click through in a running dev server against a seeded Greenhouse job —
      PRO account (queues, toast, state persists on reload), free account (refused, no
      button state change), an already-declined entry (button rendered disabled on load).

## 6. Verification

- [ ] 6.1 `gofmt -l .` clean on every touched Go file; `pnpm lint`/`pnpm check` clean on
      every touched frontend file (per this repo's own web conventions).
- [ ] 6.2 `go vet ./...` and `go test ./...` green.
- [ ] 6.3 `go vet -tags=integration ./...` green.
- [ ] 6.4 `go test -tags=integration ./...` green — this change adds a new route and a new
      write path, not just a signature.
- [ ] 6.5 `pnpm --filter web test` (or this repo's equivalent) green for the new pure
      state-decision function.
