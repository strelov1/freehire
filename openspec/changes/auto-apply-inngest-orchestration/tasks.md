## 1. Auth: shared orchestrator secret (replaces the abandoned per-user credential — see
   design.md's own Decisions and Alternatives considered)

- [x] 1.1 sqlc query `GetAutoApplyQueueEntryByID` (by `id` alone, no `user_id` filter) —
      what the two auto-apply routes read from when the caller is the orchestrator rather
      than the entry's own owner.
- [x] 1.2 RED: a request presenting a valid `Authorization: Bearer <AUTO_APPLY_ORCHESTRATOR_SECRET>`
      resolves the entry's owner FROM THE ENTRY (`GetAutoApplyQueueEntryByID`), not from any
      authenticated user id, and proceeds.
- [x] 1.3 RED: a request presenting no valid credential at all (no cookie, no live `api_keys`
      row, wrong or missing secret) is refused 401 — the new gate must not weaken the
      existing refusal contract.
- [x] 1.4 RED: a request presenting a human credential (cookie or a live `api_keys` row)
      still resolves via the existing ownership-scoped `GetAutoApplyQueueEntryForReview`,
      unaffected by the new gate — the secret path is a fallback, not a replacement.
- [x] 1.5 RED: the process-wide rate limiter (`internal/api/ratelimit`, a single fixed key —
      not per-user, not per-IP) bounds requests authenticated via the shared secret; a
      human-authenticated request on the same routes is unaffected by it.
- [x] 1.6 GREEN: implement the route-local gate (`internal/api/handler`, not the `auth`
      package — this is not a user identity), the entry-resolution branch in
      `PostAutoApplyTailor`/`PostAutoApplyReview`, and the rate limiter wiring.

## 2. `cmd/auto-apply-orchestrate`: the Inngest function

- [x] 2.1 Add `github.com/inngest/inngestgo` to `go.mod`.
- [x] 2.2 New long-lived worker (`cmd/auto-apply-orchestrate/main.go`, `Restart=always`,
      mirrors `cmd/mail-ingest`'s own shape — `worker.Main` without `worker.Bootstrap`,
      since this binary needs no database at all). Wires the shared secret from config, a
      plain `http.Client` with a bounded timeout (NOT `internal/platform/safehttp`), and
      the Inngest client (`Dev` unset — real signing/event keys from config,
      `ServeWithOpts(EnableUnauthedSync: true)` since this instance is internal-only and
      single-tenant). Self-registers at startup by PUTting its own `/api/inngest`
      endpoint — the same handshake 2.9's integration tests drive by hand. Any of
      `AUTO_APPLY_ORCHESTRATOR_SECRET`/`INNGEST_EVENT_API_URL`/`INNGEST_EVENT_KEY`/
      `INNGEST_SIGNING_KEY` unset fails startup (exit 1) rather than serving a function
      that could never authenticate anywhere.
- [x] 2.3 RED (integration, real Inngest dev server + a fake `hire` HTTP server standing in
      for the two endpoints): a submitted entry calls the tailor endpoint with the shared
      secret, in the URL path this repo's own routes expect.
- [x] 2.4 RED: a tailor call returning non-200 ends the run without ever calling the review
      endpoint.
- [x] 2.5 RED: after a successful tailor call, the run is durably paused — no review call
      happens until a matching `auto-apply/review.decided` event arrives.
- [x] 2.6 RED: a decision event for a DIFFERENT `queueId` does not resume this run (assert no
      review call happens for it). This is the test that caught 2.9's own real bug — see
      its own note.
- [x] 2.7 RED: a matching decision event resumes the run and calls the review endpoint with
      that decision.
- [x] 2.8 RED: a pause that exceeds its own timeout ends the run without a review call and
      without retrying tailor.
