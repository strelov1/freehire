## Context

`web/src/lib/autoApplyReview.ts`'s `autoApplyReviewBanner` already maps the six-value
`autoapply.Status` (`internal/application/autoapply/status.go`) to a drawer banner variant, but
returns `null` for `tailoring` and `approved` — see proposal.md for why that gap matters. The
`pending_review` variant in `JobDrawer.svelte` already renders a "View tailored CV" link to
`/tailor/[slug]`; the new `approved` variant reuses that same link.

On the notification side, `internal/engage/nudge/transports.go`'s `renderOne` (Telegram) and
`EmailNotifier.render` (email) build a `trackingURL` from `n.origin + "/my/tracking"` for every
kind except `KindJobClosed`. `web/src/lib/notificationTarget.ts` mirrors this on the frontend:
`nudge_auto_apply_submitted/blocked/failed` resolve to `{kind: 'tracking'}` with no slug, by a
documented, deliberate choice to match the (undifferentiated) Telegram/email link. This change
reverses that choice for these three kinds only, now that the drawer has something worth jumping
to for every live auto-apply status. See proposal.md for what stays out of scope
(follow-up/interview-prep, push).

`web/src/lib/components/BoardCard.svelte` already derives `needsAutoApplyReview` from
`autoApplyNeedsReviewBadge(item.auto_apply_status)` and renders a text-only "Review" badge when
truthy. `JobBoard.svelte` already filters both its board and list views through one `shown`
derived value, built from `matchesQuery` (`web/src/lib/board.ts`) over the text in
`UrlSyncedState`-backed `search`, mirrored to `?q=` — the same pattern a second, boolean toggle
can reuse.

## Goals / Non-Goals

**Goals:**
- Give `tailoring` and `approved` their own read-only banner in the tracker drawer.
- Make the three outcome notifications' single-application case deep-link to that application.
- Let the candidate narrow the board to only applications needing an auto-apply decision, and
  make that state more visible on the card itself.

**Non-Goals:**
- Changing `follow_up`/`interview_prep` link targets, or push notification click behavior.
- Adding a retry/cancel action to any terminal status — `blocked`/`declined`/`failed` remain
  read-only, unchanged by this design.
- Touching the pre-existing, unarchived `add-auto-apply-outcome-notifications` change directory —
  its tasks are already complete and its code is already live; this design only extends what it
  shipped.

## Decisions

- **`tailoring` gets a plain status line, no link.** There is no tailored CV yet to open
  (`DerivedStatus` only reaches `tailoring` when `hasTailoredCV` is false), so any link would
  either 404 or point at a workspace that has nothing to show yet. A future change can revisit
  this once there is something to view mid-tailoring.
- **`approved` reuses the `pending_review` banner's tailored-CV link verbatim**, rather than
  building a separate read-only preview view. The candidate already approved this exact tailored
  CV; showing the same artifact through the same link keeps the two states visually and
  behaviorally consistent, and avoids a second rendering path for one preview.
- **The notification-link fix touches only the single-application render path**
  (`renderOne`/`EmailNotifier.render`'s per-message branch, and `notificationTarget.ts`'s
  slug-bearing branch), not `batchDestination`. A batch has no single job to deep-link to, and
  `batchDestination`'s existing fallback to the general board is correct for that case — it needs
  no change.
- **The "needs attention" predicate is a new pure function in `board.ts`** (`needsAttention(item)`,
  wrapping the same `autoApplyNeedsReviewBadge` call `BoardCard.svelte` already makes), not a
  second copy of the pending_review/blocked check. The board's `shown` derived ANDs it with the
  existing `matchesQuery` result when the toggle is on, mirroring how the two views already share
  one filtered value.
- **The toggle is mirrored into the URL** (`UrlSyncedState<boolean>`, its own query param) the
  same way `search` already is, so a filtered board survives a reload or a shared link — consistent
  with the existing search field rather than a one-off local `$state`.
- **The board card gets a dot, not a badge replacement.** The existing "Review" text stays exactly
  as it renders today (same copy, same `aria-label`); the dot is a purely decorative sibling
  element (`aria-hidden`), so no accessible-name behavior changes — only the visual weight of an
  already-present signal.
- **Push is left untouched.** `push.go` already carries `data: {"slug": ...}` for a single-message
  batch, but no `notificationclick` handler exists anywhere in this repo to consume it — whatever
  reads that field (if anything) lives outside this codebase, so there is nothing here to wire up
  without inventing an unverifiable click target.

## Risks / Trade-offs

- **[Risk]** Reversing the documented "match Telegram/email" rationale in
  `notificationTarget.ts` for these three kinds only, while leaving `follow_up`/`interview_prep`
  on the old rule, leaves the file's remaining kinds inconsistent with each other on purpose. →
  **Mitigation**: the comment explaining the split is rewritten as part of this change, so a
  future reader sees this was a deliberate, scoped reversal rather than a partial fix left behind.
- **[Risk]** `approved`'s "View tailored CV" link points at `/tailor/[slug]`, the same workspace
  route `pending_review` uses — if that workspace ever adds an editing affordance, an already
  approved-and-queued attempt could be edited out from under the auto-apply submission it is
  queued for. → **Mitigation**: out of scope here; the workspace's own read/write behavior is
  unchanged by this design, and `/tailor/[slug]`'s current behavior (idempotently resolving the
  existing tailored CV) is exactly what `pending_review` already relies on today.
