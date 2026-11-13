## Decision

Keep `jobs` as the content and lifecycle row. Move observation timestamps and strikes into a one-to-one `job_activity` row. A read-only `jobs_read` view explicitly projects the posting with activity fields. Its `LEFT JOIN LATERAL` uses a primary-key lookup with `LIMIT 1`: without that barrier, a merge join can scan every earlier activity ID for each keyset page. A regression test counts scanned rows, including filtered tuples, at several cursors. sqlc names the view model `Job` and the base model `JobRow`, preserving public/domain shapes without per-caller activity loads.

Candidate B is the base. The independent review adds a content/lifecycle fence, narrow writer returns, a transaction retry on a rejected cheap write, and a backfill that adds the foreign key only after copying. Candidate A contributes the complete reader inventory and the warning about generated column expansion.

## Compatible releases

Release A adds the table, insert invariant, temporary one-way legacy update bridge, state fence, bounded backfill, foreign key and view. Every observation reader and closer moves to activity. Ordinary writers continue updating legacy columns; the bridge mirrors them in the same transaction. Probe increments and resets explicitly update both rows, in jobs-then-activity lock order, because a computed strike can equal a frozen legacy value and therefore not fire the changed-value bridge. Their data-modifying CTEs have explicit return dependencies. This release is additive and temporarily increases writes.

Before B, inventory executable build revisions on both core and operator and wait until every pre-A reader exits. A workers with the compatible lock ordering below may overlap B: their reads use activity and their old writes still mirror. Do not infer this gate from the API revision alone.

The compatible lock-order follow-up must also precede B. The initial A bridge locks activity
at statement end, after its touch/cheap-refresh outbox CTEs. That order deadlocks with B's
activity-first observations. Both legacy statements now lock jobs then activity before
queueing. Drain ingest binaries from before this follow-up before enabling B writers;
current observation reads alone do not establish compatibility. The transition does not
pause the crawler fleet or accept known deadlock failures.
The gate also requires the validated backfill and insert invariant: a missing activity row
is a broken invariant, and the full touch now fails instead of silently observing it.

Release B moves all observation writers to activity, with explicit returns that never expand the base row's legacy columns. New inserts preserve body hydration semantics. Before C, drain every process naming legacy columns and inspect private operations code, host scripts and dashboards.

Release C drops the legacy bridge before dropping the three columns. Its migration runs before the code flip, so B binaries must already be compatible with the contracted schema. No shadow schema directory or bidirectional bridge is introduced. Rollback to a legacy reader requires resynchronizing its columns from activity first.

## Schema and backfill

The activity primary key is `job_id`; no index covers frequently changing observation fields. A permanent insert trigger ensures raw SQL inserts, fixtures and all intake paths create activity. The initial trigger copies explicit legacy values, including a nullable hydration stamp.

Install the insert and legacy-update triggers before copying. Copy bounded id ranges with separate commits and `ON CONFLICT DO NOTHING`, preserving concurrent trigger writes. The timestamp bridge uses `GREATEST`; strikes mirror only when the legacy strike value actually changes. Fixtures that deliberately backdate after insertion must update activity explicitly.

The initial copy has no foreign key: checking it on every insert would lock and dirty the wide parent tuples. After the copy, assert no missing activity, add the foreign key `NOT VALID`, remove possible orphans, then validate. A no-transaction migration's top-level transaction-controlling DO block must be proven through the real migration runner on PostgreSQL 18 before use. If that proof fails, use an explicit resumable operational backfill rather than a giant transaction.

## Concurrency

Lock order is jobs, then activity. Closers lock both with `FOR NO KEY UPDATE OF j, a` and recheck activity freshness against the latest locked version. `FOR UPDATE` would conflict with foreign-key KEY SHARE locks from downstream queue/form inserts and create a deadlock with a cheap writer holding activity.

A single activity `state_version` changes when lifecycle or the complete cheap-match key changes. The central jobs trigger covers closed_at, content_hash, cities, salary_min_source, salary_max_source, salary_currency_source, salary_period_source, english_level, is_tech, engagement and application_deadline. Snapshot-returned duplicate/privacy state also needs fencing where used. Reopening resets strikes in this trigger.

The cheap update targets activity and compares its current version to the match snapshot. PostgreSQL rechecks that target tuple after waiting. A mismatch returns a distinct fenced result. The caller rolls back before retrying the cheap path from a fresh snapshot, avoiding an activity-to-jobs lock reversal. A changed match key then falls through to full without an activity lock; a metadata-only change returns the current stored company slug. Bound repeated fences and surface failure rather than silently dropping an observation or forcing a stale metadata overwrite.

The version also covers returned company and publication metadata (`company_slug`, `posted_at`, `created_at`). In particular, `RekeyCompanySlugChunk` changes the stored slug without changing its content hash. A cheap retry must observe that slug rather than force a full upsert using the earlier input.

Ordinary observations must leave jobs xmin, xmax and ctid unchanged for the whole already-enriched ingestion transaction. Necessary downstream foreign-key inserts for unenriched jobs are an explicit exception. Full content writes and lifecycle transitions may update jobs.

## Verification and measurement

Regression tests cover insertion, bridge deltas, backfill restart/races, view completeness, per-column state fencing, content and lifecycle races, no parent tuple mutation, and deadlock ordering on PostgreSQL 18. Guards prevent full base-table returns from expanding columns removed by C. Existing public/private population checks cover the read view.

Check representative plans for board identity scans, time crossings, company facets and stale closures. Measure WAL/FPI and tuple updates before and after writer cutover; do not claim an I/O saving from tuple-size estimates. Dropping columns does not immediately reclaim disk. Full job content and all existing API/search timestamps retain their semantics.

## Read-cost evidence and runtime gate

On a cached 1.2-million-row synthetic catalogue, a midpoint keyset page read 12,679 logical buffers with the plain join, versus 258 on the base table. The parameterized view reduced it to 4,258 and bounded activity work by the batch size at every cursor. This fixes repeated prefix scans, but adds per-row activity lookups.

The same synthetic runs added about 9–13% logical buffers to several aggregate reads, about five times the board seen-set buffers, and up to 7.5 times the whole-provider stale-candidate buffers. These are read costs, not elapsed-time estimates: the host was heavily loaded and all data was cached. Inspect real plans and normal worker/API execution after additive migrations, preferably on the physical replica, before the writer cutover. Do not evict production caches or launch duplicate heavy workers to manufacture a comparison.

## Foreign-key publication order

Legacy observations acquire `jobs` before the bridge writes `job_activity`. Adding a foreign key directly takes the child table lock first, which can deadlock with those writers. Migration `0259_job_activity_foreign_key_lock_order.sql` sorts after the completed copy and before immutable `0260`; it takes the parent lock before adding the not-valid constraint. `0260` retains its completeness check and skips an existing constraint; `0261` performs orphan cleanup and validation. Completeness remains a gate before validation and binary cutover, even if the not-valid constraint was already created. Keep the parent lock inside the missing-constraint branch so environments that already have the FK do not block writers unnecessarily.

The copied data is independently recorded before FK publication. A failed FK attempt therefore retries the remaining migrations without repeating the copy. The parent DDL lock still obeys the runner's five-second lock timeout; check conflicting maintenance before the normal release.
