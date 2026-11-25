# lifecycle-nudges Specification

## Purpose

The one-shot nudges a tracked application earns from its own lifecycle: a follow-up
when it has gone silent past its stage's tolerated threshold, and interview prep when
it moves into `interview`. Both are matched from current state on every pass and
deduplicated by an episode key, so a condition that persists is nudged once, not once
per pass.
## Requirements
### Requirement: Follow-up nudge on silence past threshold

The system SHALL identify tracked applications whose current silence state (per
`userjob.SilenceStateFor`, driven by the stage's tolerated-silence threshold and
`last_activity_at`) is `silent`, and SHALL deliver at most one follow-up nudge per
distinct silence episode. A silence episode is identified by the application's
`last_activity_at` value at the time it is first matched: as long as that value is
unchanged, the same episode has already been nudged and SHALL NOT be nudged again,
regardless of how many further passes find it still silent.

#### Scenario: Application crosses the silence threshold

- **WHEN** a tracked application's silence state becomes `silent`
- **THEN** a follow-up nudge is scheduled for delivery

#### Scenario: Continued silence does not re-nudge

- **WHEN** an application already nudged for its current silence episode is found
  silent again on a later pass, with `last_activity_at` unchanged
- **THEN** no additional nudge is scheduled

#### Scenario: New activity starts a fresh episode

- **WHEN** an application's `last_activity_at` advances (e.g. new inbound mail is
  linked) after a follow-up nudge was already delivered for the prior episode, and
  it later becomes silent again
- **THEN** a new follow-up nudge is scheduled for the new episode

### Requirement: Interview-prep nudge on stage transition

The system SHALL identify `stage_set` events transitioning an application to
`interview`, and SHALL deliver at most one interview-prep nudge per distinct
transition event.

#### Scenario: Application enters the interview stage

- **WHEN** an application's stage is set to `interview`
- **THEN** an interview-prep nudge is scheduled for delivery

#### Scenario: Re-entering the interview stage nudges again

- **WHEN** an application that already received an interview-prep nudge for an
  earlier `stage_set → interview` event transitions into `interview` again (a later
  round)
- **THEN** a new interview-prep nudge is scheduled for the new transition

### Requirement: Delivery gated by the shared notification setting

Both nudge kinds SHALL only be matched and delivered for users whose shared
`notification-settings` rule is enabled, over the channels that rule configures.

#### Scenario: Notifications disabled

- **WHEN** a user's notification setting is disabled
- **THEN** no follow-up or interview-prep nudge is matched or delivered for that
  user

#### Scenario: Disabled between match and delivery

- **WHEN** a nudge was matched while notifications were enabled, and the user
  disables notifications before it is delivered
- **THEN** the nudge is cancelled, not delivered

### Requirement: Re-check before send

Before delivering a matched nudge, the system SHALL re-verify that its triggering
condition still holds, and SHALL cancel rather than deliver a nudge whose condition
no longer holds.

#### Scenario: Follow-up condition no longer holds

- **WHEN** a matched follow-up nudge's application has received a reply (silence
  state no longer `silent`) by the time delivery is attempted
- **THEN** the nudge is cancelled, not delivered

#### Scenario: Interview-prep condition no longer holds

- **WHEN** a matched interview-prep nudge's application has moved off the
  `interview` stage by the time delivery is attempted
- **THEN** the nudge is cancelled, not delivered

### Requirement: One-shot delivery per matched nudge

A matched, still-actionable nudge SHALL be delivered as a message over each channel
in the user's configured channel set for which a usable destination exists, and then
marked delivered. Delivery SHALL be idempotent under worker retries.

#### Scenario: Nudge delivered once

- **WHEN** the nudge worker runs after a nudge was matched
- **THEN** the user receives one message per configured channel with a usable
  destination, and the nudge is marked delivered

#### Scenario: Worker re-run does not resend

- **WHEN** the worker runs again after a nudge was already delivered
- **THEN** no additional message is sent for that nudge

