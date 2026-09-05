## Why

Auto-apply already has a working backend for tailoring, candidate review (approve/decline),
and unattended submission (`auto-apply-worker`, `auto-apply-tailored-resume`,
`auto-apply-submit-trigger`, `auto-apply-inngest-orchestration`), but no surface in the SPA
ever calls the review endpoint or shows the candidate where an attempt stands. A candidate who
starts an auto-apply attempt gets no notification they can act on, no way to approve or decline
the tailored CV, no way to see what would actually be submitted on their behalf, and no way to
see why an attempt got stuck (a required question the profile could not answer). A real attempt
run this session got stuck at the review step with nowhere in the product to resolve it.

## What Changes

- Auto-apply's review surface lives **in the existing tracker board**, not a new page. The
  tracker already has the exact interaction shape this needs: `JobDrawer.svelte` already
  renders an "action needed" banner for mail-derived stage suggestions
  (`stageSuggestion`/`StageSuggestion`), with an action button and a dismiss link, opened from a
  `BoardCard.svelte` that deliberately carries no controls of its own. Auto-apply review reuses
  that exact pattern instead of inventing a second one.
- Triggering auto-apply (the existing job-detail button, `auto-apply-submit-trigger`) now also
  ensures the job is tracked (`jobtracking.Service.Track`, stage `preparing`, a new
  `source="auto_apply"`) if it is not already — today an auto-apply attempt is entirely
  invisible to the board until it silently succeeds.
- The tailor-and-review orchestrator (`cmd/auto-apply-orchestrate`) gains one new step, run right
  after tailoring and before the entry is marked `pending_review`: it resolves the application
  form's answers the same way the real unattended submission eventually will (reusing
  `internal/api/atsapply`'s existing, already browser-independent `resolve.go`/`draft.go`
  pipeline; for Greenhouse specifically this needs one live, headless DOM render — the same
  scan the real submission does — since a schema-only guess measurably under-reports Greenhouse
  fields), and persists the result. The candidate reviews an exact snapshot of what would be
  submitted, computed once, off the request path — not a live guess made when they open the
  drawer.
- `GET` responses for a tracked job gain an optional `auto_apply` object (status, the persisted
  answer preview, the tailored CV reference, and — only when blocked — the unmapped question
  list; never the internal `last_error`), mirroring the existing `stage_suggestion` field
  exactly.
- `BoardCard.svelte` gets a small "needs your review" badge (same visual family as the existing
  silence badge) when an entry is `pending_review` or `blocked`, so the candidate does not have
  to open every card to notice one needs them.
- `JobDrawer.svelte` gets a new banner: for `pending_review`, the answer preview + tailored CV
  link + Approve/Decline (calling the existing, unchanged `POST /me/auto-apply/:queueId/review`);
  for `blocked`/`declined`/`failed`, a read-only variant naming why, explicit that the attempt is
  final for this job (no retry path exists anywhere in the backend today).
- The existing tailoring-complete notification now links into `/my/tracking?job=<id>`, which
  opens that job's drawer on load, instead of `/tailor/[slug]`.
- No new database columns for status derivation (unchanged from the queue's existing
  `tailored_cv_id`/`review_decision`/`blocked_at`/`failed_at`); one new column,
  `auto_apply_queue.resolved_preview jsonb`, holds the persisted answer-preview snapshot.

Explicitly **not** part of this change:
- **No dedicated `/my/auto-apply` page and no "recently completed by auto-apply" list.**
  Recently-completed attempts are already visible in the tracker at the `applied` stage
  (`cmd/auto-apply`'s existing success path already calls `jobtracking.MarkJobApplied`); a
  second, separate view of the same fact was not requested and would only duplicate it.
- **No cover-letter generation.** Auto-apply does not generate or attach a cover letter today —
  a `cover_letter` form field always resolves as unmapped
  (`resolve_test.go:TestResolve_ACoverLetterFieldStaysUnmappedEvenWithAnApprovedCV`). The answer
  preview surfaces whatever a job's form actually requires; it does not invent a letter to fill
  that gap.
- **No retry/unpark mechanism** for a `blocked` or `failed` entry — none exists in the backend
  today and adding one is a separate, larger change.

## Capabilities

### New Capabilities
- `auto-apply-tracker-review`: the tracker-embedded read/act surface for an auto-apply attempt
  — the six-value status derivation, the persisted answer-preview snapshot, and the
  approve/decline interaction, all surfaced through the existing tracked-job shape rather than a
  standalone endpoint or page.

### Modified Capabilities
(none — `auto-apply-cv-tailoring`, `atsapply-resume-upload`, and `auto-apply-orchestration`'s own
requirements are unchanged; this only adds one more step inside the orchestrator's existing run
and a caller for the existing review endpoint, neither of which changes what those capabilities
themselves guarantee.)

## Impact

- **Backend**: one migration (`auto_apply_queue.resolved_preview jsonb`); the orchestrator gains
  a resolve-preview step; `internal/application/autoapply` gains the status derivation + preview
  assembly; the tracked-job read path (wherever `stage_suggestion` is assembled today) gains
  `auto_apply`; `auto-apply-submit-trigger`'s enqueue handler gains one `jobtracking.Track` call.
- **Frontend**: `BoardCard.svelte` gains a badge; `JobDrawer.svelte` gains a banner section;
  the tracking route supports opening a drawer via `?job=<id>`; the tailoring-complete
  notification's target changes.
- **No impact** to `cmd/auto-apply`'s real submission path, `internal/api/atsapply`'s resolve
  logic (reused, not changed), or the job-detail auto-apply button's own 4-state contract.
