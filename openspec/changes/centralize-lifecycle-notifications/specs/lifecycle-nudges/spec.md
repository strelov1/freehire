## ADDED Requirements

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
