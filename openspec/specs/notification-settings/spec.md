# notification-settings Specification

## Purpose

The single account-level rule — an enabled flag and a channel set — that governs every
notification the product ORIGINATES about a candidate's own activity: saved-job
reminders and both lifecycle nudges. One rule, one place to turn it off, and no
per-kind or per-job override of it.

It does NOT govern a transactional message about something the user is a PARTY to — a
mentorship session's confirmation, cancellation and pre-session reminders reach both
sides regardless. That is a boundary on what the rule covers, not an override inside it:
a user who silenced nudges must still turn up to a meeting somebody is holding an hour
for.

## Requirements

### Requirement: Single account-level notification gate

The system SHALL maintain one per-user notification rule holding an `enabled` flag,
a set of delivery channels, and one email switch per silenceable mail group. The
`enabled` flag SHALL govern the `activity` group — saved-job reminders, follow-up
nudges, and interview-prep nudges — and SHALL NOT govern any other group; there
SHALL be no per-notification-kind, per-stage, or per-job override of it. A separate
switch SHALL govern the `alerts` group (saved-search digests) and a third SHALL
govern the `news` group (one-off campaigns, the onboarding sequence, referral
pings), so that declining one group leaves the others untouched.

#### Scenario: Enabling notifications turns on all three activity kinds

- **WHEN** an authenticated user enables their notification rule with channel
  `email`
- **THEN** the system persists the rule
- **AND** subsequent saved-job reminders, follow-up nudges, and interview-prep
  nudges are all gated by this one flag and delivered over `email`

#### Scenario: Disabling notifications turns off all three activity kinds

- **WHEN** a user with the rule enabled sets it to disabled
- **THEN** no new saved-job reminder, follow-up nudge, or interview-prep nudge is
  scheduled for that user
- **AND** already-pending items are cancelled rather than delivered once their
  condition is next checked

#### Scenario: Disabling notifications no longer silences campaigns

- **WHEN** a user sets the `enabled` flag to disabled
- **THEN** one-off campaigns and the onboarding sequence continue, governed by the
  `news` switch alone

#### Scenario: Declining news leaves activity mail alone

- **WHEN** a user turns the `news` switch off
- **THEN** campaigns, onboarding mail, and referral pings stop for that account
- **AND** saved-job reminders and lifecycle nudges continue

#### Scenario: Declining alerts leaves activity and news alone

- **WHEN** a user turns the `alerts` switch off
- **THEN** no saved-search digest is delivered by email for that account
- **AND** lifecycle nudges and campaigns continue

### Requirement: New accounts default to enabled

An account with no notification rule configured SHALL be treated as enabled for
every group: the `activity` group with channel `email`, the `alerts` group on, and
the `news` group on. This default applies only to accounts that have never
configured the rule — an account with an existing, explicitly-set rule SHALL keep
that value; the default's own change SHALL NOT alter any row that already exists.
Introducing the `alerts` and `news` switches SHALL NOT change what any existing
account receives.

#### Scenario: A never-configured account is notified

- **WHEN** a user who has never opened their notification settings saves a job
- **THEN** a saved-job reminder is scheduled, as if the user had explicitly enabled
  notifications with channel `email`

#### Scenario: An existing explicit choice is preserved

- **WHEN** an account already has a notification rule row with an explicit value
  (enabled or disabled) set before this default changed
- **THEN** that account's rule continues to read exactly as it was explicitly set,
  unaffected by the new default for never-configured accounts

#### Scenario: Adding the group switches changes nobody's mail

- **WHEN** the `alerts` and `news` switches are introduced for an account that
  already has a rule row and for an account that has none
- **THEN** both accounts receive exactly the mail they received before, because both
  switches default to on

#### Scenario: A never-configured account can still decline news

- **WHEN** a user who has never opened their notification settings turns the `news`
  switch off through an unsubscribe link
- **THEN** the rule row is created carrying that choice, and the account's other
  groups keep their defaults

### Requirement: Notification settings UI

The system SHALL expose the notification rule on its own account-navigation section,
separate from the activity page and from the saved-search subscription list. That
section SHALL show all three group switches — `activity` with its channel choice,
`alerts`, and `news` — so a signed-in user sees and controls the same preferences
the public unsubscribe page exposes.

#### Scenario: Settings page is reachable from account navigation

- **WHEN** a signed-in user opens the account navigation
- **THEN** a "Notifications" section is available, showing the current enabled
  state and channel choice

#### Scenario: All three group switches are shown

- **WHEN** a signed-in user opens the notification settings section
- **THEN** the `activity`, `alerts`, and `news` switches are all present and reflect
  the account's current values

### Requirement: A group switch is honoured at selection time

Each group's switch SHALL be applied when the system selects who to mail, not after
a message has been rendered, so that a delivery path added later inherits the gate
rather than having to remember it. An account with no rule row SHALL be treated as
having the group on for `alerts` and `news`, and as configured today for `activity`.

#### Scenario: An opted-out account is never selected

- **WHEN** the system builds the list of accounts to receive a campaign
- **THEN** accounts whose `news` switch is off are absent from the list, rather than
  being filtered out at send time

#### Scenario: A missing rule row still receives alerts and news

- **WHEN** the system builds a mailing list and an account has no notification rule
  row at all
- **THEN** that account is included for the `alerts` and `news` groups
