## Context

See `proposal.md` for motivation. The orchestration that decides WHEN to tailor and collects
the candidate's approval lives in a separate, external automation pipeline (outside this
repository) that calls freehire's own API with a per-user API key. This design covers only
freehire's side of that boundary: what it exposes, and what it does with an approval once
recorded.

Relevant existing pieces (see `internal/ai/assistant/AGENTS.md`, `internal/candidate/cv/AGENTS.md`,
`internal/application/autoapply/AGENTS.md`, `internal/api/atsapply/AGENTS.md`):
- `cv.Store.Tailor` bootstraps or reuses the tailored CV for (user, vacancy) — already usable
  outside HTTP, it takes a plain `context.Context`.
- `assistant.Runner.Run(ctx, sess, reg, system, brief, TurnConfig{MaxSteps}, emit)` is the turn
  loop the HTTP handler's SSE writer wraps — it is not itself coupled to `*fiber.Ctx`; only
  `PostAssistantAutopilot`'s surrounding handler code (the pre-run analysis fetch, the plan-limit
  gate via `h.meterTurn`, the SSE emit sink) is.
- `POST /assistant/sessions/:id/autopilot` is deliberately `mw.cookie`-only: "an unattended run
  rewrites a CV, and the browser is the only place the candidate can watch it happen and undo
  it." This change does not touch that route, its middleware, or its rationale.
- `auto_apply_queue` (migration 0116) already carries `claimed_at`/`failed_at`/`blocked_at`/
  `last_error`/`unmapped` — the same outbox shape every other queue in this codebase uses.
- `internal/api/atsapply`'s `resolve.go` never matches a `file`-kind field (documented gap);
  `cmd/auto-apply` does not require `S3_*` today because of it.

## Goals / Non-Goals

**Goals:**
- Let the external pipeline trigger tailoring and record a review decision for one queue entry,
  by API key, without widening any existing session-level surface.
- Keep the daily `tailor` allowance a single counter regardless of which surface spent it.
- Attach the approved CV to a real ATS file-upload field, rendered on demand.

