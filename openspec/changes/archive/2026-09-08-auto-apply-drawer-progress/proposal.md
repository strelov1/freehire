## Why

A candidate whose auto-apply attempt is `tailoring` (CV not produced yet) or `approved` (queued
for unattended submission) currently sees nothing in the tracker drawer — only `pending_review`,
`blocked`, `declined`, and `failed` render a banner. There is no way to tell, from the tracker,
that auto-apply is even working on a job, or to open the tailored CV once it has already been
approved. Separately, the submitted/blocked/failed outcome notifications (email, Telegram, in-app)
all link to the general `/my/tracking` board rather than the specific application, so a candidate
who taps one has to find the job again by hand — even though the notification already knows
exactly which job it is about.

## What Changes

- The tracker drawer's auto-apply banner gains two new read-only states:
  - `tailoring`: a neutral "preparing your tailored CV" status, no action (there is nothing to
    open yet).
  - `approved`: a "queued for automatic submission" status with a "View tailored CV" link —
    the same destination `pending_review` already links to — but no Approve/Decline actions,
    since the decision is already made.
- The three auto-apply outcome notifications — submitted, blocked, failed — deep-link to the
  specific application's tracker drawer instead of the general tracking board, on every channel
  that already carries a per-job slug for them (email, Telegram, in-app). A batch covering more
  than one job still links to the general board, since there is no single application to deep-link
  to.
- Out of scope: follow-up/interview-prep nudges (kept on the general board, by explicit choice)
  and push notifications (no click-target handling exists in this repo for push today).
- The board (Kanban and list view) gains a "Needs attention" toggle next to the existing search
  field, narrowing every column to only the applications whose auto-apply status already earns
  the existing "Review" badge (`pending_review`/`blocked`) — combinable with the text search, the
  same way the two views already share `matchesQuery`.
- The board card's existing "Review" badge gains a small red dot alongside its text, so an
  application needing attention is visually distinct at a glance, not only readable as a label.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `auto-apply-tracker-review`: adds visibility for the `tailoring`/`approved` statuses in the
  tracker drawer, adds the requirement that the submitted/blocked/failed outcome notifications
  deep-link to the specific application they are about, and adds a board-level "needs attention"
  filter plus a stronger visual marker on the existing review badge.

## Impact

- `web/src/lib/autoApplyReview.ts` — new `tailoring`/`approved` banner variants.
- `web/src/lib/components/JobDrawer.svelte` — renders the two new banner variants.
- `web/src/lib/notificationTarget.ts` — the three outcome kinds resolve to a slug-carrying
  tracking target instead of the bare board.
- `internal/engage/nudge/transports.go` — Telegram and email single-message rendering for the
  three outcome kinds link to `/my/tracking/<slug>` instead of `/my/tracking`.
- `web/src/lib/board.ts` — new pure `needsAttention()` predicate, reused by both the board's
  filter toggle and the existing card badge.
- `web/src/lib/components/BoardCard.svelte` — red dot alongside the existing "Review" badge.
- Tests: `web/src/lib/autoApplyReview.test.ts`, `web/src/lib/notificationTarget.test.ts`,
  `web/src/lib/board.test.ts`, `internal/engage/nudge/transports_test.go`.
