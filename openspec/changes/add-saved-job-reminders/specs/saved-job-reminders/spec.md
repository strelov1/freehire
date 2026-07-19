## ADDED Requirements

### Requirement: Account-level reminder default rule

The system SHALL maintain a per-user reminder default rule with an enabled flag, a
default delay (in days), and a set of delivery channels. The rule governs whether and
how reminders are scheduled for newly saved jobs. The default rule SHALL be disabled
until the user opts in, so no reminders are created for users who never touch the
setting.

#### Scenario: User enables reminders with a default delay

- **WHEN** an authenticated user sets their reminder default rule to enabled with a
  delay of 3 days and channel `telegram`
- **THEN** the system persists the rule and returns it
- **AND** subsequent saves schedule a reminder 3 days out over Telegram

#### Scenario: Feature is off by default

- **WHEN** a user who has never configured reminders saves a job
- **THEN** no reminder is scheduled

#### Scenario: User disables reminders globally

- **WHEN** a user with the rule enabled sets the rule to disabled
- **THEN** the default rule no longer schedules reminders for new saves
- **AND** existing pending reminders are left intact unless individually cancelled

### Requirement: Scheduling a reminder on save

When a user saves a job, the system SHALL schedule at most one pending reminder for that
`(user, job)` pair, using the account default delay unless the save request supplies a
per-job override. A per-job override MAY set a custom delay or opt out of a reminder for
that job entirely. An explicit delay override SHALL take effect even when the account rule
is disabled — a per-save "remind me" is a self-standing opt-in — falling back to the email
channel when the rule has none configured. Without an override, a reminder is scheduled
only when the account rule is enabled. Reminders SHALL only be scheduled for jobs that are
saved and not yet applied.

#### Scenario: Save uses the account default

- **WHEN** a user with an enabled 3-day default saves a job without an override
- **THEN** a pending reminder is created with a fire time 3 days from the save

#### Scenario: Save overrides the delay for one job

- **WHEN** the save request specifies a "tomorrow" override
- **THEN** the reminder fire time is 1 day from the save, regardless of the account default

#### Scenario: Save opts out for one job

- **WHEN** the save request specifies "don't remind"
- **THEN** no reminder is created for that job even though the account default is enabled

#### Scenario: Explicit override schedules while the rule is disabled

- **WHEN** a user whose reminder rule is disabled saves a job with a "tomorrow" override
- **THEN** a pending reminder is created 1 day out, delivered over the email channel
- **AND** an ordinary save (no override) by that same user schedules nothing

#### Scenario: Re-saving replaces the pending reminder

- **WHEN** a job that already has a pending reminder is saved again with a new override
- **THEN** the pending reminder is replaced by one reflecting the new choice, and no
  duplicate reminder exists for the pair

### Requirement: One-shot delivery

A reminder SHALL fire exactly once at or after its scheduled fire time and then be marked
delivered. A due reminder SHALL be delivered as a message over each channel in the rule's
channel set for which the user has a usable destination, reusing the existing notification
delivery engine. Delivery SHALL be idempotent under worker retries: a reminder already
marked delivered is never sent again.

#### Scenario: Due reminder is delivered once

- **WHEN** the reminder worker runs after a reminder's fire time has passed
- **THEN** the user receives one reminder message per configured channel with a usable
  destination
- **AND** the reminder is marked delivered

#### Scenario: Worker re-run does not resend

- **WHEN** the worker runs again after a reminder was already delivered
- **THEN** no additional message is sent for that reminder

#### Scenario: Channel has no destination

- **WHEN** a reminder's rule includes `telegram` but the user has not linked Telegram
- **THEN** that channel is skipped without failing the reminder, and remaining channels
  still deliver

#### Scenario: Not yet due

- **WHEN** the worker runs before a reminder's fire time
- **THEN** the reminder is left pending and nothing is sent

### Requirement: Automatic cancellation

The system SHALL cancel a pending reminder before it fires when the underlying intent no
longer holds: when the user marks the job `applied`, when the user unsaves the job, or
when the job closes. A cancelled reminder SHALL NOT be delivered.

#### Scenario: Applying cancels the reminder

- **WHEN** a user marks a job with a pending reminder as applied
- **THEN** the pending reminder is cancelled and never delivered

#### Scenario: Unsaving cancels the reminder

- **WHEN** a user unsaves a job that has a pending reminder
- **THEN** the pending reminder is cancelled

#### Scenario: Job closure cancels the reminder

- **WHEN** a job with pending reminders is closed
- **THEN** those reminders are cancelled and no reminder is sent for the dead job

### Requirement: Per-job reminder management

The system SHALL expose, for each saved job, its pending reminder state (fire time or
none) and allow the user to reschedule the reminder to a new delay or cancel it, without
unsaving the job.

#### Scenario: Reschedule a pending reminder

- **WHEN** a user reschedules a saved job's reminder to a new delay
- **THEN** the pending reminder's fire time is updated accordingly

#### Scenario: Turn off a single reminder

- **WHEN** a user turns off the reminder for one saved job
- **THEN** that job's pending reminder is cancelled while the job stays saved

#### Scenario: Saved-jobs listing exposes reminder state

- **WHEN** a user views their saved jobs
- **THEN** each saved job shows whether a reminder is pending and, if so, when it fires
