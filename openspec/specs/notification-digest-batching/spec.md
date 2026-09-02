# notification-digest-batching Specification

## Purpose

The grouping rule the saved-job reminder and lifecycle nudge engines share: what forms
one message, how many items that message itemizes versus carries, what the in-app record
looks like, and how a failed group is retried. It exists because a per-item send turned
one busy day into a mailbox full of near-identical notifications.

## Requirements

### Requirement: A delivery group is the unit of sending

The saved-job reminder and lifecycle nudge engines SHALL deliver in groups rather than
per item. A reminder group is every one of an account's due, still-actionable reminders
in a worker pass **that share a channel set** — a reminder's channels are snapshotted
when it is scheduled, so an account that changed its rule between two saves has two
genuinely different deliveries due and they SHALL NOT be merged. A nudge group is every
one of an account's due, still-actionable nudges of the SAME kind in a worker pass; the
kinds (`follow_up`, `interview_prep`, `job_closed`) SHALL NOT be merged into one message,
because each carries a different call to action. A group of one SHALL be
indistinguishable from today's single-item delivery.

#### Scenario: Reminders with different channel sets are not merged

- **WHEN** one account has two due reminders, one scheduled for email only and one for
  email and Telegram
- **THEN** each is delivered over its own channel set, and neither message lists the
  other's job

#### Scenario: Channel order does not split a group

- **WHEN** two of an account's due reminders carry the same two channels in a different
  stored order
- **THEN** they are delivered as one message

#### Scenario: Same-kind nudges are one message

- **WHEN** the nudge worker delivers four `follow_up` nudges for one account in a pass
- **THEN** that account receives one `follow_up` message listing four applications

#### Scenario: Different kinds stay apart

- **WHEN** one account has two `follow_up` nudges and one `interview_prep` nudge due in
  the same pass
- **THEN** that account receives two messages: one `follow_up` message listing two
  applications, and one `interview_prep` message listing one

#### Scenario: A single-item group reads as it does today

- **WHEN** exactly one reminder is due for an account
- **THEN** the message is the single-job message, not a list of one

### Requirement: A grouped message itemizes fewer items than it carries

A grouped message SHALL itemize at most the first 10 items and indicate the remainder
as a count ("and N more"), while the in-app record for the same delivery SHALL carry up
to 200 items. These two bounds are separate values and SHALL NOT be collapsed into one:
lowering how much a message lists must not truncate the page that message links to.
Items SHALL be ordered oldest-due first, so the item that has waited longest is listed
first.

#### Scenario: A long group is truncated in the message only

- **WHEN** 25 reminders are delivered as one group
- **THEN** the message lists the 10 oldest-due jobs and says 15 more
- **AND** the in-app record carries all 25

#### Scenario: A group under the list limit has no tail

- **WHEN** 4 reminders are delivered as one group
- **THEN** the message lists all 4 and shows no "more" count

#### Scenario: Beyond the carried bound the excess is released, not delivered

- **WHEN** more of an account's items are due than the group may carry
- **THEN** the excess is released back to the pending queue with no delivery attempt
  recorded, so a later pass sends it as its own message
- **AND** nothing is marked delivered that appeared in no message

### Requirement: Grouping applies to every channel

A group SHALL be rendered as one message on every configured channel — email, Telegram
and mobile push alike. No channel SHALL fall back to per-item sending, because a channel
that does not group reproduces the flood the grouping exists to remove.

#### Scenario: Telegram receives one message per group

- **WHEN** a group of six reminders is delivered to an account with Telegram linked
- **THEN** the Telegram bot posts one message, not six

#### Scenario: Push receives one notification per group

- **WHEN** a group of six reminders is delivered to an account with a registered device
- **THEN** one push notification is delivered, summarizing the count

### Requirement: A grouped delivery records one in-app notification

A delivered group SHALL write exactly one `user_notifications` row. A group of more than
one item SHALL carry its job list in the row's `jobs` field and leave `public_slug`
unset, so the notification center renders the list page. A group of exactly one item
SHALL keep today's shape: `public_slug` set and `jobs` unset. A failure to record SHALL
NOT fail the delivery it accompanies.

#### Scenario: Multi-item group writes a list record

- **WHEN** a group of three reminders is delivered
- **THEN** one notification row is written whose `jobs` field holds all three jobs and
  whose `public_slug` is unset

#### Scenario: Single-item group writes a slug record

- **WHEN** a group of one reminder is delivered
- **THEN** one notification row is written whose `public_slug` is that job's slug and
  whose `jobs` field is unset

#### Scenario: Recording failure does not fail delivery

- **WHEN** writing the in-app record fails after the group was sent
- **THEN** the failure is logged and the group stays marked delivered

### Requirement: The group is the retry unit

A send failure SHALL be recorded against every item in the group, and the whole group
SHALL be retried on a later pass. As today, one channel succeeding is enough for the
group to count as delivered, and a co-channel error SHALL be logged rather than
retried. A channel that is unconfigured, or has no destination for the recipient, SHALL
remain a soft skip that burns no delivery attempt.

#### Scenario: A failed group send retries whole

- **WHEN** the only configured channel errors while sending a group of three
- **THEN** all three items record a delivery attempt and stay pending for a later pass

#### Scenario: One channel succeeding delivers the group

- **WHEN** a group sends successfully over email but errors over Telegram
- **THEN** every item in the group is marked delivered and the Telegram error is logged

#### Scenario: No usable channel is a soft skip

- **WHEN** a group's account has no usable destination on any configured channel
- **THEN** the group is released unsent, no delivery attempt is burned, and the soft
  skip is counted in the pass summary
