## 1. Schema

- [x] 1.1 New migration on `auto_apply_queue`: `tailored_cv_id uuid REFERENCES cvs(id)`
      (nullable), `reviewed_at timestamptz` (nullable), `review_decision text` (nullable,
      CHECK IN ('approved','declined')). `sqlc` query additions as needed (set tailored CV,
      record a review decision, and widen `Claim`'s WHERE to
      `tailored_cv_id IS NOT NULL AND review_decision = 'approved'`). Run `make sqlc` after
      editing `internal/platform/db/queries/*.sql`.

## 2. Enqueue preflight gate (auto-apply-cv-tailoring: "Enqueuing reuses the preflight check")

- [ ] 2.1 **DEFERRED — confirmed during implementation (2026-09-04) that no enqueue trigger
      exists yet.** `internal/platform/db/queries/auto_apply_queue.sql` has no `INSERT`; the
      only write is a raw SQL statement in `auto_apply_queue_integration_test.go`. What
      populates `auto_apply_queue` is out of scope for `auto-apply-worker` per its own
      proposal.md and is not built anywhere else. This task assumed a trigger to route
      through — there is none. Deferred until that trigger (candidate action vs. standing
      rule, and where it lives) is designed, either in a follow-up change or as part of
      whatever calls the endpoints this change adds. Product direction (confirmed with the
      user 2026-09-04): the trigger is a button on the job posting (JD) page, for postings
      from an ATS this feature supports (Greenhouse today) — scope this as its own
      openspec change with its own proposal/design/tasks, touching `web/` and a new
      enqueue endpoint; not built as part of this change. All other tasks in this change do not
      depend on 2.1.

## 3. Queue-scoped tailoring endpoint

