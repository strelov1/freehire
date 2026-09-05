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

- [x] 3.1 RED: recording a decision publishes `auto-apply/review.decided` (`queueId`,
      `decision`) to the configured Inngest event API via a plain `http.Client` (see
      design.md's own Decisions on why not `safehttp`).
- [x] 3.2 RED: a publish failure is logged and does NOT change the endpoint's existing
      response (still 200 with the decision, per `auto-apply-tailored-resume`'s own
      contract) — construct the fake event publisher to always error and assert the response
      is unaffected.
- [x] 3.3 GREEN: implement, reusing the existing `RecordNotification`-style
      "best-effort, log and continue" shape already in this same handler file
      (`internal/api/handler/auto_apply_review_publish.go`, `assistantHandlers.events`).

## 4. Config and deploy

- [x] 4.1 `internal/platform/config`: `INNGEST_EVENT_API_URL`, `INNGEST_EVENT_KEY`,
      `INNGEST_SIGNING_KEY`, `AUTO_APPLY_ORCHESTRATOR_SECRET` and
      `AUTO_APPLY_ORCHESTRATE_PORT` (the worker's own callback listen port). The
      orchestrator worker (`cmd/auto-apply-orchestrate`) fails its own startup loudly on
      any of the four required values missing; `AUTO_APPLY_ORCHESTRATOR_SECRET` unset on
      the server side simply means the secret-auth path never matches, and
      `INNGEST_EVENT_API_URL`/`INNGEST_EVENT_KEY` unset there disables the review-decided
      publish (see 3.3) — neither degrades the server's own startup.
- [x] 4.2 `deploy/`: `freehire-auto-apply-orchestrate.service` (`Restart=always`, mirrors
      `freehire-mail-ingest.service`, built from `hire-current` — added to
      `deploy/bin/release.sh`'s own build list and long-lived-daemon restart line, the
      same two places `mail-ingest` is, per that script's own comment about what happens
      when a unit is missing from them) and `freehire-inngest.service` (the self-hosted
      Inngest server itself — `inngest start`, not built by this repo, mirrors
      `freehire-logo.service`'s "third-party binary in /opt/freehire/bin" shape rather
      than a `hire-current` one). No new env FILE — `AUTO_APPLY_ORCHESTRATOR_SECRET` /
      `INNGEST_*` join the one shared `/opt/freehire/.env` every non-mail worker already
      reads. Per `deploy/AGENTS.md`: this only edits the checked-in unit files — copying
      them to host-2, provisioning the self-hosted Inngest server's own Postgres database,
      and enabling both units is a separate, manual step, not part of this task.

## 5. Verification

- [x] 5.1 `gofmt -l .` clean on every touched file.
- [x] 5.2 `go vet ./...` and `go test ./...` green.
- [x] 5.3 `go vet -tags=integration ./...` green.
- [x] 5.4 `go test -tags=integration ./...` green (195/195 packages, whole module —
      includes the new `internal/application/autoapplyorchestrate` and the extended
      `internal/api/handler` integration suites).
- [x] 5.5a Manual, done locally this session: a REAL self-hosted `inngest start` (not
      `inngest dev`) against its own local Postgres database, with real signing/event
      keys, `cmd/server` and `cmd/auto-apply-orchestrate` both built and run as plain
      processes against a local Postgres seeded with one user/CV/job/queue-entry row,
      exactly the deploy-model shape (not testcontainers, not the dev-mode image). A hand
      PUT self-registration (matching `cmd/auto-apply-orchestrate`'s own `selfRegister`)
      wrote a real row into `inngest`'s own `apps`/`functions` tables — the signed,
      non-dev registration handshake 2.9's own suite could not exercise (that suite runs
      against `inngest dev`, whose unsigned callback path does not emulate `inngest
      start`'s closely enough — see the note this replaces). One hand-published
      `auto-apply/submit` event then ran the REAL sequence: a real ~50s tailor call landed
      on `PostAutoApplyTailor` and wrote `tailored_cv_id`; the run paused (`function_runs`
      showed no further progress until a decision arrived — confirmed no premature review
      call); `POST /me/auto-apply/1/review` recorded the decision AND published
      `auto-apply/review.decided` to the real self-hosted instance (visible in its own
      logs); the paused `step.WaitForEvent` received it and resumed, calling
      `PostAutoApplyReview` again — which answered 409 (already reviewed). **This was
      misdiagnosed at the time as a testing artifact** (recording the decision directly
      rather than only through the orchestrator); a later code review caught that it is
      not one — `PostAutoApplyReview` is the ONLY thing that ever publishes
      `auto-apply/review.decided`, and it always records the decision before publishing,
      so this same 409 fires on every real run regardless of who calls `/review`. Fixed by
      removing the orchestrator's own post-wait call to `/review` entirely: the event's
      arrival already means the decision is durably recorded, so the run has nothing left
      to do but complete with it (`orchestrate.go`'s own doc comment explains why). One
      environment quirk found and worked around, not a code bug:
      `cmd/auto-apply-orchestrate`'s own `selfRegister` PUTs `127.0.0.1<addr>`, whose Host
      header becomes the URL the Inngest server calls back — correct when both processes
      share one host's `localhost` (the real host-2 deploy shape), wrong when the Inngest
      server runs in Docker and the orchestrator runs on the bare host (this local
      verification's own shape only) — worked around by re-issuing the self-registration
      PUT with an explicit `Host: host.docker.internal:<port>` header.
- [ ] 5.5b Still open: the SAME check against the actual host-2 deployment (real systemd
      units, real `AUTO_APPLY_ORCHESTRATOR_SECRET`, both processes on one real host where
      5.5a's Docker-vs-bare-host quirk cannot occur) — before task 2.1's future trigger
      (in `auto-apply-tailored-resume`) ever emits a real `auto-apply/submit` for real
      traffic. 5.5a already retired the specific risk this task was originally written to
      close (the signed registration handshake, unverifiable against `inngest dev`); 5.5b
      is what confirms the checked-in systemd units themselves are wired correctly on the
      real host, not a substitute for it.
