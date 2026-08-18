## MODIFIED Requirements

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
