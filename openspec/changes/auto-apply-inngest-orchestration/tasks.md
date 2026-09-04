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

- [ ] 2.1 Add `github.com/inngest/inngestgo` to `go.mod`.
- [ ] 2.2 New long-lived worker (`Restart=always`, mirrors `cmd/mail-ingest`'s own shape —
      not a `worker.Main` run-once-and-exit binary). Wires the shared secret from config, an
      HTTP client to `internal/platform/safehttp`'s transport, and the Inngest client
      (`Dev: nil` — a real signing key/event key from config, never the dev-mode bypass this
      session's spike used).
- [ ] 2.3 RED (integration, real Inngest dev server + a fake `hire` HTTP server standing in
      for the two endpoints): a submitted entry calls the tailor endpoint with the shared
      secret, in the URL path this repo's own routes expect.
- [ ] 2.4 RED: a tailor call returning non-200 ends the run without ever calling the review
      endpoint.
- [ ] 2.5 RED: after a successful tailor call, the run is durably paused — no review call
      happens until a matching `auto-apply/review.decided` event arrives.
- [ ] 2.6 RED: a decision event for a DIFFERENT `queueId` does not resume this run (assert no
      review call happens for it).
- [ ] 2.7 RED: a matching decision event resumes the run and calls the review endpoint with
      that decision.
- [ ] 2.8 RED: a pause that exceeds its own timeout ends the run without a review call and
      without retrying tailor.
- [ ] 2.9 GREEN: implement the function to satisfy 2.3–2.8.

## 3. `PostAutoApplyReview`: publish the decision event

- [ ] 3.1 RED: recording a decision publishes `auto-apply/review.decided` (`queueId`,
      `decision`) to the configured Inngest event API via `safehttp`.
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
      real traffic.
