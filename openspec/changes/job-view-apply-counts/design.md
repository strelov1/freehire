## Context

`user_jobs (user_id, job_id, viewed_at, applied_at, …)` already records every
signed-in user's interaction with a job — one row per pair, `viewed_at` always
set. Views are recorded only for authenticated users (`RecordJobView`), and
`applied_at` is set by `MarkJobApplied`. Nothing today surfaces aggregate
interest on a posting. The job wire shape (`jobview.Job`, built by
`FromRow(db.Job)`) is shared by the list, detail, search-index, and company
surfaces.

## Goals / Non-Goals

**Goals:**
- Show "N views · M applied" on the job **detail** page.
- Keep it lightweight: zero read-time counting; reads serve stored values.
- Never inflate counters on refresh or repeat actions.
- Backfill existing jobs so the feature isn't all-zeros on release.

**Non-Goals:**
- Counting anonymous opens (kept as a future seam — would need an anonymous
  write path plus bot/dedup defense).
- Displaying the counts on job cards, search results, or company pages.
- Total-view-event semantics (we count distinct users, not raw opens).

## Decisions

### Materialized counters on `jobs`, not read-time `COUNT`

Add `jobs.view_count` and `jobs.applied_count` (`INT NOT NULL DEFAULT 0`).
`FromRow` reads them straight off the row, so all read paths carry them for free
and the detail page needs no extra query.

*Alternative — read-time `COUNT(*) FROM user_jobs WHERE job_id = $1`:* rejected.
It would need a new `user_jobs(job_id)` index (the PK is `(user_id, job_id)`, no
help for a job-keyed scan) and a bespoke detail-only response shape or a second
query, while the counter approach reuses the existing shared shape and is O(1) at
read. The user explicitly asked to materialize on write.

### Increment inside the existing write query via a snapshot CTE

`RecordJobView` and `MarkJobApplied` become single-statement CTEs that upsert the
interaction and conditionally bump the job counter, detecting the transition by
reading the row's **prior** state before the upsert:

```sql
-- RecordJobView
WITH prior AS (
  SELECT 1 FROM user_jobs WHERE user_id = $1 AND job_id = $2
), upsert AS (
  INSERT INTO user_jobs (user_id, job_id) VALUES ($1, $2)
  ON CONFLICT (user_id, job_id) DO UPDATE SET viewed_at = now()
  RETURNING *
), bump AS (
  UPDATE jobs SET view_count = view_count + 1
  WHERE id = $2 AND NOT EXISTS (SELECT 1 FROM prior)
)
SELECT * FROM upsert;
```

```sql
-- MarkJobApplied
WITH prior AS (
  SELECT applied_at FROM user_jobs WHERE user_id = $1 AND job_id = $2
), upsert AS (
  INSERT INTO user_jobs (user_id, job_id, applied_at, stage)
  VALUES ($1, $2, now(), 'applied')
  ON CONFLICT (user_id, job_id) DO UPDATE
    SET applied_at = now(), stage = COALESCE(user_jobs.stage, 'applied')
  RETURNING *
), bump AS (
  UPDATE jobs SET applied_count = applied_count + 1
  WHERE id = $2 AND NOT EXISTS (SELECT 1 FROM prior WHERE applied_at IS NOT NULL)
)
SELECT * FROM upsert;
```

All CTEs see one MVCC snapshot, so `prior` reflects the OLD row: the `bump`
fires only on first-ever view / first `applied_at` transition, and `upsert`'s
`RETURNING` still returns the row every time (idempotency preserved). Chosen over
the `xmax = 0` insert-detection trick because the snapshot CTE also handles the
"row already existed but `applied_at` was NULL" case, which `xmax` cannot.

*Alternative — two statements in the Go repository (read then upsert):* rejected;
a single statement is atomic and avoids a race between the read and the bump.

### Backfill in the migration

`view_count` ← count of the job's `user_jobs` rows (every row has `viewed_at`
set); `applied_count` ← count filtered on `applied_at IS NOT NULL`.

### Display: omit zero counters

Render each count only when `> 0`, so a fresh posting never shows "0 views".
Shown to all visitors, muted, near the title/metadata on the detail page.

## Risks / Trade-offs

- **Signed-in-only undercount** → accepted and documented; the numbers are
  labelled as engagement among users, and it avoids anonymous bot inflation. The
  anonymous path is a clean future extension (a separate increment), not a
  refactor.
- **Backfill vs. incremental drift** → the backfill counts any interacting user
  as a viewer (all rows have `viewed_at`), whereas the incremental `view_count`
  bumps only on the `RecordJobView` insert. A user whose first interaction is a
  save-without-open would be counted by the backfill but not by a later view.
  This drift is at most ±1 per such user and immaterial for a social-proof
  number; not worth extra write-path touch points.
- **Concurrent first-interaction over-count** → the `prior` snapshot CTE detects
  the transition per-transaction, not across concurrent ones: two simultaneous
  first-views (or first-applies) of the *same* `(user, job)` both read an empty
  `prior` from their pre-write snapshots and both bump, yielding +2 for one
  distinct user. Requires the same user hitting the same job in two overlapping
  requests (double tab / double-click) inside the commit window — rare, and the
  effect is a cosmetic +1 on a metric already framed as approximate social proof.
  Accepted, not serialized: row locks / `xmax` insert-detection would harden it
  but add complexity disproportionate to a ±1 drift on an approximate count, and
  would not fully cover the apply case anyway. Documented so it isn't mistaken
  for an oversight.
- **Counter can't go negative / no decrement** → applies/views are monotonic
  here (no "un-apply" decrements the count). `applied_count` is not decremented
  by `ClearJobProgress`/`UntrackJob`; accepted — the counter is cumulative
  interest, not current-state. Documented so it isn't mistaken for a bug.
- **Migration is manual in prod** → no versioned runner; apply the migration by
  hand before deploying the new binary (existing project convention).

## Migration Plan

1. Add migration `00NN_job_engagement_counts.sql`: two columns + backfill.
2. `make sqlc` after editing `user_jobs.sql`; commit generated code.
3. Deploy order (prod): apply the migration by hand
   (`docker exec -i freehire-db psql -U hire -d hire < …`) **before** rolling the
   new app/web images, so the columns exist when the new queries run.
4. Rollback: the columns are additive and defaulted; an old binary ignores them.
   Reverting code needs no schema change.
