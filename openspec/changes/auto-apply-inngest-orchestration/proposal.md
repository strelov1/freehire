## Why

`openspec/changes/auto-apply-tailored-resume` built freehire's own half of the tailor →
candidate-review sequence (`POST /me/auto-apply/:queueId/{tailor,review}`) on the assumption
that "a separate, external automation pipeline" would call them in order. That pipeline was
never built, and nothing in or outside this repository invokes those endpoints in production
today. A spike this session (documented in this session's own transcript, not committed
anywhere else) evaluated three real orchestration engines — SuperPlane, Temporal, Inngest —
specifically for the property this sequence actually needs: a durable pause of unbounded
length between "tailoring finished" and "candidate decided," surviving process restarts.
Inngest's `step.WaitForEvent` was verified end to end against this repo's own endpoints and
a real LLM call in this session: a run paused, a signal sent minutes later resumed the SAME
run, and the decision was recorded in Postgres. It also self-hosts as one process that can
reuse freehire's existing Postgres, which fits the single-bare-metal-host, no-Kubernetes
deploy model the rest of the fleet already uses (see `deploy/AGENTS.md`).

This change makes that spike a real, freehire-owned worker: something finally calls the
tailor endpoint automatically, durably, in production, and durably waits for whichever
caller (today, only the candidate's own browser) records the review decision.

## What Changes

- New cron-independent worker, `cmd/auto-apply-orchestrate`: a long-lived process (like
  `cmd/mail-ingest`, not a run-once-and-exit cron job — an Inngest function must stay
  reachable for the SDK's own callback protocol) that hosts one Inngest function,
  `auto-apply-tailor-and-review`.
  - Triggered by event `auto-apply/submit` (data: `queueId`). Calls
    `POST /me/auto-apply/:queueId/tailor` via a freehire API key (the same surface an
    external pipeline was always meant to call — see that change's own design.md), as one
    `step.Run`.
  - Then durably waits (`step.WaitForEvent`, a multi-day timeout) for
    `auto-apply/review.decided` (data: `queueId`, `decision`) and completes with that
    decision — no second HTTP call: `PostAutoApplyReview` (below) already recorded the
    decision before publishing the event that resumes this wait, so calling it again would
    always be refused as an already-reviewed entry.
  - A tailor call that fails, or a wait that times out, ends the run without ever recording
    a review decision — the entry stays exactly where the existing endpoints already leave
    it (unreviewed, unclaimed), for a human to notice, matching every other queue in this
    codebase.
- `PostAutoApplyReview` (`internal/api/handler/auto_apply_tailor.go`) gains one best-effort
  side effect: after recording the candidate's decision, it publishes
  `auto-apply/review.decided` to the self-hosted Inngest event API so a waiting orchestrator
  run resumes. A publish failure is logged, never fails the request — the same
  never-block-the-user-facing-write convention `RecordNotification` already follows in this
  same handler.
- Self-hosted Inngest server added to `deploy/`: one more systemd-managed process on host-2,
  pointed at freehire's own Postgres (`inngest start --postgres-uri`, per the spike's own
  finding that this needs no dedicated datastore cluster) rather than a managed Inngest Cloud
  account.

Explicitly **not** part of this change:
- **What emits `auto-apply/submit` in the first place** — still `auto-apply-tailored-resume`
  tasks.md's own deferred item (2.1): a candidate-facing trigger, its own future change.
- **True mid-run, per-requirement pause inside the tailoring LLM loop** (the candidate hits
  "Datadog" specifically, mid-pass, and only that one requirement waits). This change only
  durably pauses BETWEEN the two existing atomic HTTP calls — the exact "between runs, never
  mid-run" boundary `auto-apply-tailored-resume`'s own design.md already drew, and confirmed
  unavoidable without one: an Inngest `step.Run` around `assistant.Runner.Run` is one opaque
  unit to Inngest regardless of which orchestrator sits on top, because the runner itself
  exposes no per-requirement checkpoint today. Widening that is a separate, larger,
  not-yet-decided change to `internal/ai/assistant`'s own runner.

## Capabilities

### New Capabilities
- `auto-apply-orchestration`: the durable event-driven sequencing of an auto-apply queue
  entry's tailor call and its candidate-review call, and what happens on failure or timeout
  at each step.

### Modified Capabilities
(none — `auto-apply-cv-tailoring` and `atsapply-resume-upload`'s own requirements are
unchanged; this only adds a caller for the first one's endpoints and a best-effort event
publish alongside the second one's own review-decision write, neither of which changes what
either capability itself guarantees.)

## Impact

- **New Go binary**: `cmd/auto-apply-orchestrate`, importing `github.com/inngest/inngestgo`
  (new module dependency).
- **`internal/api/handler/auto_apply_tailor.go`**: `PostAutoApplyReview` gains one best-effort
  call to publish an Inngest event; no change to its existing response shape, status codes,
  or the review-decision write itself.
- **`deploy/`**: one new systemd unit (self-hosted Inngest server) and its own environment
  file; `internal/platform/config` gains the settings the new worker and the event-publish
  call need (Inngest event API base URL/key, the orchestrator's own freehire API key).
- **Operational**: a new always-on process to deploy and monitor, alongside the existing
  fleet of run-once-and-exit cron workers and the two long-lived daemons
  (`cmd/server`, `cmd/mail-ingest`).
