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

**The preview is computed once, off the request path — not lazily on read — by `cmd/auto-apply`
itself, in a second claim pass alongside its existing submit pass.** `internal/api/atsapply`'s
deterministic resolve pipeline (`resolve.go`'s `Resolve`, never `draft.go`'s `ResolveWithDrafting`
— see the next decision for why drafting is excluded) is a pure function over `[]MergedField`
once a schema is in hand; the browser is only needed to produce the DOM-scanned field list for
Greenhouse (`client.go`'s `renderedHTML` → `ScanGreenhouseForm`). For the other three providers,
`mergedFromAPIOnly` already builds the schema without a browser, and can read the schema
`cmd/capture-apply-form` already persisted in `apply_forms` instead of re-fetching it.

This runs in `cmd/auto-apply`, not `cmd/auto-apply-orchestrate`, because the orchestrator has
neither capability by its own existing design (`auto-apply-inngest-orchestration`'s own
proposal.md): it holds no `DATABASE_URL` and calls `cmd/server`'s HTTP endpoints exactly the way
an external caller would, one `step.Run` per call. Giving the orchestrator a database connection
and a Chrome dependency just for this preview would duplicate infrastructure `cmd/auto-apply`
already has and already manages (lease, retry, park) for the structurally identical problem of
"resolve this attempt's form against a browser, then record the outcome." A new exported
function, `autoapply.RunPreviews` (mirroring `autoapply.Run`'s own `outbox.RunPool`-based shape),
claims entries with a tailored CV and no `resolved_preview` yet, calls a new
`atsapply.PreviewClient.Preview`, and persists the result — called from `cmd/auto-apply/main.go`
alongside the existing `autoapply.Run` call, in the same run, same process, same Chrome
dependency, no new binary and no new deploy unit.

The orchestrator's own `step.WaitForEvent` wait for `auto-apply/review.decided` needs no change:
it already waits for a decision the candidate can only make once the tracker shows them
something to decide on, and `pending_review` status (assembled in Go, §"Status and preview ride
on the tracked-job read path") is simply never reported until `resolved_preview` is set —
whichever one of tailoring or preview-resolution finishes last is a fact the status derivation
already handles, not something either side needs to coordinate on.

Alternative considered: an orchestrator step, as originally drafted — rejected once
`auto-apply-inngest-orchestration`'s own proposal.md was reread carefully: the orchestrator's
whole point is staying database- and browser-free, calling back into `cmd/server` over HTTP the
same way any future external pipeline would (per that change's own "What Changes" section); a
step that reached into Postgres or launched Chrome directly would contradict the reason that
process exists.
Alternative considered: a new `cmd/server` HTTP endpoint the orchestrator calls, mirroring the
tailor endpoint — rejected because Greenhouse's preview still needs a live DOM render, which
would put a Chrome dependency on the API server process (today: zero HTTP handlers import
`internal/api/atsapply`; only `cmd/auto-apply` does) for every provider, not just the one worker
already provisioned for it.
Alternative considered: resolve lazily when the candidate opens the drawer — rejected for the
same two reasons: a Chrome dependency on the interactive request path, and (per the next
decision) LLM spend before the candidate has approved anything.
Alternative considered: always approximate from the cached/API-only schema, skipping the
Greenhouse DOM render — rejected because a spike measured 36 DOM-scanned fields against 17
API-declared ones for the same form; an approximation could omit a field the real submission
requires, which defeats the point of showing the candidate what will actually be sent.

**The preview never runs LLM drafting (`ResolveWithDrafting`), even though the real submission
does.** `internal/ai/llmkey/scope_test.go` names exactly two binaries allowed to resolve a
per-user LLM credential (`cmd/server`, `cmd/auto-apply`) and requires a deliberate, justified
third line for any other. More fundamentally: drafting spends the candidate's own LLM credit,
and a preview exists BEFORE the candidate has approved anything — drafting at preview time would
spend on an attempt they might decline. The preview instead reports, for each still-unresolved
field, whether the real submission will draft one (`draftable()`, the same pure eligibility
check `ResolveWithDrafting` itself applies before ever calling the drafter — no LLM call, just
the same required/labeled/kind/non-sensitive predicate) — so the candidate sees "this will be
filled in automatically" rather than a blank the drafter would in fact have answered.

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

**Notification target changes from `/tailor/[slug]` to `/my/tracking/[id]`.**
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
- **[Risk]** The Greenhouse preview pass adds one more headless-browser render per attempt to
  `cmd/auto-apply`'s own run, before the entry is even approved → **Mitigation**: this reuses the
  exact render `Submit` already performs for the real attempt (same cost, same failure mode,
  already handled there); a render failure parks the entry via the same `Store.Park` path a
  resolve failure during the real submission already uses, rather than leaving it silently stuck
  short of `pending_review` forever.
- **[Risk]** A window exists between "tailored" and "preview resolved" where the candidate has
  no action to take yet → **Mitigation**: status derivation reports `tailoring` for that window
  (not a seventh status), and `cmd/auto-apply`'s preview pass runs in the same cron cadence the
  submit pass already does, so the window is bounded by that cadence, not by anything new.
- **[Risk]** `EventSource`/status vocabulary drift: if a future change adds a way to un-park an
  entry, the "final, no retry" banner copy this design writes becomes stale →
  **Mitigation**: none needed structurally now; worth a comment at the banner site
  cross-referencing this design.

## Post-implementation review fixes

A full-diff code review (before archiving this change) caught two real bugs and two
lower-severity gaps in the implementation above, all fixed in the same branch:

- **The preview pass and the submit pass originally shared `attempts`/`failed_at`
  (`RecordAutoApplyFailure`).** A transient preview-resolution error (a flaky schema fetch, a
  chromedp launch failure) spent down the SAME retry budget the real ATS submission depends
  on, and could dead-letter a row (`failed_at` set, reported to the candidate as "could not
  submit after retrying") before a submission was ever attempted. Fixed with migration
  0140: `preview_attempts`/`preview_failed_at`, a new `RecordAutoApplyPreviewFailure` query,
  and `PreviewStore.FailPreview` (distinct from `Store.Fail` — the two interfaces can no
  longer collapse onto one shared implementation by accident). `ClaimAutoApplyPreviewBatch`
  now excludes on `preview_failed_at`, not the submit pass's own `failed_at`. `DeriveStatus`'s
  `failed` input is now `failed_at.Valid || preview_failed_at.Valid` at both read paths, so a
  permanently-stuck preview still surfaces as `failed` to the candidate instead of reading as
  `tailoring` forever.
- **`SetAutoApplyResolvedPreview` never released the `claimed_at` lease it took as a preview
  claim.** An approval landing before that lease's own `AUTO_APPLY_LEASE_SECONDS` window
  expired would sit unclaimed by `ClaimAutoApplyBatch` for up to that long, for no reason —
  nothing was still working the row. Fixed: the same statement that persists the preview now
  also sets `claimed_at = NULL`.
- **A deliberate re-tailor didn't invalidate a stale preview.** `SetAutoApplyTailoredCV` only
  wrote `tailored_cv_id`; an entry already at `pending_review` whose CV got re-tailored kept
  showing the candidate an answer preview computed against the PREVIOUS CV. Fixed: the same
  statement now also clears `resolved_preview` and resets `preview_attempts`/
  `preview_failed_at` — a fresh CV gets a fresh preview attempt budget, not a permanently
  exhausted one.
- **`JobDrawer.svelte`'s optimistic update mutated the `item` prop directly**, working only by
  relying on `JobBoard.svelte` passing a live `$state`-proxied object reference — an
  implementation detail two components away that every other mutation in the same file
  respects via a callback prop instead. Fixed: `onautoapplyreview` mirrors `onsetstage`'s own
  division of labor (the drawer makes the API call, since it alone holds the queue id; the
  parent owns the mutation, since it alone owns `item`'s reactivity).

## Migration Plan

One migration: `auto_apply_queue.resolved_preview jsonb` (nullable). Additive-only otherwise: a
new claim pass in an existing worker, new use-case functions, one new field on an existing read
path, new frontend banner/badge, a relocated notification call. Rollback is a plain revert —
nothing else depends on `resolved_preview` or the new field.
