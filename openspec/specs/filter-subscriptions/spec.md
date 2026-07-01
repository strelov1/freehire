# filter-subscriptions Specification

## Purpose
TBD - created by syncing change filter-subscriptions. Update Purpose after archive.
## Requirements
### Requirement: Subscribe a saved search to notifications

The system SHALL let an authenticated user subscribe one of their saved searches
to a delivery channel, so that matching jobs are pushed to them. A subscription
references a saved search (the filter of record) and a channel; at most one
subscription MAY exist per (saved search, channel). Subscription management SHALL
require the session cookie (`RequireAuth`), never an API key.

#### Scenario: Create a subscription

- **WHEN** an authenticated user POSTs `{saved_search_id, channel:"telegram"}` for a saved search they own
- **THEN** the system creates a subscription with `active=true` and `start_at=now()`, and returns it as `{"data": subscription}`

#### Scenario: Duplicate subscription is rejected

- **WHEN** a user creates a second subscription for the same saved search and channel
- **THEN** the system returns a 409 (or idempotently returns the existing subscription) and does not create a duplicate row

#### Scenario: Cannot subscribe to another user's saved search

- **WHEN** a user references a `saved_search_id` they do not own
- **THEN** the system returns a 404 and creates no subscription

#### Scenario: Toggle and unsubscribe

- **WHEN** the user PATCHes a subscription's `active` flag or DELETEs it
- **THEN** the subscription is deactivated/removed and no further notifications are produced for it

### Requirement: Windowed filter matching

The system SHALL match jobs against subscriptions with a pull/windowed worker
whose per-pass cost is proportional to the number of *distinct* filter queries,
not to the number of jobs or subscribers. The worker SHALL group active
subscriptions by their canonical query, run each distinct query once against the
search index sorted by recency with a bounded limit, and record matches. It MUST
NOT use a freshness signal that re-crawls bump (e.g. `updated_at`); recency is
measured by job creation time.

#### Scenario: Distinct filters queried once

- **WHEN** N subscriptions share one canonical query
- **THEN** the worker issues a single search for that query and fans the results to all N subscriptions

#### Scenario: Only jobs at or after the subscription cutoff

- **WHEN** the worker finds a matching job whose creation time is before a subscription's `start_at`
- **THEN** that job is not recorded as a match for that subscription

#### Scenario: A job that becomes matchable after enrichment is still caught

- **WHEN** a job did not match a filter at ingest but matches after enrichment fills a facet, and it is still within the recency window
- **THEN** a later pass records it as a match (the worker re-scans recent jobs, it does not only look at jobs newer than a cursor)

### Requirement: Match dedup ledger

The system SHALL guarantee that a job is delivered to a subscription at most once,
independent of how many times the worker re-scans it. A `(subscription, job)`
ledger with a uniqueness constraint SHALL be the source of truth for "already
matched"; recording a match SHALL be idempotent.

#### Scenario: Re-scanning an already-recorded match is a no-op

- **WHEN** the worker re-scans a job already present in the ledger for a subscription
- **THEN** the insert is ignored and no duplicate match or notification is produced

### Requirement: Digest delivery with retry and dead-letter

The system SHALL deliver all of a subscription's newly matched jobs from one
worker pass as a single digest message. Delivery SHALL be claimed safely under
concurrency so overlapping worker runs cannot send the same digest twice. A
failed delivery SHALL be retried on a later pass and dead-lettered after a bounded
number of attempts; a successful delivery SHALL mark its matches as notified so
they are not sent again.

#### Scenario: One digest per subscription per pass

- **WHEN** a subscription has several pending matches in a pass
- **THEN** they are delivered as one digest message and all included matches are marked notified

#### Scenario: Failed delivery is retried, not lost

- **WHEN** a delivery attempt fails
- **THEN** the matches stay pending (not marked notified), the attempt count increases, and a later pass retries them until the attempt limit, after which they are dead-lettered

#### Scenario: Overlapping passes do not double-send

- **WHEN** two worker passes run concurrently
- **THEN** pending matches are claimed exclusively (skip-locked) so a digest is sent at most once

### Requirement: Pluggable delivery channel

The system SHALL deliver through a narrow `Notifier` abstraction selected by the
subscription's channel, so additional channels (webhook, email) can be added
without changing the matching engine. The `telegram` channel SHALL resolve the
recipient from the user's linked Telegram chat.

#### Scenario: Telegram delivery without a stored destination

- **WHEN** a `telegram` subscription is delivered
- **THEN** the worker resolves the recipient `chat_id` from the user's Telegram link rather than from a per-subscription destination

#### Scenario: Unlinked Telegram is skipped, not failed

- **WHEN** a `telegram` subscription's user has no linked Telegram chat
- **THEN** the delivery is softly skipped (matches stay pending, no attempt is counted) rather than dead-lettered
