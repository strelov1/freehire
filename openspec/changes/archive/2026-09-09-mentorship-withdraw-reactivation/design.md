## Context

See proposal.md - Why. The relevant existing shapes:

- `mentors.status` is `pending | approved | rejected | withdrawn` (check constraint,
  migration 0145); `mentors.paused` is a separate boolean, deliberately independent
  (`internal/engage/mentorship/profile.go:13-16`).
- `mentors.user_id` is `UNIQUE` — one profile row per account for life; withdrawal marks
  it, never deletes it (`ON DELETE CASCADE` on bookings/reviews is why).
- `Service.Withdraw` already loads `s.ownProfile(ctx, userID)` first, which raises
  `ErrProfileNotFound` if the caller has no profile at all — this happens before the
  repository's withdraw call, regardless of the row's status.
- `DecideProfile`'s SQL guard (`WHERE id = ... AND status = 'pending'`) and its mapping
  to `ErrProfileNotPending` → 409 is the existing precedent for "the row exists but is
  in the wrong status for this operation."

## Goals / Non-Goals

**Goals:**
- Withdrawal stops writing `paused`; a repeat withdrawal succeeds instead of 404ing.
- A withdrawn mentor can resubmit into the ordinary moderation queue.
- The owner-facing status badge reflects `status` and `paused` as the independent
  signals they are.

**Non-Goals:**
- No auto-approval path for a resubmission, and no different queue or priority for a
  returning mentor — it is exactly a first-time submission's `pending` state.
- No change to the public directory or public-read predicates (`approved AND NOT
  paused`) — those are already correct and untouched by this change.
- No schema change: `status` already has every value this needs (`pending` is reused
  for resubmission), so no migration is required.

## Decisions

**Withdraw becomes idempotent by dropping the status guard, not by special-casing "already withdrawn."**
`WithdrawMentorProfile` changes from
`UPDATE ... WHERE user_id = $1 AND status <> 'withdrawn'` to
`UPDATE ... WHERE user_id = $1`. Since `Service.Withdraw` already confirmed the row
exists via `ownProfile` before calling this, the only way this now affects 0 rows is a
race with another delete of the same row — vanishingly rare for a per-account
resource — and treating that as `ErrProfileNotFound` is still correct in that case.
Alternative considered: keep the guard and catch 0-rows in the repository to swallow it
into a success — rejected because it hides the real "no profile at all" case behind the
same code path the ownProfile check already guards for free.

**Reactivation is a new method (`Reactivate`), not a parameter on an existing one.**
`SetPaused` and `Decide` each mean one thing; overloading either with "and also maybe
move a withdrawn profile to pending" would put a second concept behind an existing name.
A dedicated `Service.Reactivate(ctx, userID) (Profile, error)` + `POST
/me/mentorship/profile/reactivate` mirrors the existing `/profile/pause` shape.

**A new sentinel, `ErrProfileNotWithdrawn`, guards the wrong-status case — symmetric with `ErrProfileNotPending`.**
`ReactivateMentorProfile`'s SQL is `UPDATE mentors SET status = 'pending', paused =
false WHERE user_id = $1 AND status = 'withdrawn'`. Zero rows is ambiguous between "no
profile" and "profile exists but isn't withdrawn"; the repository resolves it the same
way `UpdateProfile`/`SetPaused` already do for their own ambiguous zero-rows case: by
checking `ownProfile` first, exactly as `Withdraw` does. If the profile exists but isn't
`withdrawn`, that's `ErrProfileNotWithdrawn` → 409 "this profile is not withdrawn"; if
no profile exists at all, `ErrProfileNotFound` → 404, unchanged.

**The badge fix reorders the existing two fields; it does not add a third UI state.**
`MentorProfileEditor.svelte`'s badge becomes: show `paused` only when `status ===
'approved'`; otherwise show `status` itself. This is a rendering-order fix, not a new
label — `withdrawn`, `pending`, `rejected` already exist as strings the type system
carries (`OwnMentorProfile.status`).

## Risks / Trade-offs

- [Dropping the status guard on withdraw could mask a genuine bug if some other caller
  ever relied on 0-rows meaning "already withdrawn"] → No other caller reads
  `WithdrawMentorProfile`'s row count except `Service.Withdraw`, which no longer needs
  to distinguish the cases now that idempotency is the desired behavior per spec.
- [`ErrProfileNotWithdrawn` on a resubmission from `pending`/`rejected` could confuse a
  caller who expects "resubmit" to work from any non-approved state] → The spec scenario
  makes this explicit (409, not silently treated as pending-already); the UI never
  offers the action outside `status === 'withdrawn'`, so a real user never sees it.

## Migration Plan

Backend and frontend deploy together as usual (no feature flag: the old behavior — 404
on repeat withdrawal, no resubmission — was a bug and a gap, not a behavior anyone
depends on keeping). No data migration: existing `withdrawn` rows already have
`paused = true` from the old code path, which is harmless under the new badge logic
(paused is ignored for a non-`approved` status) and gets cleared the moment such a
mentor reactivates.
