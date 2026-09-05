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
- `cmd/auto-apply` — the one worker that already owns a headless-Chrome dependency — gains a
  second claim pass, run in the same invocation as its existing submit pass: it claims entries
  that have a tailored CV but no resolved preview yet, resolves the application form's answers
  the same way the real unattended submission eventually will (reusing `internal/api/atsapply`'s
  existing, already browser-independent `resolve.go` pipeline — deterministic only, no LLM
  drafting; for Greenhouse specifically this still needs one live, headless DOM render, since a
  schema-only guess measurably under-reports Greenhouse fields), and persists the result.
  **`cmd/auto-apply-orchestrate` is unchanged by this** — it has no direct database access and no
  browser by its own existing design (proposal.md of `auto-apply-inngest-orchestration`: it only
  ever calls `cmd/server`'s HTTP endpoints), so the preview cannot be computed inside an
  orchestrator step without giving that process a browser and a database connection it was
  deliberately built without. Computing it in the worker that already has both is reuse, not a
  new component. The orchestrator's own sequence (tailor, then wait for `review.decided`) does
  not need to know a preview exists at all — `pending_review` status simply is not reported
  until `resolved_preview` is set (see `auto-apply-tracker-review`'s own spec), so the wait
  still resolves the same way once the candidate reviews.
- The tailoring-complete notification (`auto-apply-tailored-resume` task 3.5) moves from firing
  synchronously inside `PostAutoApplyTailor` to firing from this new preview pass instead, once
  the preview is actually ready to show — a candidate notified the moment tailoring finishes but
  before there is anything to review would land on an empty screen.
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
- The existing tailoring-complete notification now links into `/my/tracking/[id]` (an
  existing deep-link route, built for the inbox's mail-linking feature, that already opens
  the given application's drawer on load), instead of `/tailor/[slug]`.
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
requirements are unchanged; `cmd/auto-apply` gains a second, independent claim pass and
`PostAutoApplyTailor` loses a notification call it already made, neither of which changes what
those capabilities themselves guarantee — the orchestrator's own sequence and the tailor/review
endpoints' own contracts are untouched.)

## Impact

- **Backend**: one migration (`auto_apply_queue.resolved_preview jsonb`); `cmd/auto-apply` gains a
  second claim pass (`internal/application/autoapply.RunPreviews`) reusing its existing
  `atsapply`/chromedp dependency; `internal/application/autoapply` gains the status derivation +
  preview assembly; the tracked-job read path (wherever `stage_suggestion` is assembled today)
  gains `auto_apply`; `auto-apply-submit-trigger`'s enqueue handler gains one `jobtracking.Track`
  call; `PostAutoApplyTailor` loses its notification call, moved to the new preview pass.
- **Frontend**: `BoardCard.svelte` gains a badge; `JobDrawer.svelte` gains a banner section;
  the notification's target moves to the existing `/my/tracking/[id]` deep link.
- **No impact** to `cmd/auto-apply`'s real submission path, `internal/api/atsapply`'s resolve
  logic (reused, not changed), or the job-detail auto-apply button's own 4-state contract.
