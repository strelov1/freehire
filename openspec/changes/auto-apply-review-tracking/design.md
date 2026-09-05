## Context

`auto_apply_queue` (migrations/0116, /0128) already carries `tailored_cv_id`, `review_decision`
(`approved`/`declined`/NULL), `blocked_at` (+ `unmapped` jsonb), `failed_at`, `last_error`. On a
successful submission the row is deleted in the same transaction that calls
`jobtracking.MarkJobApplied` (`cmd/auto-apply/store.go`) — a completed attempt already reaches
`applied` on the tracker with no change needed here.

The candidate-facing gap is entirely on the *pending* side: nothing surfaces a `pending_review`
entry anywhere, nothing shows what a `blocked` entry is missing, and triggering auto-apply
(`auto-apply-submit-trigger`) does not put the job on the board at all — the tracker has no idea
an attempt exists until it silently succeeds or fails.

The existing review write path (`internal/api/handler/auto_apply_tailor.go`,
`POST /me/auto-apply/:queueId/review`) and its Inngest resume plumbing
(`auto_apply_review_publish.go`) are unchanged by this design — they already implement
approve/decline and resuming the orchestrator (`auto-apply-inngest-orchestration`). This design
only adds: (1) a way for the tracker to see a `pending_review`/`blocked` entry and call that
existing endpoint, and (2) the one missing piece neither of those changes built — an exact
preview of what the unattended submission would send.

## Goals / Non-Goals

**Goals:**
- Surface auto-apply's live status inside the tracker the candidate already uses, via the same
  "action needed" interaction shape the tracker already has (mail-derived stage suggestions).
- Give the candidate an exact preview — not a guess — of the application-form answers before they
  approve, computed once and persisted, not on the read path.
- Make triggering auto-apply immediately visible on the board.

**Non-Goals:**
- No retry/unpark mechanism for a `blocked` or `failed` entry — none exists in the backend today
  (no `Unpark`/requeue query or handler), and `auto_apply_queue`'s own `UNIQUE (user_id, job_id)`
  plus `PostJobAutoApply`'s permanent-decline check mean there is no recovery path regardless of
  what the UI shows. A separate, larger change.
- No cover-letter generation — the preview reflects whatever the form's `resolve.go` already
  produces today, which never includes one.
- No dedicated auto-apply page, no separate "recently completed" list — both already exist as the
  tracker's own `applied` stage.
- No pagination anywhere in this change (a candidate's own live queue is small).
- No change to the job-detail button's existing 4-state contract (`autoApplyButtonState`,
  `autoApplyEntryStatus`) — it keeps its own, coarser status derivation; it is not wired to the
  tracker and stays that way.

## Decisions

