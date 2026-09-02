# saved-job-reminders Specification

## Purpose

The one-shot nudge a saved job earns: come back before the vacancy goes stale. Every
save schedules at most one, at a fixed delay, gated by the shared
`notification-settings` rule; applying, unsaving, or the job closing cancels it before
it fires.

## Requirements

### Requirement: Scheduling a reminder on save

When a user saves a job, the system SHALL schedule at most one pending reminder for
that `(user, job)` pair, using the fixed account default delay. A reminder is
scheduled only when the shared `notification-settings` rule is enabled for that
user. Reminders SHALL only be scheduled for jobs that are saved and not yet applied.

The scheduled fire time SHALL be the first occurrence of the account's daily
notification hour at or after the default delay. The notification hour is
`notification_settings.digest_time` interpreted in the account's timezone; an account
with no configured digest time SHALL use 09:00, and an account with no configured
timezone SHALL be treated as UTC. Rounding SHALL only move the fire time forward,
never earlier than the default delay.

This is what lets a day's saves be delivered together: every save made on one day
lands on one of exactly TWO fire times — the notification hour on the delay's day for
saves at or before that hour, and the next day's for saves after it. It is not a
guarantee that all of a day's saves share one fire time, and a spec MUST NOT claim
one: honouring the delay floor and honouring a fixed hour cannot both hold for saves
that straddle that hour.

#### Scenario: Save schedules at the fixed default delay, rounded to the notification hour

- **WHEN** a user with notifications enabled and no configured digest time saves a job
- **THEN** a pending reminder is created whose fire time is the first 09:00 in the
  account's timezone at or after the fixed default delay from the save

#### Scenario: Saves on the same side of the notification hour become due together

- **WHEN** the same user saves two jobs several hours apart on one day, both after the
  account's notification hour
- **THEN** both pending reminders carry the same fire time

#### Scenario: Saves straddling the notification hour fall into two fire times

- **WHEN** the same user saves one job before the account's notification hour and
  another after it on the same day
- **THEN** the two reminders carry fire times one day apart, each still at the
  notification hour and each still at or after the default delay

#### Scenario: Configured digest time is the rounding target

- **WHEN** a user whose digest time is 18:00 and whose timezone is `Europe/Berlin`
  saves a job
- **THEN** the reminder's fire time is the first 18:00 Berlin time at or after the
  fixed default delay from the save

#### Scenario: Notifications disabled means no reminder

- **WHEN** a user with notifications disabled saves a job
- **THEN** no reminder is created

### Requirement: One-shot delivery

A reminder SHALL fire exactly once at or after its scheduled fire time and then be marked
delivered. Due reminders SHALL be delivered grouped by user AND by the channel set
snapshotted on the reminder: all of one account's due, still-actionable reminders in a
pass that share a channel set form one batch, and that batch SHALL be sent as a single
message over each of those channels for which the user has a usable destination, reusing
the existing notification delivery engine. One account MAY therefore produce more than one
batch in a pass. Delivery SHALL be idempotent under worker retries: a reminder already
marked delivered is never sent again. If the account's local time falls inside its
configured quiet-hours window when its reminders become due, delivery of that account's
batches SHALL be deferred to a later worker pass rather than sent or dropped.

#### Scenario: Due reminders for one user arrive as one message

- **WHEN** the reminder worker runs and three of one account's reminders are due
- **THEN** the user receives exactly one reminder message per configured channel with a
  usable destination, listing all three jobs
- **AND** all three reminders are marked delivered

#### Scenario: Due reminder is delivered once

- **WHEN** the reminder worker runs after a reminder's fire time has passed
- **THEN** the user receives one reminder message per configured channel with a usable
  destination
- **AND** the reminder is marked delivered

#### Scenario: Reminders for different users are not merged

- **WHEN** the worker runs and two different accounts each have due reminders
- **THEN** each account receives its own message and neither message lists the other
  account's jobs

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

#### Scenario: Due reminder deferred during quiet hours

- **WHEN** a reminder becomes due while the account's local time is inside
  its configured quiet-hours window
- **THEN** delivery is deferred (the claim is released, not marked
  delivered or failed) and retried on a later pass

#### Scenario: A cancelled reminder leaves the batch

- **WHEN** one of an account's due reminders is no longer actionable (its job closed,
  or the user applied or unsaved) while the others still are
- **THEN** that reminder is cancelled and excluded from the message, and the remaining
  reminders are still delivered as one message

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


