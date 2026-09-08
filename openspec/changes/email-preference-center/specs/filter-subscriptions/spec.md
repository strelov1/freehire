## MODIFIED Requirements

### Requirement: Subscribe a saved search to notifications

The system SHALL let an authenticated user subscribe one of their saved searches
to a delivery channel, so that matching jobs are pushed to them. A subscription
references a saved search (the filter of record) and a channel; the channel SHALL
be one of the supported channels (`telegram`, `email`, or `webhook`); at most one
subscription MAY exist per (saved search, channel), so a user MAY subscribe the
same saved search on Telegram, email, and their webhook at once. Creating a
subscription, and changing one from within the account, SHALL require the session
cookie (`RequireAuth`), never an API key. Deactivating an email subscription MAY
additionally be done without a session through a signed unsubscribe link, which
SHALL only ever deactivate and never create or activate one.

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

#### Scenario: Deactivate one subscription through a signed link

- **WHEN** a recipient turns off one saved-search subscription on the public preference page opened by a valid unsubscribe token
- **THEN** that subscription is deactivated and produces no further digests
- **AND** the account's other subscriptions are unchanged

#### Scenario: A signed link cannot create or reactivate a subscription

- **WHEN** a request arriving with an unsubscribe token names a saved search the account has no subscription for
- **THEN** no subscription is created, and the request neither errors in a way that reveals the account's saved searches nor changes any other subscription

## ADDED Requirements

### Requirement: A master switch above the per-subscription switches

The system SHALL provide one account-level switch governing all saved-search digests
delivered by email, sitting above the per-subscription switches. When it is off, no
email digest SHALL be delivered for that account regardless of any individual
subscription's own state, and the individual states SHALL be preserved rather than
cleared, so turning the master switch back on restores exactly what was subscribed
before.

#### Scenario: The master switch silences every email digest

- **WHEN** an account with several active email subscriptions turns the alerts
  master switch off
- **THEN** no email digest is delivered for any of them

#### Scenario: Individual choices survive the master switch

- **WHEN** that account turns the alerts master switch back on
- **THEN** exactly the subscriptions that were active before are active again, and
  the ones that were individually off remain off

#### Scenario: The master switch does not affect other channels

- **WHEN** the alerts master switch is off and the account has a Telegram
  subscription to the same saved search
- **THEN** the Telegram digest is still delivered