**The preview is computed once, server-side, inside the existing orchestrator run — not lazily
on read.** `internal/api/atsapply`'s resolve pipeline is already a pure function over
`[]MergedField` once a schema is in hand (`resolve.go`, `draft.go`, no chromedp import anywhere in
either file); the browser is only needed to produce the DOM-scanned field list for Greenhouse
(`client.go`'s `renderedHTML` → `ScanGreenhouseForm`, itself "a pure function over an HTML
string" per its own doc comment). For the other three providers, `mergedFromAPIOnly` already
builds the schema without a browser, and can read the schema `cmd/capture-apply-form` already
persisted in `apply_forms` instead of re-fetching it. A new orchestrator step (`step.Run`, after
the existing tailor step, before the entry moves to `pending_review`) calls this same pipeline —
including, for Greenhouse, one headless render, exactly like the real submission eventually does
— and persists the result to `auto_apply_queue.resolved_preview jsonb`.
Alternative considered: resolve lazily when the candidate opens the drawer — rejected because it
would put a Chrome dependency on the interactive API request path for Greenhouse specifically
(today only workers own a browser lifecycle), adding request latency and a new failure mode to a
read.
Alternative considered: always approximate from the cached/API-only schema, skipping the
Greenhouse DOM render — rejected because a spike measured 36 DOM-scanned fields against 17
API-declared ones for the same form; an approximation could omit a field the real submission
requires, which defeats the point of showing the candidate what will actually be sent.

**Auto-apply gets its own `Track` source, called from the enqueue handler.**
`jobtracking.Service.Track(ctx, userID, slug, stage, notes, source)` already accepts a `source`
string (the same vocabulary `application_events.source` uses elsewhere in this codebase). The
existing `auto-apply-submit-trigger` enqueue handler calls it with `stage: "preparing"`,
`source: "auto_apply"`, idempotently (an already-tracked job is left alone — `Track`'s own
existing semantics, unchanged here) — so the job appears on the board the moment auto-apply
starts, not once it happens to finish.

**Status and preview ride on the tracked-job read path as one optional field, not a separate
endpoint.** The tracker's existing wire shape already carries an analogous optional field,
`stage_suggestion` (from `jobtracking/suggestion.go`'s `StageSuggestion`), assembled alongside the
rest of a `TrackedJob` when one exists. `auto_apply` is added the same way: a new
`internal/application/autoapply` function derives the six-value status
(`tailoring`/`pending_review`/`approved`/`blocked`/`declined`/`failed`) from
`(tailoredCVID, reviewDecision, blockedAt, failedAt)` — mirroring `autoAppyEntryStatus`'s existing
precedence (declined checked before blocked/failed) — and assembles
`{status, resolved_preview, tailored_cv_id, unmapped}` (never `last_error`) for whatever entry
matches the job. Alternative considered: a separate `GET /me/auto-apply` list endpoint (the
original draft of this change) — rejected once the surface moved into the tracker: the tracker
already has to fetch the board to render at all, and a second round trip the frontend would need
to cross-reference by job id adds a request for no reader benefit the embedded field doesn't
already give for free.

**The tracker UI reuses the stage-suggestion banner's shape, not its component.**
`JobDrawer.svelte`'s existing suggestion banner (warning-tinted, one action button, one dismiss
link) is the right *shape* — action needed, one clear next step, dismissible — but auto-apply's
banner needs richer content (the answer preview, a CV link, two buttons instead of one) and two
outcomes (approve/decline) instead of one (move-stage/dismiss), so it is a new banner section
built to match that visual language rather than a prop-driven variant of the same component. The
non-actionable states (`blocked`/`declined`/`failed`) render a third, read-only variant of that
same banner family.

**Notification target changes from `/tailor/[slug]` to `/my/tracking?job=<id>`.**
The tailoring-complete notification (`auto-apply-tailored-resume` task 3.5) currently links to the
tailoring workspace, which has no approve/decline affordance and was never meant to be one — it
is where a CV gets *edited*, not where an auto-apply decision gets made. Once the resolve-preview
step lands (this change), the notification is meaningful only once approve/decline is possible,
so its target moves to the tracker, deep-linked to open the relevant job's drawer directly.

**`unmapped` is returned verbatim on a `blocked` entry; `last_error` is never serialized.**
`unmapped` ([{id, label, required, reason}], migrations/0116) was designed to be legible without
replaying the attempt — exactly the "what got blocked" detail the candidate needs. `last_error` is
an internal diagnostic string never intended for a candidate to read; the new assembly function's
return struct has no field for it.

## Risks / Trade-offs

- **[Risk]** A candidate reads `blocked`/`failed`/`declined` and expects a way to fix it, but no
  retry path exists → **Mitigation**: the banner copy for these states is explicit that the
  attempt is final for this job (matches `PostJobAutoApply`'s own existing
  "permanently stuck"/"already declined" wording).
- **[Risk]** The Greenhouse resolve-preview step adds one more headless-browser render to the
  orchestrator run, between the tailor step and the review wait → **Mitigation**: this reuses the
  exact render `cmd/auto-apply`'s real submission already performs (same cost, same failure mode,
  already handled there); a render failure here should park the entry the same way a resolve
  failure during the real submission already does, rather than leaving it silently stuck.
- **[Risk]** `EventSource`/status vocabulary drift: if a future change adds a way to un-park an
  entry, the "final, no retry" banner copy this design writes becomes stale →
  **Mitigation**: none needed structurally now; worth a comment at the banner site
  cross-referencing this design.

## Migration Plan

One migration: `auto_apply_queue.resolved_preview jsonb` (nullable). Additive-only otherwise: new
orchestrator step, new use-case function, one new field on an existing read path, new frontend
banner/badge, notification target change. Rollback is a plain revert — nothing else depends on
`resolved_preview` or the new field.
