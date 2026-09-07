## MODIFIED Requirements

### Requirement: Single account-level notification gate

The system SHALL maintain one per-user notification rule with an `enabled` flag and
a set of delivery channels. This single rule SHALL govern every notification the
system originates about a user's activity — saved-job reminders, follow-up nudges, and
interview-prep nudges — and there SHALL be no per-notification-kind, per-stage, or
per-job override of it.

The rule SHALL NOT govern a transactional message about a mentorship session the user is
A PARTY TO — its confirmation, its cancellation, and its pre-session reminders — and this
SHALL apply to both the seeker and the mentor. Those SHALL be delivered regardless of the
rule.

The test is participation, not authorship: a mentor did not make the booking, the seeker
did, and the mentor is the party holding an hour for it. This is a boundary on what the
rule covers, not an override inside it — a user who silenced nudges must still turn up to
the session, or somebody's held time is wasted and the marketplace's core promise breaks.

#### Scenario: Enabling notifications turns on all three kinds

- **WHEN** an authenticated user enables their notification rule with channel
  `email`
- **THEN** the system persists the rule
- **AND** subsequent saved-job reminders, follow-up nudges, and interview-prep
  nudges are all gated by this one rule and delivered over `email`

#### Scenario: Disabling notifications turns off all three kinds

- **WHEN** a user with the rule enabled sets it to disabled
- **THEN** no new saved-job reminder, follow-up nudge, or interview-prep nudge is
  scheduled for that user
- **AND** already-pending items are cancelled rather than delivered once their
  condition is next checked

#### Scenario: Disabling notifications does not silence a booking the user made

- **WHEN** a user with the rule disabled books a mentorship session
- **THEN** the booking confirmation, its calendar invitation, its cancellation notice
  and its pre-session reminders are still delivered
- **AND** the user's saved-job reminders and nudges remain silenced