- [x] 2.9 GREEN: implement the function to satisfy 2.3–2.8
      (`internal/application/autoapplyorchestrate`). Found and fixed a real bug 2.6 alone
      caught: `step.WaitForEvent`'s own CEL "if" expression binds the candidate event as
      `async`, not `event` — this session's own earlier spike (and this package's first
      draft) used `event.data.queueId == "..."`, which silently matched EVERY
      `auto-apply/review.decided` event rather than failing to parse, so a decision for ANY
      entry would have resumed EVERY paused run. The proposal.md's "verified end to end"
      spike claim only ever exercised the matching case; the non-matching case was never
      actually checked until 2.6. Corrected to `async.data.queueId == "..."`.

## 3. `PostAutoApplyReview`: publish the decision event

- [ ] 3.1 RED: recording a decision publishes `auto-apply/review.decided` (`queueId`,
      `decision`) to the configured Inngest event API via a plain `http.Client` (see
      design.md's own Decisions on why not `safehttp`).
- [ ] 3.2 RED: a publish failure is logged and does NOT change the endpoint's existing
      response (still 200 with the decision, per `auto-apply-tailored-resume`'s own
      contract) — construct the fake event publisher to always error and assert the response
      is unaffected.
- [ ] 3.3 GREEN: implement, reusing the existing `RecordNotification`-style
      "best-effort, log and continue" shape already in this same handler file.

## 4. Config and deploy

- [ ] 4.1 `internal/platform/config`: `INNGEST_EVENT_API_URL`, `INNGEST_EVENT_KEY`,
      `INNGEST_SIGNING_KEY` (the worker's own callback auth), and
      `AUTO_APPLY_ORCHESTRATOR_SECRET` (shared between `cmd/server` and
      `cmd/auto-apply-orchestrate` — see section 1). Follow the existing `LLM_ADMIN_*`
      pattern of "empty/unset degrades, never panics" ONLY where that is safe: the
      orchestrator worker itself has no useful degraded mode (see design.md's Non-Goals — it
      either runs against a real Inngest instance or is not deployed), so its own missing
      config should fail its startup loudly. `AUTO_APPLY_ORCHESTRATOR_SECRET` unset on the
      server side must simply mean the secret-auth path never matches (nothing to fail
      loudly about — the two routes still work for human callers).
- [ ] 4.2 `deploy/`: new systemd unit for `cmd/auto-apply-orchestrate` (`Restart=always`,
      mirrors `cmd/mail-ingest`'s unit) and for the self-hosted Inngest server
      (`inngest start --postgres-uri <freehire Postgres, a NEW database>`), plus their env
      files. Per `deploy/AGENTS.md`: this only edits the checked-in unit files — copying them
      to host-2 and enabling them is a separate, manual step, not part of this task.

## 5. Verification

- [ ] 5.1 `gofmt -l .` clean on every touched file.
- [ ] 5.2 `go vet ./...` and `go test ./...` green.
- [ ] 5.3 `go vet -tags=integration ./...` green.
- [ ] 5.4 `go test -tags=integration ./...` green.
- [ ] 5.5 Manual: one `auto-apply/submit` event, published by hand against the deployed
      self-hosted Inngest instance (mirroring this session's own spike), observed end to end
      through a real tailor call, a real pause, a real decision, and a real review call —
      before task 2.1's future trigger (in `auto-apply-tailored-resume`) ever emits one for
      real traffic. **This is the ONLY real verification of the signed, non-dev
      registration handshake** (`cmd/auto-apply-orchestrate`'s actual production client
      construction: real `INNGEST_SIGNING_KEY`/`INNGEST_EVENT_KEY`, `ServeWithOpts` with
      `EnableUnauthedSync`): 2.9's own integration suite runs entirely against `inngest
      dev` in its default unsigned mode, and a same-shaped test built with real (if
      arbitrary) signing/event keys against that SAME `inngest dev` image failed with a
      401 the executor's own callback could not get past — `inngest dev` does not emulate
      `inngest start`'s (the actual self-hosted production command per design.md's own
      Decisions) signed callback behavior closely enough to validate it. Standing up a
      real `inngest start` deployment (its own Postgres, its own signing-key handshake)
      to test this in CI is out of scope for this change; this manual step is where that
      risk actually gets retired before task 2.1's trigger sends real traffic through it.
