## Why

A job's detail page shows no signal of how much interest a posting has drawn. Surfacing how many people have viewed a job and how many have applied gives visitors lightweight social proof and helps them gauge competition — using data the platform already records in `user_jobs`, at effectively zero read cost.

## What Changes

- Materialize two counters on the `jobs` row — `view_count` and `applied_count` — so every read path (list/detail/search) already carries them with no extra query or join.
- Increment the counters in the existing tracking write path, each only on a first-time transition, so refreshes and repeat actions never inflate them:
  - `view_count` +1 when a signed-in user's first view row is created (first time they open the detail page).
  - `applied_count` +1 when a user's `applied_at` goes from unset to set.
- Backfill both counters from existing `user_jobs` rows in the same migration, so live jobs are not all zero on release.
- Expose both counters on the public job wire shape (`jobview.Job`) and display them on the job **detail** page only (`JobView.svelte`).

Semantics (accepted, signed-in-only): counts reflect **distinct signed-in users**. Anonymous opens are not counted — noted as a future seam, not built now.

## Capabilities

### New Capabilities
- `job-engagement-counts`: a job carries and publicly exposes materialized `view_count`/`applied_count`, displayed on the job detail page.

### Modified Capabilities
- `user-job-tracking`: recording a view and marking applied now also increment the job's materialized counter, each on a first-time transition only.

## Impact

- **Schema**: new migration — `jobs.view_count`/`jobs.applied_count` (`INT NOT NULL DEFAULT 0`) + one-pass backfill.
- **DB queries** (`internal/db/queries/user_jobs.sql`): `RecordJobView` and `MarkJobApplied` become single-statement CTEs that upsert the interaction and conditionally bump the job counter; `make sqlc` regen.
- **Wire shape** (`internal/jobview/jobview.go`): two new fields on `Job`, populated by `FromRow`.
- **Frontend** (`web/src/lib/components/JobView.svelte`, `web/src/lib/types.ts`): render the counts on the detail page.
- No new endpoints, no read-time counting, no write-path added for anonymous traffic.
