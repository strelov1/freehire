## MODIFIED Requirements

### Requirement: Withdrawing a profile preserves booking history

A mentor SHALL be able to withdraw their profile. Withdrawal SHALL cancel every
confirmed future booking and notify each affected seeker, and SHALL retain past
bookings as history rather than deleting them.

The profile row SHALL be MARKED withdrawn rather than deleted. Bookings and reviews
reference it with `ON DELETE CASCADE`, so a delete would erase every session that ever
happened — the opposite of what this requirement asks for. A withdrawn profile SHALL
leave the directory and the public read, exactly as a paused one does.

Withdrawal SHALL NOT change the mentor's own pause switch — status and pause are
independent decisions, and withdrawal is a status change only.

A repeat withdrawal of an already-withdrawn profile SHALL succeed without error: it is
a no-op that leaves the profile withdrawn, not a failure. The system SHALL still refuse
with a not-found error when the caller has no mentor profile at all.

#### Scenario: Withdrawal cancels the future and keeps the past

- **WHEN** a mentor with one past completed booking and two confirmed future bookings
  withdraws their profile
- **THEN** both future bookings become cancelled and both seekers are notified
- **AND** the completed booking is still readable in each party's history
- **AND** its review is still readable

#### Scenario: A withdrawn profile is no longer public

- **WHEN** a mentor withdraws
- **THEN** their profile is absent from the directory and from the public read
- **AND** a second withdrawal changes nothing

#### Scenario: A second withdrawal succeeds instead of failing

- **WHEN** a mentor whose profile is already withdrawn withdraws again
- **THEN** the request succeeds
- **AND** the profile remains withdrawn

#### Scenario: Withdrawal does not touch the pause switch

- **WHEN** a mentor withdraws a profile that was not paused
- **THEN** the profile's pause switch remains off
- **AND** the profile's status is `withdrawn`

#### Scenario: Withdrawing without ever having a profile is refused

- **WHEN** an account with no mentor profile at all requests withdrawal
- **THEN** the system refuses with a not-found error

## ADDED Requirements

### Requirement: A withdrawn mentor can resubmit for review

A mentor whose profile is `withdrawn` SHALL be able to resubmit it for moderation,
moving its status to `pending` and clearing the pause switch. Resubmission SHALL NOT
auto-approve the profile: it re-enters the same moderation queue a first-time
submission does, with no special treatment for having been a mentor before.

A resubmission SHALL be refused when the caller has no profile, or when their profile's
status is not `withdrawn` — resubmitting is not a way to force a `pending` or
`rejected` profile back to review out of turn, and an `approved` profile is not
withdrawn in the first place.

#### Scenario: A withdrawn mentor resubmits and awaits moderation again

- **WHEN** a mentor whose profile is `withdrawn` resubmits it
- **THEN** the profile's status becomes `pending`
- **AND** the profile's pause switch is off
- **AND** the profile appears in the moderation queue, oldest first
- **AND** the profile does not appear in the public directory

#### Scenario: Resubmitting a profile that was never withdrawn is refused

- **WHEN** a mentor whose profile is `pending`, `rejected` or `approved` requests
  resubmission
- **THEN** the system refuses with a conflict rather than a not-found error
- **AND** the profile's status is unchanged

#### Scenario: Resubmitting with no profile at all is refused

- **WHEN** an account with no mentor profile requests resubmission
- **THEN** the system refuses with a not-found error
