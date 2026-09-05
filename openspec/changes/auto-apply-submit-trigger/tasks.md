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

- [x] 5.1 Add `auto_apply_status` to the frontend `Job` type — turned out to need no manual
      edit: `web/src/lib/generated/contracts.ts` is generated from the Go struct
      (`make gen-contracts`), and regenerating after jobview.go's own field picked it up
      automatically, comment and all.
- [x] 5.2 A small pure function (`web/src/lib/autoApplyButton.ts`,
      `autoApplyButtonState(source, status)`) deciding the button's rendered state — kept
      out of the component so it is testable without mounting Svelte, mirroring
      `notificationTarget.ts`'s own convention. Covers: not-greenhouse (hidden), no status
      (idle), `queued` (disabled), `declined` (disabled).
- [x] 5.3 Wired the button into `JobView.svelte` beside the existing "Apply" link, calling
      `POST /api/v1/jobs/:slug/auto-apply` on click. Used the existing inline-banner
      pattern `justApplied` already establishes on this same page, not a toast — this
      repo's job page has no toast primitive, and matching the established idiom beat
      introducing one. Idle → click → disabled + a success banner + flips to the queued
      state locally; a refusal surfaces the backend's own error message as inline text
      rather than the success banner.
- [x] 5.4 Manual, done locally this session (rebuilt `app`+`web` images, real Postgres,
      real cookies for a PRO and a free-tier user): confirmed via direct API calls (with
      cookie) that `auto_apply_status` is correct (`declined`/absent) once authenticated,
      and confirmed the click→queue backend path end-to-end (already covered by 3.4's own
      integration test). **Found a real, pre-existing limitation, not a defect in this
      change**: `web/src/routes/jobs/[slug]/+page.server.ts` calls `serverApi(fetch)` with
      no cookie, so the job page's SERVER render is always anonymous — `auto_apply_status`
      (like the already-shipped `my_vote`, same call site, same omission) is absent on a
      first load/reload and only reflects the caller's own state after a client-side
      interaction on that same page view, not after a reload. Confirmed with the user this
      session: leave as-is, matching `my_vote`'s own existing behavior — not something this
      change should fix on its own.

## 6. Verification

- [x] 6.1 `gofmt -l .` clean; `pnpm lint`/`pnpm check` clean on every touched frontend file
      (2 pre-existing `no-shadow` warnings remain in `JobView.svelte`, unrelated to this
      change's own code — confirmed by line number against the pre-change file).
- [x] 6.2 `go vet ./...` and `go test ./...` green.
- [x] 6.3 `go vet -tags=integration ./...` green.
- [x] 6.4 `go test -tags=integration ./...` green — 195/195 packages, whole module.
- [x] 6.5 `pnpm test` (web) green — 1383/1383 tests, 121/121 files, including the new
      `autoApplyButton.test.ts`.
