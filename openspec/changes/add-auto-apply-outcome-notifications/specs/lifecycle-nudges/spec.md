## ADDED Requirements

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
most one "blocked" nudge per distinct attempt.

#### Scenario: Auto-apply attempt is blocked on a required question

- **WHEN** an unattended auto-apply attempt is permanently parked for lacking an
  answer to a required question
- **THEN** a blocked nudge is scheduled for delivery

#### Scenario: Re-scanning an already-nudged blocked attempt does not re-nudge

- **WHEN** a blocked attempt already nudged is found again on a later matching
  pass
- **THEN** no additional nudge is scheduled

### Requirement: Failed nudge on a dead-lettered auto-apply attempt

The system SHALL identify an auto-apply attempt that has been dead-lettered after
exhausting its retries, and SHALL deliver at most one "failed" nudge per distinct
attempt.

#### Scenario: Auto-apply exhausts its retries

- **WHEN** an unattended auto-apply attempt is dead-lettered after exhausting its
  retries
- **THEN** a failed nudge is scheduled for delivery

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