- File-upload resolution stays Greenhouse-only, matching `internal/api/atsapply`'s existing
  scope (Lever always parks on captcha; every other provider merges from `applyform.Form`
  alone with no live DOM fill path at all — see that package's AGENTS.md). Widening the DOM
  fill path to another provider is that package's own known, separate gap, not this change's.

**Non-Goals:**
- The pipeline's own definition (its automations, its approval-step configuration, Claude
  Code's role in it) — that lives in the separate repository and is out of scope here.
- A review UI beyond linking into the existing `/tailor/[slug]?cv=<id>` workspace — the
  candidate reviews the SAME workspace an interactive tailoring session shows; no new page.
- Widening `/assistant/sessions/:id/autopilot` itself, or changing its cookie-only posture.
- Cover-letter or other non-résumé file fields — still parks, unchanged.

## Decisions

**A new, queue-scoped endpoint — not a variant of `/autopilot` — is the tailoring trigger.**
Reusing `/autopilot` (even behind a new API-key-accepting flag) would mean the general
"run autopilot on any session I own" capability becomes reachable by API key, which is exactly
what its own comment says must not happen: an unattended CV rewrite normally needs the browser
watching it live. The auto-apply flow does not have that live browser — it substitutes a
review checkpoint AFTER the run instead of live observation DURING it. That is a different
safety argument (a decision the candidate makes once, having seen the diff, rather than a
window they watch while it happens), and a NEW endpoint that is scoped to exactly one
caller-owned queue entry — never any session id the caller supplies — is what keeps that
argument sound: nothing about it can be pointed at an unrelated tailoring session, and nothing
about `/autopilot` changes for every other caller of it. `POST /me/auto-apply/:queueId/tailor`
(API key via `mw.key`) does not take a session id at all; it resolves the session itself from
the queue entry's own vacancy, mirroring `cv.Store.Tailor`'s own bootstrap-or-reuse call.

**Synchronous response, not SSE.** The existing autopilot endpoint streams because a human is
watching a live chat. An automation pipeline's HTTP step wants a plain request/response: call,
wait, get a result. The new endpoint calls `assistant.Runner.Run` with a no-op emit sink
(events are dropped, not persisted twice — the transcript is still written by `Runner.Run`
itself the same way it always is) and responds once `Run` returns, carrying the tailored CV id
and its `autopilot_report`. A run is bounded (`autopilotMaxSteps = 30`, each step bounded by the
LLM client's own per-call timeout), so a long-lived POST is an acceptable trade against the
complexity of teaching an external pipeline to consume SSE for one call.

**The plan-limit gate is called directly, not through `h.meterTurn`.** `meterTurn` writes a 402
onto a `*fiber.Ctx` before a stream opens; the new endpoint calls `internal/ai/plan`'s decide
function for the `tailor` feature directly and maps a refusal to its own 402 JSON body, before
`cv.Store.Tailor` ever runs — so a spent allowance costs nothing, exactly like every other
tailoring entry point.

**Review state rides on `auto_apply_queue` itself, as new columns, not a second table.**
`tailored_cv_id` (nullable, set once tailoring finishes) and `reviewed_at`/`review_decision`
(nullable, set once by the review endpoint) are enough: one row per (user, vacancy) already
exists, and a decision is recorded exactly once per row (an existing enforced-by-check-before-
write rule, mirroring how `Park`/`Fail`/`Submit` already each assume a row is only ever acted on
once per outcome). `internal/application/autoapply`'s `Store.Claim` query gains one more
predicate: `tailored_cv_id IS NOT NULL AND review_decision = 'approved'`. A DECLINED entry sets
`blocked_at` through the SAME `Park` the runner already calls for an unresolved form field, with
a distinct `last_error` text ("candidate declined the tailored CV") — one park vocabulary, one
claimable-index shape, and legible apart from a form-field park purely by that text (matching
how `Fail`'s `last_error` already disambiguates different transient causes with text, not a
second column).

**File rendering is on-demand, no object storage.** `Client.Submit` (atsapply) reads the
approved CV's document via the existing CV store, renders it through the existing Typst
renderer (`internal/candidate/cv`) straight to a temp file (`os.CreateTemp`, removed after the
chromedp session closes), and calls `chromedp.SetUploadFiles(selector, []string{path})`. No new
dependency: `internal/candidate/cv`'s renderer and `chromedp` are both already imported by
their respective packages.

**A run that leaves "open" requirements is addressed BETWEEN runs, never mid-run.** The
autopilot invariant this repo already documents ("asks nothing until the run is finished") is
unchanged and unnegotiable here — widening it would be a rewrite of the runner itself, out of
scope. The candidate reviewing an "open" item in the report goes to the SAME interactive
tailoring chat the preflight gate above sends them to, adds what is missing, and presses the
workspace's existing "Run again" (`AutopilotReport.onRerun`) — a second, ordinary run, still
asking nothing mid-run. This is why the review endpoint links into the real workspace rather
than a read-only summary: "fix it and rerun" has to be a real, already-built action away.

**Alternatives considered:**
- *Flip `/autopilot`'s route to `mw.key` for this case.* Rejected outright per the user's own
  instruction and the reasoning above — this is a new way in, not a change to the existing one.
- *A second table for review state.* Rejected: one row per (user, vacancy) already exists on
  `auto_apply_queue`, and the review fields are exactly as many as the queue's own `Park`/`Fail`
  fields — a second table would only be justified by a retry/lease lifecycle review does not
  have (a decision is made once, not reclaimed).
- *Store the rendered PDF once, in S3, instead of rendering on demand at submit time.*
  Rejected for now: it would reintroduce the `S3_*` requirement `cmd/auto-apply` currently
  avoids, for a render that is fast (the interactive workspace already renders on every PDF
  download) and only needed once per submission attempt.

## Risks / Trade-offs

- **A declined queue entry has no automatic path back to "try again."** Same trade-off noted in
  the earlier design discussion for this feature: `Park` here means terminal until a human
  notices, exactly like an unresolved form field today. Not addressed by this change.
- **A synchronous, long-lived POST for tailoring** ties up an HTTP connection (and a goroutine)
  for the run's full duration. Acceptable for a low-volume, pipeline-driven call pattern; would
  need reconsidering if this endpoint's call volume ever approached the interactive workspace's.
- **The external pipeline is a hard dependency this repository cannot test against.** Every
  scenario above is testable from freehire's own side with a fake caller (a test hitting the new
  endpoints directly) — nothing here assumes anything about the pipeline's internals, only about
  the HTTP contract it is handed.

## Open Questions

None — every decision above was resolved in conversation before writing this file, including
the explicit instruction not to touch `/autopilot`'s own posture.
