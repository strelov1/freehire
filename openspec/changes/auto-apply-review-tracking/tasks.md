## 1. Schema

- [x] 1.1 New migration `0139_auto_apply_queue_resolved_preview.sql`:
      `auto_apply_queue.resolved_preview jsonb` (nullable). Added `SetAutoApplyResolvedPreview`,
      `GetApplicationStage`, extended `GetAutoApplyQueueEntryForJob` with
      `tailored_cv_id`/`unmapped`/`resolved_preview`, extended `ListUserJobs` with the raw
      auto-apply columns behind the board badge. `make sqlc` regenerated cleanly; `pnpm
      check:sql` clean on the new migration.

## 2. Answer-preview resolution (`internal/api/atsapply`, reused resolve logic, no drafting)

- [x] 2.1 Add `PreviewAnswers(fields []MergedField, answers map[string]string, hasApprovedCV
      bool) ResolvedPreview` (`preview.go`): calls `Resolve` (never `ResolveWithDrafting` — no
      LLM spend before the candidate approves anything), shapes resolved fields as
      `{label, value}` (résumé/file fields omitted — the tailored CV reference covers that
      separately), and shapes unmapped required fields as `{label, will_draft_at_submission}`
      using `draftable()`'s existing eligibility check (no LLM call).
- [x] 2.2 RED+GREEN: unit tests for `PreviewAnswers` — a resolved field is labelled, a field
      with no label falls back to its id, the résumé field is omitted rather than shown blank, a
      draftable unmapped field is marked `will_draft_at_submission`, a non-draftable (sensitive)
      one is not, an optional unanswered field is not reported at all.
- [x] 2.3 Add `PreviewClient` (`preview_client.go`) and `StoredFormReader` interface: for
      Greenhouse, launches a browser and scans the live form exactly like `Client.Submit` does
      (`newBrowserSession`, `renderedHTML`, `hasRecaptchaMarker`, `ScanGreenhouseForm`,
      `Reconcile`), returning a parked result (no error) for a captcha/unscannable page — the
      same outcomes `Client.Submit` already classifies. For every other provider, reads the
      schema from `StoredFormReader` when configured (the `apply_forms` row `cmd/capture-
      apply-form` already persists) rather than a live refetch, falling back to a live
      `fetchSchema`-equivalent fetch only when no reader is configured or no row exists yet.
- [x] 2.4 RED+GREEN: `PreviewClient.Preview` unit tests — a captcha-listed provider
      (`requiresCaptcha`) parks without touching a browser or the form reader; a configured
      `StoredFormReader` is preferred over a fetch for a non-Greenhouse provider (fake fetcher
      asserts it is never called when the reader has a row).

## 3. `cmd/auto-apply`: second claim pass for preview resolution

Runs in the SAME worker as the existing submit pass, not the orchestrator — see design.md's
"The preview is computed once..." decision for why `cmd/auto-apply-orchestrate` (no database,
no browser, by its own existing design) is the wrong place for this.

- [x] 3.1 New sqlc query `ClaimAutoApplyPreviewBatch`: leases entries with `tailored_cv_id IS
      NOT NULL AND resolved_preview IS NULL AND review_decision IS NULL AND blocked_at IS NULL
      AND failed_at IS NULL`, reusing the existing `claimed_at` lease column (safe to share with
      `ClaimAutoApplyBatch`'s own lease — the two predicates are mutually exclusive on
      `review_decision`, so an entry is never claimable by both at once). New sqlc query
      `SetAutoApplyResolvedPreview` (already added in §1) is the write.
- [x] 3.2 In `internal/application/autoapply`, add `RunPreviews(ctx, store, answers, sidecar,
      opts) (PreviewStats, error)`, mirroring `Run`'s own `outbox.RunPool`-based shape: a new
      `PreviewSidecar` interface (`Preview(ctx, Claimed, answers) (PreviewResult, error)`,
      implemented by `atsapply.PreviewClient`), and `Store` gains `ClaimForPreview`/`SetPreview`
      alongside its existing `Park`/`Fail` (reused as-is for a preview-pass failure/park).
- [x] 3.3 RED+GREEN: `RunPreviews` unit tests against a fake store/sidecar — a successful
      preview call results in `SetPreview` called with the sidecar's result; a parked result
      calls `Park` with no unmapped fields (a form-level park, not a field-level one) and does
      not call `SetPreview`; a sidecar error is treated as a transient failure (`Fail`), same as
      `Run`'s own handling.
- [x] 3.4 `SetPreview`'s implementation also records the "ready for review" notification
      (`RecordNotification`, best-effort — see §8), replacing the notification call this task
      removes from `PostAutoApplyTailor` — see §8.1.
- [x] 3.5 Wire `autoapply.RunPreviews` into `cmd/auto-apply/main.go`, alongside the existing
      `autoapply.Run` call, using the same `atsapply.PreviewClient` (constructed alongside the
      existing `atsapply.Client`) and the same `answers`/`RunOptions`.

