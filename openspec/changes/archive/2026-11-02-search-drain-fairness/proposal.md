## Why

Closed postings remain searchable outside rebuild windows because one indexing pass can spend minutes sending small waves before deletions run again. Pending deletions also outlive a reopening, and hydrating-source touches do not requeue the reopened posting.

## What Changes

- Replace physical deletion for ordinary closures with an in-place `is_closed` marker; keep physical deletion for missing/private/noncanonical jobs.
- Exclude marked documents inside keyword, vector, federated, facet-count, distinct-count, and sitemap queries.
- Alternate bounded deletion and indexing waves while preserving separate per-queue run budgets.
- End a queue pass after a short wave so incoming trickles accumulate for the next scheduled run.
- Delete only currently closed or missing jobs, and requeue reopened jobs when acknowledging their old removal.
- Atomically enqueue a hydrating-source job when a touch reopens it.

## Capabilities

### New Capabilities
- `search-drain-fairness`: Cooperative queue draining without starvation or tiny-wave loops.

### Modified Capabilities
- `job-search`: Closed documents stop matching through status updates while text and vectors remain until rebuild.
- `job-lifecycle`: Reopened postings return to search even when an earlier removal is pending or in flight.

## Impact

Changes `cmd/search-drain`, `internal/search/searchdrain`, and SQL queries under `internal/platform/db`. No migration, public API change, or new infrastructure. Rollout requires the `is_closed` filterable setting and Meilisearch document-editing feature before the reader/worker binary. Rebuild-time scheduling remains unchanged until its contention experiment is complete.
