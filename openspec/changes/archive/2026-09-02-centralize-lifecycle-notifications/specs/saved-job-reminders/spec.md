## MODIFIED Requirements

### Requirement: Scheduling a reminder on save

When a user saves a job, the system SHALL schedule at most one pending reminder for
that `(user, job)` pair, using the fixed account default delay. A reminder is
scheduled only when the shared `notification-settings` rule is enabled for that
user. Reminders SHALL only be scheduled for jobs that are saved and not yet applied.

#### Scenario: Save schedules at the fixed default delay

- **WHEN** a user with notifications enabled saves a job
- **THEN** a pending reminder is created with a fire time at the fixed default delay
  from the save

#### Scenario: Notifications disabled means no reminder

- **WHEN** a user with notifications disabled saves a job
- **THEN** no reminder is created

## REMOVED Requirements

### Requirement: Account-level reminder default rule

**Reason**: Superseded by the `notification-settings` capability, which holds one
shared enabled+channel rule for saved-job reminders and both lifecycle nudges —
this reminder-specific rule (and its per-feature configurable delay) no longer
exists as a separate concept.

**Migration**: Read/write the account-level notification rule through
`notification-settings` instead. There is no longer a configurable default delay —
`ScheduleOnSave` uses the fixed `DefaultDelayDays` constant.

### Requirement: Per-job reminder management

**Reason**: Confirmed by direct product decision: this control (reschedule a saved
job's reminder, turn it off individually) has never been exercised by any account
other than the developer's. Removed rather than simplified, to match the single
account-level toggle the rest of this change centralizes around.

**Migration**: None — there is no replacement. A saved job's reminder follows the
account-level `notification-settings` rule only; there is no per-job state to read
or mutate. The saved-jobs listing no longer exposes a per-job reminder fire time.
