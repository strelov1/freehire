## Why

Candidates need one daily view of new recruiter mail, requests and application updates without repeatedly checking the inbox. Existing Gmail/hosted ingestion, classification and outbound delivery provide the inputs and transport.

## What Changes

- Add a Pro/Ultra opt-in daily inbox email to notification settings, using the account timezone and chosen daily digest time.
- Group new classified incoming mail by known provider thread, show recent outgoing evidence, and skip empty days.
- Deliver inside the existing notify worker with a per-local-day claim and delivery ledger, independent of search availability.

## Capabilities

### New Capabilities

- `inbox-daily-digest`: Consent, selection, rendering and delivery of daily hiring-mail summaries.

### Modified Capabilities

None.

## Impact

Notification settings contract/API/UI, an additive migration, inbox-digest use case and SQL, notify worker and documentation. No new worker, model calls, CV matching, reply sending or production deployment.