- [x] 3.1 RED: handler test — `POST /me/auto-apply/:queueId/tailor` refuses a foreign queue
      entry as not-found (mirrors `TailorCV`'s own ownership check).
- [x] 3.2 RED: refuses an entry whose review has already been recorded.
- [x] 3.3 RED: refuses when the caller's daily `tailor` plan allowance is spent, before any CV
      or session is created. (Integration-level against the real `plan.Store`, `.Enforcing()`
      — mirrors `TestTailorCVOutOfCredits`' own setup — rather than a fake `plan` port: the
      gate is `h.cv.refuseNewTailoring`, reused as-is, so there is no new plan-facing logic
      here to isolate with a fake.)
- [x] 3.4 GREEN: implement the endpoint — `mw.key`, resolves the queue entry's own vacancy,
      calls `cv.Store.Tailor`, constructs/derives the tailoring session the same way
      `TailorCV`'s bootstrap does, then calls `assistant.Runner.Run` with `TurnConfig{MaxSteps:
      autopilotMaxSteps}` and a no-op `emit` (events dropped; the transcript still persists via
      `Runner.Run` itself) — extracted `runAutopilotToCompletion` (assistant.go), the shared
      pre-run/run body, out of `PostAssistantAutopilot`'s own stream callback so both call
      sites run the same code. Responds with the tailored CV id and its `autopilot_report`.
- [x] 3.5 RED+GREEN: notification on completion, linking to `/tailor/[slug]` (the tailoring
      workspace idempotently resolves the same tailored CV a reload does — no `?cv=` param
      needed). Direct `db.Queries.RecordNotification` call, best-effort, mirroring how every
      background engine (`internal/engage/notify`, `/nudge`) already records one alongside
      its own delivery — there was no existing synchronous-HTTP-triggered precedent to reuse,
      so this is the narrowest addition matching that convention rather than a new engine.

## 4. Review-decision endpoint

- [x] 4.1 RED: `POST /me/auto-apply/:queueId/review` refuses a foreign entry as not-found.
- [x] 4.2 RED: refuses an entry with no tailored CV yet, and an entry already reviewed.
- [x] 4.3 RED: approving sets `review_decision='approved'`, `reviewed_at`, and makes the entry
      show up in `autoapply.Store.Claim`'s results (integration-level, per that package's own
      test conventions).
- [x] 4.4 RED: declining calls the SAME `Park` `internal/application/autoapply` already uses,
      with a reason distinct from an unresolved form field (e.g. "candidate declined the
      tailored CV") — assert `blocked_at` is set and the entry is absent from `Claim`'s results.
      (`DeclineAutoApplyReview` sets the same `blocked_at`/`last_error` fields
      `MarkAutoApplyBlocked` does, in one statement — reusing that park vocabulary rather than
      calling through `internal/application/autoapply.Store`, which the API handler layer has
      no reason to depend on.)
- [x] 4.5 GREEN: implement. Plus one more RED+GREEN beyond the plan: a decision cannot be
      recorded twice (`review_decision IS NULL` guard, race-safe via affected-row-count).

## 5. `internal/application/autoapply`: carry the approved CV

- [x] 5.1 RED: `Claimed` gains a field for the approved tailored CV reference; a fake `Store`
      test asserts `Claim` only returns entries with both `tailored_cv_id` set and
      `review_decision='approved'`. (Written at the SQL/integration layer — `Claim` is a
      thin generated-query wrapper with no domain logic of its own to fake-test; the real
      predicate is exercised in `internal/platform/db/auto_apply_queue_integration_test.go`.)
- [x] 5.2 GREEN: implement; update every existing fake/test construction of `Claimed`. (No
      existing construction needed changes — all are keyed struct literals, which compile
      fine against an added field.)

## 6. `internal/api/atsapply`: résumé file-upload resolution

- [x] 6.1 RED: `resolve.go` — a `file`-kind field whose label/id identifies it as the résumé
      upload resolves (rather than staying unmapped) when the `Claimed` carries an approved CV
      reference; a cover-letter (or other non-résumé) file field still does not.
- [x] 6.2 GREEN: implement the résumé-field recognition + resolution.
- [x] 6.3 RED: `Client.Submit` — a render failure for the approved CV parks the attempt
      (naming the résumé field), rather than being retried as a transient failure.
- [x] 6.4 GREEN: render the approved CV via the existing Typst renderer to a temp file on
      render success; wire the park path on failure.
- [x] 6.5 GREEN (no unit test — `fill.go`'s fill/submit path is this package's own documented
      "least-verified," no fakeable browser): add `chromedp.SetUploadFiles(selector, []string{
      path})` for the resolved résumé field in `fillAndSubmit`, removing the temp file after
      the browser session closes.

## 7. Verification

- [x] 7.1 `gofmt -l .` clean on every touched file.
- [x] 7.2 `go vet ./...` and `go test ./...` green.
- [x] 7.3 `go vet -tags=integration ./...` green (this change touches
      `internal/api/handler`, which the AGENTS.md warns holds most of the integration-tagged
      tests that only this command catches).
- [x] 7.4 `go test -tags=integration ./...` green — this change alters behavior (a new
      claimable predicate, a new endpoint), not just a signature, so the full tagged suite is
      warranted per the repo's own stated policy. Confirmed clean: 194/194 packages ok, zero
      failures.
- [x] 7.5 Manual: NOT exercised — cannot, from this repo. The real external pipeline is a
      separate system this repo has no access to, and a live Greenhouse submission needs this
      package's own least-verified path (`fill.go`, no fakeable browser, per its own AGENTS.md
      "least-verified" note). Stated plainly rather than claiming an end-to-end pass. What IS
      verified, end to end, against a real Postgres: both new endpoints (§3, §4) via HTTP
      through a real `assistant.Runner`/`plan.Store`, the widened `ClaimAutoApplyBatch`
      predicate, and the résumé-field render/park logic (unit-level, real Typst renderer via
      `cv.ResolveTemplate`'s embedded templates). Not verified anywhere: the Greenhouse DOM
      fill step actually finding and uploading through a real file-input selector — `fill.go`'s
      own long-standing gap, unchanged by this addition beyond the new "file" case.
