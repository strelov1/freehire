## Why

The measured daily window contained about 21 million updates to `jobs`, including content changes and routine observations. Those observations currently rewrite the wide posting row. Moving their mutable state into a compact relation should reduce heap churn without weakening lifecycle checks or changing public responses. The observation share and resulting WAL savings still need measurement.

## What Changes

- Store `last_seen_at`, `hydrated_at`, and `liveness_strikes` in one `job_activity` row per posting.
- Read complete postings through an activity-aware projection while keeping the public Go and JSON job shapes.
- Make ordinary observations update and lock only the activity row; retain atomic content, lifecycle and queue transitions.
- Deploy an additive reader release, an activity-writer release after old readers drain, and a final legacy-column cleanup release.
- Measure main-row tuple stability, per-observation WAL and API/read costs; claim savings only after measurement.

## Capabilities

### New Capabilities

- `job-activity`: Compact observation persistence, concurrency guarantees and compatible schema transition.

### Modified Capabilities

None. Existing lifecycle, visibility, hydration and public wire contracts are preserved.

## Impact

PostgreSQL migrations and backfill; sqlc models and queries; ingest persistence; liveness and sweeps; job projections; integration fixtures and guards; deployment verification. Counters, analytics, content enrichment and source scheduling are outside this change.
