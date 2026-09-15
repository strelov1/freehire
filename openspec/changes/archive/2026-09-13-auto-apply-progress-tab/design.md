## Context

`JobDrawer.svelte` already loads everything this change needs, eagerly, on mount
(`loadEmails()`, called from an `$effect` for any signed-in user): `autoApply` (the
`AutoApplyReviewInfo | null` from `GET /me/tracking/:slug`) and `events` (that same response's
`TimelineEvent[]`, the application-event ledger). The status banner today lives inline in the
`{#if tab === 'application'}` block (`autoApplyBanner = $derived(autoApplyReviewBanner(autoApply?.status))`,
rendered lines ~514-660), sharing that block's wrapping `<div>` with unrelated content (the
stage select, notes editor, the ledger "History" list, the pending-outcome picker) that must stay
in the `application` tab. See proposal.md for why this needs its own tab instead.

`TimelineEvent` already carries `kind`/`source` on the wire (`internal/application/apptimeline`'s
`Event.Source`, populated with `appevent.SourceAutoApply` by `cmd/auto-apply`'s `Store.Submit` when
it calls `MarkJobApplied`) — so "this application was submitted by auto-apply" is answerable from
data already in hand, with no backend change.

## Goals / Non-Goals

**Goals:**
- Give the auto-apply status/decision UI its own tab, driven by a small, independently testable
  progress-stage mapping.
- Show the submitted outcome even after the live attempt's queue row is gone.
- Let the candidate mark a live attempt "paused" as a personal note, with zero coupling to
  `cmd/auto-apply` or the queue schema.

**Non-Goals:**
- Any change to `cmd/auto-apply`, `auto_apply_queue`, or a migration — confirmed with the user
  that pause is purely cosmetic.
- Syncing the pause marker across devices/browsers — confirmed acceptable as browser-local.
- Touching the separate, already-existing `auto-apply-drawer-progress` change directory (deep
  links, board filter toggle, card dot) — different, unrelated scope.
- A dynamic tab label (e.g. a badge count or "needs attention" marker on the tab itself) — the
  sibling change above already owns board/card-level attention markers; adding a second, tab-local
  one here would be a second place answering the same question.

## Decisions

- **Progress-stage mapping lives in a new pure module, `web/src/lib/autoApplyProgress.ts`**,
  mirroring `autoApplyReview.ts`'s existing convention (pure function, unit-tested without
  mounting Svelte). `autoApplyProgressSteps(status: AutoApplyStatus | null | undefined, autoApplied: boolean)`
  returns a fixed 3-tuple of `{ id: 'tailoring' | 'review' | 'submitted', state: 'done' | 'active' | 'queued' | 'error' | 'pending' }`,
  plus which step (if any) is the error step. One function, not three, because the three stages
  are never evaluated independently — a later stage's state always depends on how an earlier one
  resolved.
  - `tailoring` → step1 `active`, step2/3 `pending`
  - `tailor_failed` → step1 `error`, step2/3 `pending`
  - `pending_review` → step1 `done`, step2 `active`, step3 `pending`
  - `declined` → step1 `done`, step2 `error`, step3 `pending`
  - `approved` → step1/2 `done`, step3 `queued`
  - `blocked` / `failed` → step1/2 `done`, step3 `error`
  - no live status but `autoApplied` → all three `done`
  - no live status and not `autoApplied` → the caller does not render the tab at all (see below)
- **`autoApplied` is computed in `JobDrawer.svelte`** as
  `events.some((e) => e.kind === 'applied' && e.source === 'auto_apply')` — a one-line derived
  value, not worth its own module, using data the component already holds.
- **The `Progress` tab's visibility condition is `autoApply != null || autoApplied`** (a new
  `$derived`), following the exact pattern `canSeeMail`/the `emails` tab already use for
  conditional tabs.
- **The existing banner block moves verbatim** from the `application` tab into the new tab's
  branch — same markup, same handlers (`decideAutoApply`, `saveBankedAnswer`, the answer-bank
  state), just relocated. No behavior change to approve/decline/answer-saving.
- **Pause storage is a new `web/src/lib/autoApplyPauseStorage.ts`**, mirroring
  `filterStorage.ts`'s shape exactly: `typeof localStorage === 'undefined'` feature-detection,
  every access wrapped and failures swallowed, one existence-keyed entry per queue id
  (`hire.autoApplyPaused:<queueId>`), because presence/absence is all this needs — there is no
  value to store beyond the boolean itself. `isAutoApplyPaused(queueId)` /
  `setAutoApplyPaused(queueId, paused)`.
- **The pause control is gated on the *stage*, not a separate flag check in the storage
  module**: `JobDrawer.svelte` only renders it when `autoApply` is non-null and its status is one
  of `tailoring`/`pending_review`/`approved`/`blocked` — i.e. never for `declined`/`failed`/
  `tailor_failed`/already-submitted. The storage module itself has no opinion on when it is legal
  to call it; that rule belongs with the UI that decides what to show, the same division
  `autoApplyReviewBanner` already draws between "what does this status mean" and "what can you do
  about it".

## Risks / Trade-offs

- **[Risk]** Moving the banner out of the `application` tab is a real, deliberate UX change: a
  candidate who previously saw the pending-review decision immediately on opening the drawer now
  has to click into `Progress` first. → **Mitigation**: this is what the user explicitly asked
  for (a dedicated tab); out of scope for this change to also add a tab-level attention indicator
  (see Non-Goals) — if that gap turns out to matter, it is a small follow-up once the sibling
  `auto-apply-drawer-progress` change's board-level attention work has landed, so the two do not
  build the same signal twice.
- **[Risk]** The pause marker is keyed by `queue_id`, which does not survive a submitted/retired
  attempt (the row, and therefore the reason to show a pause control, is gone by then) — so
  nothing needs to clean up a stale key; an orphaned `hire.autoApplyPaused:<id>` entry for a
  long-gone queue row is inert, never read again. Not worth adding cleanup for.
