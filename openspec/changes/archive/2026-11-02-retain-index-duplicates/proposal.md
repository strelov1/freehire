## Why

Duplicate transitions still physically delete Meilisearch documents after ordinary closures moved to status updates. Link import additionally submits a redundant deletion outside the durable queue. Preserve existing documents and vectors while hidden and use one state-checked queue path.

## What Changes

- Mark duplicate documents with internal `is_duplicate`, using existing-document-only mutations.
- Exclude duplicate documents at the shared search/count/sitemap boundary; full replacements clear stale status.
- Classify queued removals as closure, duplicate, or physical removal from current database state.
- Remove the redundant direct link-import deletion; its atomic SQL outbox remains authoritative.
- Keep full rebuilds compact: omit closed/duplicate documents and discard obsolete indexes. Private and missing rows still physically leave the index.

## Capabilities

### Modified Capabilities

- `job-search`: duplicate visibility and retained-document lifecycle.

## Impact

Search client, drain adapter, removal query/sqlc, link import, tests, and rollout documentation. No PostgreSQL migration or public API change.