## 4. Enqueue: put the job on the tracker board

- [x] 4.1 RED: `auto-apply-submit-trigger`'s enqueue handler, given a job not yet tracked by the
      caller, results in a tracked entry at stage `preparing` after enqueue.
- [x] 4.2 RED: given a job already tracked at some other stage, enqueue leaves that stage
      unchanged.
- [x] 4.3 GREEN: call `jobtracking.Service.Track` from the enqueue handler
      (`ensureTrackedForAutoApply`), gated by a new `GetApplicationStage` read so an already-set
      stage is never overwritten (`TrackJob`'s own `COALESCE(EXCLUDED.stage, applications.stage)`
      would otherwise reset an in-progress application back to `preparing`).

## 5. Status derivation + tracked-job read path

- [x] 5.1 In `internal/application/autoapply`, add the six-value status derivation
      (`tailoring`/`pending_review`/`approved`/`blocked`/`declined`/`failed`) as a pure function
      over `(tailoredCVID, reviewDecision, blockedAt, failedAt)`, mirroring
      `autoApplyEntryStatus`'s existing precedence (declined checked before blocked/failed).
- [x] 5.2 RED+GREEN: table-driven unit test, one case per status plus the declined-vs-blocked
      precedence case from the spec.
- [x] 5.3 Add an assembly function returning `{status, resolved_preview, tailored_cv_id,
      unmapped}` for a job's live auto-apply attempt (never `last_error`), `nil` when none exists.
- [x] 5.4 RED+GREEN: wire this into wherever `stage_suggestion` is assembled onto a tracked job
      today, as a new optional `auto_apply` field, integration-tested alongside the existing
      `stage_suggestion` test for that same read path.

## 6. Review action (reused endpoint, new caller)

- [x] 6.1 Confirm `web/src/lib/api.ts` has (or add) a wrapper for the existing
      `POST /me/auto-apply/:queueId/review`.

## 7. Frontend: tracker UI

- [x] 7.1 Add the `auto_apply` field to the tracked-job type in `web/src/lib/types.ts` (or
      generated contracts, per this repo's existing convention).
- [x] 7.2 `BoardCard.svelte`: a small "needs your review" badge, same visual family as the
      existing silence badge, shown when `auto_apply.status` is `pending_review` or `blocked`.
- [x] 7.3 `JobDrawer.svelte`: new banner section for `pending_review` — answer preview (label/
      value list), tailored CV link, Approve/Decline buttons calling the endpoint from §6, with
      optimistic status update on success.
- [x] 7.4 `JobDrawer.svelte`: read-only banner variant for `blocked` (renders the `unmapped`
      question list), `declined`, and `failed` — copy makes clear the attempt is final for this
      job, no retry implied.
- [x] 7.5 Unit-test the status-to-badge/banner-variant mapping, following
      `autoApplyButton.test.ts`'s pattern.

## 8. Notification: relocate from tailor-completion to preview-ready

- [x] 8.1 Remove the "tailoring ready" `RecordNotification` call from `PostAutoApplyTailor`
      (`auto_apply_tailor.go`) — it fired too early (before there is a preview to show) and at
      the wrong target (`/tailor/[slug]`, which has no approve/decline affordance).
- [x] 8.2 Add the notification call to `SetPreview`'s implementation (§3.4) instead, targeting
      `auto_apply_ready_for_review` with the job's tracker slug.
- [x] 8.3 `notificationTarget.ts`: map `auto_apply_ready_for_review` to `{kind: 'tracking',
      slug}` (widening the existing `tracking` variant, which `nudge_follow_up`/
      `nudge_interview_prep` already use with no slug).
- [x] 8.4 **No new deep-link mechanism needed** — `/my/tracking/[id]` already exists and
      already does exactly this (`JobBoard.svelte`'s own `initialId` prop, built for the
      inbox's mail-linking feature): it server-fetches the board and opens the given
      application's drawer once loaded. `NotificationCard.svelte`'s href derivation routes a
      `tracking` target WITH a slug there instead of inventing a `?job=` query param.

## 9. Verification

- [ ] 9.1 `gofmt -l .` clean on every touched file.
- [ ] 9.2 `go vet ./...` and `go test ./...` green.
- [ ] 9.3 `go vet -tags=integration ./...` green.
- [ ] 9.4 `go test -tags=integration ./...` green (this change alters behavior — a new claim
      pass in `cmd/auto-apply`, a new tracked-job field, a new enqueue side effect, a relocated
      notification — not just a signature).
- [ ] 9.5 Frontend: `pnpm check` green.
- [ ] 9.6 Manual: drive the golden path in the browser — trigger auto-apply, see the job appear
      on the board at `preparing`, see the badge once `pending_review`, open the drawer, review
      the answer preview, approve, confirm the notification links correctly, confirm a `blocked`
      entry renders its unmapped questions with no retry affordance.
