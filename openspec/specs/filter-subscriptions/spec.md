# filter-subscriptions Specification

## Purpose
TBD - created by syncing change filter-subscriptions. Update Purpose after archive.
## Requirements
### Requirement: Subscribe a saved search to notifications

The system SHALL let an authenticated user subscribe one of their saved searches
to a delivery channel, so that matching jobs are pushed to them. A subscription
references a saved search (the filter of record) and a channel; the channel SHALL
be one of the supported channels (`telegram` or `email`); at most one
subscription MAY exist per (saved search, channel), so a user MAY subscribe the
same saved search on both Telegram and email. Subscription management SHALL
require the session cookie (`RequireAuth`), never an API key.

#### Scenario: Create a subscription

- **WHEN** an authenticated user POSTs `{saved_search_id, channel:"telegram"}` for a saved search they own
- **THEN** the system creates a subscription with `active=true` and `start_at=now()`, and returns it as `{"data": subscription}`

#### Scenario: Create an email subscription

- **WHEN** an authenticated user POSTs `{saved_search_id, channel:"email"}` for a saved search they own
- **THEN** the system creates an email subscription with `active=true`, no per-subscription destination stored, and returns it as `{"data": subscription}`

#### Scenario: Unsupported channel is rejected

- **WHEN** a user POSTs a subscription with a channel that is not `telegram` or `email`
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

### Requirement: Windowed filter matching

The system SHALL match jobs against subscriptions with a pull/windowed worker
whose per-pass cost is proportional to the number of *distinct* filter queries,
not to the number of jobs or subscribers. The worker SHALL group active
subscriptions by their canonical query, run each distinct query once against the
search index sorted by recency with a bounded limit, and record matches. It MUST
NOT use a freshness signal that re-crawls bump (e.g. `updated_at`); recency is
measured by job creation time.

A job that matches a subscription's query SHALL additionally be excluded from
that specific subscription's matches when it carries any skill in the
subscriber's current `excluded_skills` (avoid-skills) preference, evaluated at
match time against the account's *live* preference — not a value frozen into
the saved search's query string at creation time. This exclusion is evaluated
per (job, subscriber) pair and MUST NOT require an additional search per
subscriber; it does not affect subscribers on the same shared query who have no
overlapping avoid-skills.

#### Scenario: Distinct filters queried once

- **WHEN** N subscriptions share one canonical query
- **THEN** the worker issues a single search for that query and fans the results to all N subscriptions

#### Scenario: Only jobs at or after the subscription cutoff

- **WHEN** the worker finds a matching job whose creation time is before a subscription's `start_at`
- **THEN** that job is not recorded as a match for that subscription

#### Scenario: A job that becomes matchable after enrichment is still caught

- **WHEN** a job did not match a filter at ingest but matches after enrichment fills a facet, and it is still within the recency window
- **THEN** a later pass records it as a match (the worker re-scans recent jobs, it does not only look at jobs newer than a cursor)

#### Scenario: A job carrying an avoided skill is not matched for that subscriber

- **WHEN** a job otherwise matches a subscription's query, and the job's skills include a skill in that subscription's subscriber's current `excluded_skills`
- **THEN** the job is not recorded as a match for that subscription

#### Scenario: Avoid-skills exclusion is per-subscriber, not per-query

- **WHEN** two subscriptions share one canonical query, and only one subscriber has the matching job's skill in their `excluded_skills`
- **THEN** the job is recorded as a match for the subscriber without that skill in their avoid list, and not recorded for the other

#### Scenario: Updating the avoid list affects the next matching pass without recreating the subscription

- **WHEN** a subscriber adds a skill to `excluded_skills` after their subscription was created (with or without that skill previously excluded via the saved search's own query)
- **THEN** the next matching pass stops recording new matches for jobs carrying that skill, without requiring the subscription or its saved search to be recreated or edited

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
they are not sent again. When the account's saved-search digest frequency is
`daily`, delivery SHALL additionally wait until the account's configured local
delivery time before claimed matches are sent, at most once per local calendar
day; this timing gate does not affect the `instant` frequency (the default),
which delivers as soon as matches are claimed as before.

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

### Requirement: Pluggable delivery channel

The system SHALL deliver through a narrow `Notifier` abstraction selected by the
subscription's channel, dispatched by a channel router so additional channels can
be added without changing the matching engine. The `telegram` channel SHALL
resolve the recipient from the user's linked Telegram chat. The `email` channel
SHALL resolve the recipient from the user's account email, read live at delivery
time, so that no per-subscription address is stored and a changed account email
takes effect on the next delivery. A subscription whose channel has no configured
notifier SHALL be softly skipped (its matches stay pending, no attempt counted).

#### Scenario: Telegram delivery without a stored destination

- **WHEN** a `telegram` subscription is delivered
- **THEN** the worker resolves the recipient `chat_id` from the user's Telegram link rather than from a per-subscription destination

#### Scenario: Unlinked Telegram is skipped, not failed

- **WHEN** a `telegram` subscription's user has no linked Telegram chat
- **THEN** the delivery is softly skipped (matches stay pending, no attempt is counted) rather than dead-lettered

#### Scenario: Email delivery resolves the account email

- **WHEN** an `email` subscription is delivered
- **THEN** the worker resolves the recipient from the user's current account email and routes the digest to the email notifier

#### Scenario: Router dispatches by channel

- **WHEN** a digest is delivered for a subscription
- **THEN** the router sends it through the notifier registered for that subscription's channel, and a channel with no registered notifier is softly skipped

