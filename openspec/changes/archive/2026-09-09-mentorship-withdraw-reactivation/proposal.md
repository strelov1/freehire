## Why

Withdrawing a mentor profile (`DELETE /me/mentorship/profile`) sets `paused = true`
alongside `status = 'withdrawn'`, so the owner-facing UI — which prefers the `paused`
flag over `status` when labelling the profile — shows a withdrawn profile as "paused".
Worse, the withdraw query only matches rows with `status <> 'withdrawn'`, so a second
withdrawal (or a page reload that resubmits it) affects zero rows and returns 404
"mentor not found", even though the existing `mentor-profile` spec already says "a
second withdrawal changes nothing" — the current 404 violates that. Finally, there is
no way back: the account's `UNIQUE(user_id)` constraint means the withdrawn row
permanently occupies the one profile slot, `Decide`/`SetPaused` never touch a withdrawn
row, and creating a new profile 409s. A mentor who withdrew by mistake, or who wants to
come back later, is stuck.

## What Changes

- `WithdrawMentorProfile` no longer sets `paused = true` — withdrawal only changes
  `status`, keeping the mentor's own pause switch and the moderation-derived status
  independent, as the schema's own design intends.
- Withdrawing an already-withdrawn profile is now idempotent: it succeeds (no error)
  instead of returning 404 "mentor not found", matching the existing spec text.
- A withdrawn mentor can resubmit for review: a new `Service.Reactivate` moves the
  profile from `withdrawn` back to `pending`, re-entering the ordinary moderation queue
  (no auto-approval, no special notification — same as the first submission). Reachable
  via a new `POST /me/mentorship/profile/reactivate` route.
- The owner-facing profile UI stops inferring "paused" from `profile.paused` alone: a
  non-`approved` status (`pending`, `rejected`, `withdrawn`) is shown as itself, and
  "paused" is only shown for an `approved` profile that is paused. A withdrawn profile
  shows a "Submit for review again" action instead of "Withdraw profile".

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `mentor-profile`: withdrawal becomes idempotent (no error on a repeat withdrawal, and
  it no longer touches the `paused` flag), and a new requirement lets a withdrawn mentor
  resubmit for moderation rather than being permanently locked out.

## Impact

- `internal/platform/db/queries/mentorship.sql` (+ regenerated `internal/platform/db`
  via `make sqlc`): `WithdrawMentorProfile` drops the `paused` write and the
  `status <> 'withdrawn'` guard; new `ReactivateMentorProfile` query.
- `internal/engage/mentorship/{profile,repository,fake_repo_test,profile_test}.go`:
  `Service.Reactivate`, repository method, fake repo, tests.
- `internal/api/handler/mentorship*.go`: new route + handler; `mentorshipError` gains one
  mapping, `ErrProfileNotWithdrawn` → 409, symmetric with the existing
  `ErrProfileNotPending` → 409 for the moderator decision guard.
- `web/src/lib/api.ts`, `web/src/lib/components/MentorProfileEditor.svelte`: reactivate
  call, badge fix, conditional action button.
- `internal/engage/mentorship/AGENTS.md`: the "withdrawal is a one-way door" framing
  needs updating.
