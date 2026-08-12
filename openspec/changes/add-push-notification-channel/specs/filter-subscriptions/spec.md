## MODIFIED Requirements

### Requirement: Subscribe a saved search to notifications

The system SHALL let an authenticated user subscribe one of their saved searches
to a delivery channel, so that matching jobs are pushed to them. A subscription
references a saved search (the filter of record) and a channel; the channel SHALL
be one of the supported channels (`telegram`, `email`, or `push`). The `push`
channel stores no per-subscription destination — like `email`, the recipient is
resolved live at delivery time from the user's registered devices. At most one
subscription MAY exist per (saved search, channel), so a user MAY subscribe the
same saved search on Telegram, email, and push at once. Subscription management
SHALL require the session cookie (`RequireAuth`), never an API key.

#### Scenario: Create a subscription

- **WHEN** an authenticated user POSTs `{saved_search_id, channel:"telegram"}` for a saved search they own
- **THEN** the system creates a subscription with `active=true` and `start_at=now()`, and returns it as `{"data": subscription}`

#### Scenario: Create an email subscription

- **WHEN** an authenticated user POSTs `{saved_search_id, channel:"email"}` for a saved search they own
- **THEN** the system creates an email subscription with `active=true`, no per-subscription destination stored, and returns it as `{"data": subscription}`

#### Scenario: Create a push subscription

- **WHEN** an authenticated user POSTs `{saved_search_id, channel:"push"}` for a saved search they own
- **THEN** the system creates a push subscription with `active=true`, no per-subscription destination stored, and returns it as `{"data": subscription}`, regardless of whether the user currently has a registered device

#### Scenario: Unsupported channel is rejected

- **WHEN** a user POSTs a subscription with a channel that is not `telegram`, `email`, or `push`
- **THEN** the system returns a 400 and creates no subscription

#### Scenario: Duplicate subscription is rejected

- **WHEN** a user creates a second subscription for the same saved search and channel
- **THEN** the system returns a 409 (or idempotently returns the existing subscription) and does not create a duplicate row

#### Scenario: Cannot subscribe to another user's saved search

- **WHEN** a user references a `saved_search_id` they do not own
- **THEN** the system returns a 404 and creates no subscription

#### Scenario: Toggle and unsubscribe

- **WHEN** the user PATCHes a subscription's `active` flag or DELETEs it
- **THEN** the subscription is deactivated/removed and no further notifications are produced for it

### Requirement: Pluggable delivery channel

The system SHALL deliver through a narrow `Notifier` abstraction selected by the
subscription's channel, dispatched by a channel router so additional channels can
be added without changing the matching engine. The `telegram` channel SHALL
resolve the recipient from the user's linked Telegram chat. The `email` channel
SHALL resolve the recipient from the user's account email, read live at delivery
time, so that no per-subscription address is stored and a changed account email
takes effect on the next delivery. The `push` channel SHALL resolve the recipient
as the subscribing user, read live at delivery time, and deliver to every device
currently registered for that user (a user MAY have more than one); a user with no
currently registered device SHALL be treated the same as an unlinked Telegram
chat. A subscription whose channel has no configured notifier SHALL be softly
skipped (its matches stay pending, no attempt counted).

#### Scenario: Telegram delivery without a stored destination

- **WHEN** a `telegram` subscription is delivered
- **THEN** the worker resolves the recipient `chat_id` from the user's Telegram link rather than from a per-subscription destination

#### Scenario: Unlinked Telegram is skipped, not failed

- **WHEN** a `telegram` subscription's user has no linked Telegram chat
- **THEN** the delivery is softly skipped (matches stay pending, no attempt is counted) rather than dead-lettered

#### Scenario: Email delivery resolves the account email

- **WHEN** an `email` subscription is delivered
- **THEN** the worker resolves the recipient from the user's current account email and routes the digest to the email notifier

#### Scenario: Push delivery fans out to every registered device

- **WHEN** a `push` subscription is delivered and the user has two registered devices
- **THEN** the worker sends the digest to both devices, and the delivery counts as sent as long as at least one device receives it

#### Scenario: No registered device is skipped, not failed

- **WHEN** a `push` subscription's user currently has no registered device
- **THEN** the delivery is softly skipped (matches stay pending, no attempt is counted) rather than dead-lettered

#### Scenario: Router dispatches by channel

- **WHEN** a digest is delivered for a subscription
- **THEN** the router sends it through the notifier registered for that subscription's channel, and a channel with no registered notifier is softly skipped

## ADDED Requirements

### Requirement: Push digest content and deep link

A `push` digest SHALL render as a short title and body — the saved search's name
and the count of newly matched jobs — rather than the itemized listing the
`telegram`/`email` channels send. When the digest's match count is exactly one,
the push SHALL carry that job's slug as deep-link data so the mobile app can open
it directly; when the count is more than one, the push SHALL carry no deep-link
data.

#### Scenario: Digest content is name and count

- **WHEN** a push digest is rendered for a saved search named "Backend Engineer" with 3 newly matched jobs
- **THEN** the push body reads to the effect of "3 new jobs for \"Backend Engineer\""

#### Scenario: Single match carries a deep link

- **WHEN** a push digest's match count is exactly 1
- **THEN** the push's data payload includes that job's slug

#### Scenario: Multiple matches carry no deep link

- **WHEN** a push digest's match count is greater than 1
- **THEN** the push's data payload includes no job slug
