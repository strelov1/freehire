## Why

`auto_apply_queue` submissions currently always park on a required résumé/cover-letter file
field — `internal/api/atsapply`'s `resolve.go` never matches a `file`-kind field, so an
attempt that would otherwise be fully resolvable stops there. Attaching a real file means
having a CV tailored for that specific vacancy, which today only happens through the
interactive `/tailor/[slug]` workspace, one candidate click at a time.

The orchestration for "tailor, then let the candidate approve before it goes out" is being
built as a separate, external pipeline (an automation platform outside this repository,
calling freehire's own API). This change is freehire's own half: a narrow, purpose-built
surface that pipeline calls into, and the actual file-upload wiring in the browser driver.
It deliberately does NOT touch the existing interactive tailoring surface or its safety
posture — see design.md's "why not widen `/autopilot`" decision.

## What Changes

- Enqueuing a Greenhouse auto-apply attempt goes through the same deterministic
  `tailor-preflight-check` the interactive "Tailor CV" control already shows — no LLM call, no
  credit spent. A hard mismatch does not enqueue the attempt; the candidate is offered the
  existing interactive tailoring chat to add missing experience (banked with `stated_in_chat`
  provenance) before the automated run is queued at all. This is reuse of an existing
  capability, not a new check.
- `auto_apply_queue` gains the state to carry a tailoring outcome through to submission:
  which tailored CV is attached, and whether the candidate has reviewed it.
- A new, narrowly-scoped API-key-authorized endpoint starts an unattended tailoring run for
  ONE caller-owned queue entry (not any session) — bootstrapping the tailored CV and running
  its autopilot pass synchronously, returning the report once it finishes. It is not a
  variant of `/assistant/sessions/:id/autopilot`; that endpoint, its cookie-only posture, and
  its safety rationale are unchanged.
- A second endpoint records the candidate's review decision (approve/decline) for a queue
  entry's tailored CV. Declining parks the entry with its own reason; approving marks it
  ready for the ATS-submission step.
- The candidate is notified (existing notification channel) when a tailored CV is ready for
  their review, with a link into the existing tailoring workspace (`/tailor/[slug]?cv=<id>`)
  to actually look at what changed before deciding.
- `internal/api/atsapply`: `Claimed` carries the approved tailored CV; `Client.Submit`
  renders it to a PDF on demand (Typst, no object storage involved) and `fill.go` gains file-
  upload support for the résumé field via `chromedp.SetUploadFiles`. `resolve.go` recognizes
  the résumé file field instead of leaving it permanently unmapped.
- Every existing invariant of `internal/application/autoapply` (submits only when every
  required question is answered, `Park` is not a retry, an unconfirmed submission is never
  retried) is unchanged — this only adds a way to resolve one previously-always-parked field.

## Capabilities

### New Capabilities
- `auto-apply-cv-tailoring`: the queue-scoped tailoring trigger and its review gate — what
  starts a run for one queue entry, what the candidate sees and decides, and what a decision
  does to the entry's eligibility for submission.
- `atsapply-resume-upload`: resolving and filling a résumé file-upload field from an approved
  tailored CV during a live submission attempt.

### Modified Capabilities
(none — `auto_apply_queue`'s existing lifecycle, `internal/application/autoapply`'s runner,
and the interactive tailoring/autopilot surface are all unchanged in their existing
requirements; this only adds new ones.)

## Impact

- `migrations/`: new columns on `auto_apply_queue` (tailored CV reference, review timestamps).
- `internal/api/handler`: two new endpoints (start tailoring for a queue entry; record a
  review decision), reusing `cv.Store.Tailor` and `assistant.Runner` but through their own
  synchronous, non-streaming call shape rather than `PostAssistantAutopilot`'s SSE path.
- `internal/application/autoapply`: `Claimed` gains the approved CV reference; the runner's
  claim query only selects entries whose review is approved.
- `internal/api/atsapply`: `resolve.go`, `fill.go`, `client.go` gain file-upload handling.
- `internal/engage` (notifications): one new notification, "your tailored CV is ready to
  review."
- Nothing in the separate orchestration pipeline (outside this repository) is this change's
  concern — only the freehire-side surface it calls.
