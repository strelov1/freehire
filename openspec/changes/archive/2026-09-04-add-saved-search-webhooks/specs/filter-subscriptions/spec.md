## MODIFIED Requirements

### Requirement: Subscribe a saved search to notifications

The system SHALL let an authenticated user subscribe one of their saved searches
to a delivery channel, so that matching jobs are pushed to them. A subscription
references a saved search (the filter of record) and a channel; the channel SHALL
be one of the supported channels (`telegram`, `email`, or `webhook`); at most one
subscription MAY exist per (saved search, channel), so a user MAY subscribe the
same saved search on Telegram, email, and their webhook at once. Subscription
management SHALL require the session cookie (`RequireAuth`), never an API key.

#### Scenario: Create a subscription

- **WHEN** an authenticated user POSTs `{saved_search_id, channel:"telegram"}` for a saved search they own
- **THEN** the system creates a subscription with `active=true` and `start_at=now()`, and returns it as `{"data": subscription}`

#### Scenario: Create an email subscription

- **WHEN** an authenticated user POSTs `{saved_search_id, channel:"email"}` for a saved search they own
- **THEN** the system creates an email subscription with `active=true`, no per-subscription destination stored, and returns it as `{"data": subscription}`

#### Scenario: Create a webhook subscription

- **WHEN** an authenticated user POSTs `{saved_search_id, channel:"webhook"}` for a saved search they own
- **THEN** the system creates a webhook subscription with `active=true`, no per-subscription destination stored, and returns it as `{"data": subscription}` — regardless of whether the account has configured a webhook destination yet

#### Scenario: Unsupported channel is rejected

- **WHEN** a user POSTs a subscription with a channel that is not `telegram`, `email`, or `webhook`
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
takes effect on the next delivery. The `webhook` channel SHALL resolve the
recipient from the account's configured webhook destination, read live at
delivery time, so that a rotated secret, a changed URL, or a disabled
destination takes effect on the next delivery without editing the subscription.
A subscription whose channel has no configured notifier, or whose account has no
enabled destination for that channel, SHALL be softly skipped (its matches stay
pending, no attempt counted).

#### Scenario: Telegram delivery without a stored destination

- **WHEN** a `telegram` subscription is delivered
- **THEN** the worker resolves the recipient `chat_id` from the user's Telegram link rather than from a per-subscription destination

#### Scenario: Unlinked Telegram is skipped, not failed

- **WHEN** a `telegram` subscription's user has no linked Telegram chat
- **THEN** the delivery is softly skipped (matches stay pending, no attempt is counted) rather than dead-lettered

#### Scenario: Email delivery resolves the account email

- **WHEN** an `email` subscription is delivered
- **THEN** the worker resolves the recipient from the user's current account email and routes the digest to the email notifier

#### Scenario: Webhook delivery resolves the account's destination

- **WHEN** a `webhook` subscription is delivered
- **THEN** the worker resolves the URL and current secret from the account's webhook destination rather than from a per-subscription value

#### Scenario: No configured or disabled webhook destination is skipped, not failed

- **WHEN** a `webhook` subscription's account has never configured a webhook destination, or has disabled it
- **THEN** the delivery is softly skipped (matches stay pending, no attempt is counted) rather than dead-lettered

#### Scenario: Router dispatches by channel

- **WHEN** a digest is delivered for a subscription
- **THEN** the router sends it through the notifier registered for that subscription's channel, and a channel with no registered notifier is softly skipped
