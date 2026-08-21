## MODIFIED Requirements

### Requirement: Digest delivery with retry and dead-letter

The system SHALL deliver all of a subscription's newly matched jobs from one
worker pass as a single digest message. Delivery SHALL be claimed safely under
concurrency so overlapping worker runs cannot send the same digest twice. A
failed delivery SHALL be retried on a later pass and dead-lettered after a bounded
number of attempts; a successful delivery SHALL mark its matches as notified so
they are not sent again. When the account's saved-search digest frequency is
`daily`, delivery SHALL additionally wait until the account's configured local
delivery time before claimed matches are sent, at most once per local calendar
day; this timing gate does not affect the `instant` frequency (the default),
which delivers as soon as matches are claimed as before.

How many jobs a digest **records** SHALL be bounded separately from how many a
channel message **lists**. The digest SHALL carry the whole match set, bounded only
by a configured snapshot ceiling that exists to keep one notification's recorded
document from growing without limit; the per-message listing bound belongs to the
channel and SHALL NOT truncate what is recorded. The digest's match count SHALL
remain the true count of matched jobs under either bound.

The in-app notification SHALL be recorded before the digest is sent, so the digest
can carry its own notification id and each channel can link to the page that renders
the recorded match set. A delivery that then fails SHALL have that record removed, so
the notification history holds a row for a digest if and only if the digest was
delivered. A failed recording SHALL NOT block or fail the delivery: the digest is
sent carrying no notification id, and the channel falls back to a generic
destination.

#### Scenario: One digest per subscription per pass

- **WHEN** a subscription has several pending matches in a pass
- **THEN** they are delivered as one digest message and all included matches are marked notified

#### Scenario: Failed delivery is retried, not lost

- **WHEN** a delivery attempt fails
- **THEN** the matches stay pending (not marked notified), the attempt count increases, and a later pass retries them until the attempt limit, after which they are dead-lettered

#### Scenario: Overlapping passes do not double-send

- **WHEN** two worker passes run concurrently
- **THEN** pending matches are claimed exclusively (skip-locked) so a digest is sent at most once

#### Scenario: Daily-frequency digest waits for its delivery time

- **WHEN** a subscription's account has `daily` digest frequency configured
  and pending matches are claimed before the account's configured local
  delivery time
- **THEN** the claim is released without being marked notified or counted as
  a failed attempt, and delivery is retried on a later pass

#### Scenario: Daily-frequency digest delivers once per local day

- **WHEN** a `daily`-frequency subscription's local delivery time has passed
  and no digest has been sent for the current local calendar day
- **THEN** the pending matches are delivered as one digest and the
  subscription's last-sent time is stamped

#### Scenario: Instant-frequency delivery deferred during quiet hours

- **WHEN** an `instant`-frequency subscription's matches are claimed while
  the account's local time is inside its configured quiet-hours window
- **THEN** the claim is released without being marked notified or counted as
  a failed attempt, and delivery is retried on a later pass

#### Scenario: A digest records more jobs than its message lists

- **WHEN** a pass matches 67 jobs for one subscription
- **THEN** the recorded in-app notification carries all 67, while the channel message lists only the channel's bound

#### Scenario: The delivered digest carries its notification id

- **WHEN** a digest's in-app notification is recorded and the send then succeeds
- **THEN** the digest handed to the channel carried that notification's id, and the record remains

#### Scenario: A failed delivery leaves no notification history row

- **WHEN** a digest's in-app notification is recorded and the send then fails
- **THEN** the recorded row is removed, the matches stay pending for a retry, and the failure is counted as an attempt

#### Scenario: A failed recording does not block delivery

- **WHEN** recording the in-app notification fails
- **THEN** the digest is still sent, carrying no notification id, and its matches are marked notified as normal
