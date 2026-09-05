## 1. Schema

- [ ] 1.1 New migration: `auto_apply_queue.resolved_preview jsonb` (nullable). `sqlc` query
      additions as needed (set the resolved preview alongside/after setting `tailored_cv_id`;
      read it wherever the queue entry is read for status derivation). Run `make sqlc` after
      editing `internal/platform/db/queries/*.sql`.

## 2. Answer-preview resolution (reused, not new, resolve logic)

- [ ] 2.1 In `internal/api/atsapply`, add a function that produces a `ResolvedPreview` (field
      label -> resolved answer, no file contents) for a given job + candidate, calling the
      existing schema-fetch + `Resolve`/`ResolveWithDrafting` pipeline (`resolve.go`, `draft.go`)
      but stopping before `fillAndSubmit`. For Greenhouse, this performs the same live DOM render
      `Client.Submit` already does (`renderedHTML` → `ScanGreenhouseForm`); for the other three
      providers, prefer the stored `apply_forms` row (`GetApplyFormByJobID`) over a live refetch.
- [ ] 2.2 RED+GREEN: unit test this function against a fake/stored schema per provider
      (mirroring `resolve_test.go`'s existing fixtures) — asserts the returned preview matches
      what `Resolve` would produce, and that no browser call happens for non-Greenhouse
      providers.
- [ ] 2.3 RED+GREEN: a resolve failure (e.g. a Greenhouse render error) parks the entry the same
      way a resolve failure during the real submission already does, rather than leaving it
      silently stuck without ever reaching `pending_review`.

## 3. Orchestrator: resolve-preview step

- [ ] 3.1 In `cmd/auto-apply-orchestrate`'s `auto-apply-tailor-and-review` Inngest function, add
      a `step.Run` immediately after the existing tailor step and before the entry is marked
      `pending_review`: calls the new function from §2, persists `resolved_preview`.
- [ ] 3.2 RED+GREEN: integration test (or the orchestrator's own existing test harness) — a
      successful tailor step followed by a successful resolve-preview step results in
      `resolved_preview` set and the entry visible as `pending_review`; a resolve-preview failure
      results in the entry parked, never reaching `pending_review`.

## 4. Enqueue: put the job on the tracker board

- [ ] 4.1 RED: `auto-apply-submit-trigger`'s enqueue handler, given a job not yet tracked by the
      caller, results in a tracked entry at stage `preparing` after enqueue.
- [ ] 4.2 RED: given a job already tracked at some other stage, enqueue leaves that stage
      unchanged.
- [ ] 4.3 GREEN: call `jobtracking.Service.Track(ctx, userID, slug, ptr("preparing"), nil,
      "auto_apply")` from the enqueue handler, after the existing queue insert.

## 5. Status derivation + tracked-job read path

- [ ] 5.1 In `internal/application/autoapply`, add the six-value status derivation
      (`tailoring`/`pending_review`/`approved`/`blocked`/`declined`/`failed`) as a pure function
      over `(tailoredCVID, reviewDecision, blockedAt, failedAt)`, mirroring
      `autoApplyEntryStatus`'s existing precedence (declined checked before blocked/failed).
- [ ] 5.2 RED+GREEN: table-driven unit test, one case per status plus the declined-vs-blocked
      precedence case from the spec.
- [ ] 5.3 Add an assembly function returning `{status, resolved_preview, tailored_cv_id,
      unmapped}` for a job's live auto-apply attempt (never `last_error`), `nil` when none exists.
- [ ] 5.4 RED+GREEN: wire this into wherever `stage_suggestion` is assembled onto a tracked job
      today, as a new optional `auto_apply` field, integration-tested alongside the existing
      `stage_suggestion` test for that same read path.

## 6. Review action (reused endpoint, new caller)

- [ ] 6.1 Confirm `web/src/lib/api.ts` has (or add) a wrapper for the existing
      `POST /me/auto-apply/:queueId/review`.

## 7. Frontend: tracker UI

- [ ] 7.1 Add the `auto_apply` field to the tracked-job type in `web/src/lib/types.ts` (or
      generated contracts, per this repo's existing convention).
- [ ] 7.2 `BoardCard.svelte`: a small "needs your review" badge, same visual family as the
      existing silence badge, shown when `auto_apply.status` is `pending_review` or `blocked`.
- [ ] 7.3 `JobDrawer.svelte`: new banner section for `pending_review` — answer preview (label/
      value list), tailored CV link, Approve/Decline buttons calling the endpoint from §6, with
      optimistic status update on success.
- [ ] 7.4 `JobDrawer.svelte`: read-only banner variant for `blocked` (renders the `unmapped`
      question list), `declined`, and `failed` — copy makes clear the attempt is final for this
      job, no retry implied.
- [ ] 7.5 Unit-test the status-to-badge/banner-variant mapping, following
      `autoApplyButton.test.ts`'s pattern.

## 8. Notification target

- [ ] 8.1 Change the tailoring-complete notification's target from `/tailor/[slug]` to
      `/my/tracking?job=<id>` (`notificationTarget.ts`).
- [ ] 8.2 `web/src/routes/my/tracking/+page.svelte` (or `JobBoard.svelte`): on load, if `?job=`
      is present, open that job's drawer.

## 9. Verification

- [ ] 9.1 `gofmt -l .` clean on every touched file.
- [ ] 9.2 `go vet ./...` and `go test ./...` green.
- [ ] 9.3 `go vet -tags=integration ./...` green.
- [ ] 9.4 `go test -tags=integration ./...` green (this change alters behavior — a new
      orchestrator step, a new tracked-job field, a new enqueue side effect — not just a
      signature).
- [ ] 9.5 Frontend: `pnpm check` green.
- [ ] 9.6 Manual: drive the golden path in the browser — trigger auto-apply, see the job appear
      on the board at `preparing`, see the badge once `pending_review`, open the drawer, review
      the answer preview, approve, confirm the notification links correctly, confirm a `blocked`
      entry renders its unmapped questions with no retry affordance.
