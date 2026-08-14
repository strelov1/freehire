## ADDED Requirements

### Requirement: Account timezone

A signed-in user's account SHALL carry an optional IANA timezone name (e.g.
`Europe/Moscow`), editable on `/my/profile`. An account with no timezone set
SHALL be treated as UTC for every purpose that reads it.

#### Scenario: Set a timezone

- **WHEN** a signed-in user sets a valid IANA timezone name on their profile
- **THEN** the system stores it and uses it to interpret their digest time
  and quiet-hours window

#### Scenario: Invalid timezone rejected

- **WHEN** a signed-in user submits a timezone name that is not a valid
  IANA zone
- **THEN** the system rejects the update and the stored value is unchanged

#### Scenario: No timezone set

- **WHEN** an account has never set a timezone
- **THEN** the system treats it as UTC when evaluating quiet hours or a
  daily digest time

#### Scenario: Captured at password registration

- **WHEN** a new account registers with email/password from the web app
- **THEN** the browser's detected timezone is stored on the account at
  creation, without requiring a later profile visit

#### Scenario: Registration timezone is best-effort

- **WHEN** a registration request carries a missing or invalid timezone
  value
- **THEN** the account is still created (with no timezone set) rather than
  the signup failing

#### Scenario: Profile pre-fills the detected zone when unset

- **WHEN** a signed-in user with no stored timezone opens `/my/profile`
- **THEN** the timezone field shows the browser's detected zone as its
  selected value, and saving the form (even without touching that field)
  stores it

### Requirement: Saved-search digest frequency

A signed-in user SHALL be able to set one account-wide delivery frequency
for saved-search alerts: `instant` (deliver as soon as a match is claimed,
the default) or `daily` (deliver once per day at a chosen local time). This
setting governs only saved-search subscription digests; it has no effect on
saved-job reminders or lifecycle nudges.

#### Scenario: Instant is the default

- **WHEN** an account has never configured a frequency
- **THEN** saved-search digests deliver as soon as they are claimed, same as
  before this feature existed

#### Scenario: Switch to daily

- **WHEN** a signed-in user sets frequency to `daily` and a delivery time
- **THEN** a saved search's pending matches accumulate and are delivered
  together the first time the local clock passes that time each day

#### Scenario: Daily digest is not re-sent same-day

- **WHEN** a `daily`-mode digest has already been delivered for the current
  local calendar day
- **THEN** no further digest is sent for that saved search until the next
  local calendar day's delivery time

#### Scenario: Missed pass resumes next day, not immediately

- **WHEN** no worker pass runs during the delivery-time window on a given
  local day
- **THEN** that day's digest is skipped rather than delivered late, and
  delivery resumes at the next day's window

### Requirement: Quiet hours

A signed-in user SHALL be able to set an optional quiet-hours window
(start/end local time). While the current local time falls inside the
window, delivery of saved-job reminders, lifecycle nudges, and
`instant`-frequency saved-search alerts SHALL be deferred to the next
worker pass after the window ends, without being dropped, retried
prematurely, or counted as a failed delivery attempt. A `daily`-frequency
saved-search digest is delivered at its configured time regardless of quiet
hours.

#### Scenario: Delivery deferred during quiet hours

- **WHEN** a reminder, nudge, or instant-frequency alert becomes due while
  the account's local time is inside its quiet-hours window
- **THEN** delivery is deferred and retried on a subsequent pass, without
  being marked failed or losing the underlying record

#### Scenario: Delivery resumes after the window ends

- **WHEN** a worker pass runs after the account's local time has left the
  quiet-hours window
- **THEN** any notification that was deferred during the window is
  delivered on that pass

#### Scenario: Overnight window

- **WHEN** the quiet-hours window spans midnight (start time later than end
  time, e.g. 22:00–08:00)
- **THEN** the window is evaluated as continuous overnight, correctly
  covering times both before and after midnight

#### Scenario: Daily digest ignores quiet hours

- **WHEN** an account's chosen daily digest time falls inside its own
  quiet-hours window
- **THEN** the digest still delivers at the chosen time

#### Scenario: Quiet hours off by default

- **WHEN** an account has never configured quiet hours
- **THEN** no delivery is ever deferred on that basis

### Requirement: Delivery-timing settings UI

`/my/notifications/settings` SHALL present controls for saved-search
digest frequency (with a time picker shown only when `daily` is selected)
and for the quiet-hours window (optional start/end), alongside the existing
channel settings. When the account has no timezone set and the user enables
`daily` frequency or a quiet-hours window, the UI SHALL surface a hint
pointing at the profile page to set one, without blocking the save.

#### Scenario: Daily frequency reveals a time picker

- **WHEN** a user selects `daily` frequency in the settings UI
- **THEN** a time picker appears for choosing the digest delivery time

#### Scenario: Missing-timezone hint

- **WHEN** a user without a stored timezone enables `daily` frequency or a
  quiet-hours window
- **THEN** the UI shows a hint pointing at the profile page, and the setting
  still saves (interpreted as UTC until a timezone is set)