### Requirement: Submitted nudge on a successful auto-apply

The system SHALL identify a job for which an unattended auto-apply attempt has
successfully submitted a real application, and SHALL deliver at most one
"submitted" nudge per distinct submission.

#### Scenario: Auto-apply submits an application

- **WHEN** an unattended auto-apply attempt successfully submits a job's
  application
- **THEN** a submitted nudge is scheduled for delivery

#### Scenario: Re-scanning the same submission does not re-nudge

- **WHEN** a submission already nudged is found again on a later matching pass
- **THEN** no additional nudge is scheduled

### Requirement: Blocked nudge on a permanently parked auto-apply attempt

The system SHALL identify an auto-apply attempt that has been permanently parked
because it could not answer a required question unattended, and SHALL deliver at
most one "blocked" nudge per distinct attempt. An attempt that is ALSO
dead-lettered (see the failed nudge below) SHALL NOT be matched as blocked — the
two are mutually exclusive for one attempt.

#### Scenario: Auto-apply attempt is blocked on a required question

- **WHEN** an unattended auto-apply attempt is permanently parked for lacking an
  answer to a required question
- **THEN** a blocked nudge is scheduled for delivery

#### Scenario: Re-scanning an already-nudged blocked attempt does not re-nudge

- **WHEN** a blocked attempt already nudged is found again on a later matching
  pass
- **THEN** no additional nudge is scheduled

#### Scenario: An attempt parked and later dead-lettered is not double-nudged

- **WHEN** an unattended auto-apply attempt carries both a parked marker and a
  dead-letter marker
- **THEN** only a failed nudge is scheduled for delivery, never a blocked one

### Requirement: Failed nudge on a dead-lettered auto-apply attempt

The system SHALL identify an auto-apply attempt that has been dead-lettered —
whether because it exhausted its retry budget, or because the very first attempt
was judged too risky to retry (an unconfirmed submission, or a submission that
went through but could not be durably recorded) — and SHALL deliver at most one
"failed" nudge per distinct attempt. The nudge's own wording SHALL NOT assert that
a retry occurred, since a dead letter can be reached on the first attempt.

#### Scenario: Auto-apply exhausts its retries

- **WHEN** an unattended auto-apply attempt is dead-lettered after exhausting its
  retries
- **THEN** a failed nudge is scheduled for delivery

#### Scenario: Auto-apply dead-letters on the first attempt

- **WHEN** an unattended auto-apply attempt is dead-lettered immediately, with no
  prior retry (an unconfirmed submission, or a lost post-submit record)
- **THEN** a failed nudge is scheduled for delivery, worded without claiming a
  retry took place

#### Scenario: Re-scanning an already-nudged failed attempt does not re-nudge

- **WHEN** a failed attempt already nudged is found again on a later matching
  pass
- **THEN** no additional nudge is scheduled

### Requirement: Auto-apply outcome nudges gated by the shared notification setting

The submitted, blocked, and failed nudges SHALL only be matched and delivered for
users whose shared notification-settings rule is enabled, over the channels that
rule configures — the same gate the existing follow-up and interview-prep nudges
honor.

#### Scenario: Notifications disabled

- **WHEN** a user's notification setting is disabled
- **THEN** no submitted, blocked, or failed auto-apply nudge is matched or
  delivered for that user

### Requirement: Auto-apply outcome nudges are never cancelled by the pre-delivery re-check

Unlike the follow-up and interview-prep nudges, whose triggering condition can
lapse between matching and delivery, a matched submitted, blocked, or failed
nudge's triggering condition SHALL be treated as permanent: the system SHALL NOT
cancel it at delivery time.

#### Scenario: Time passes between matching and delivery

- **WHEN** a submitted, blocked, or failed nudge was matched, and delivery is
  attempted afterward
- **THEN** the nudge is delivered (subject only to the notification-setting gate
  above), never cancelled for its triggering condition having lapsed

