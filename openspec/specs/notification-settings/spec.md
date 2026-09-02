# notification-settings Specification

## Purpose

The single account-level rule — an enabled flag and a channel set — that governs every
notification the product sends a candidate about their own activity: saved-job
reminders and both lifecycle nudges. One rule, one place to turn it off, and no
per-kind or per-job override of it.

## Requirements

### Requirement: Single account-level notification gate

The system SHALL maintain one per-user notification rule with an `enabled` flag and
a set of delivery channels. This single rule SHALL govern saved-job reminders,
follow-up nudges, and interview-prep nudges — there SHALL be no per-notification-kind,
per-stage, or per-job override of it.

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

### Requirement: New accounts default to enabled

An account with no notification rule configured SHALL be treated as enabled, with
channel `email`, rather than disabled. This default applies only to accounts that
have never configured the rule — an account with an existing, explicitly-set rule
SHALL keep that value; the default's own change SHALL NOT alter any row that
already exists.

#### Scenario: A never-configured account is notified

- **WHEN** a user who has never opened their notification settings saves a job
- **THEN** a saved-job reminder is scheduled, as if the user had explicitly enabled
  notifications with channel `email`

#### Scenario: An existing explicit choice is preserved

- **WHEN** an account already has a notification rule row with an explicit value
  (enabled or disabled) set before this default changed
- **THEN** that account's rule continues to read exactly as it was explicitly set,
  unaffected by the new default for never-configured accounts

### Requirement: Notification settings UI

The system SHALL expose the notification rule (enabled flag and channel choice) on
its own account-navigation section, separate from the activity page and from the
saved-search subscription list.

#### Scenario: Settings page is reachable from account navigation

- **WHEN** a signed-in user opens the account navigation
- **THEN** a "Notifications" section is available, showing the current enabled
  state and channel choice
