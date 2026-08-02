## Why

Every crawl rewrites every row it re-sees, whether or not anything changed: `UpsertJob`'s
`ON CONFLICT DO UPDATE` carries no `WHERE`, so a re-ingest of an untouched posting writes a
full new tuple — a re-TOAST of the ~2.5 KB description, an entry in each of the table's 21
indexes, WAL for all of it, and a dead tuple behind it. Measured on prod 2026-08-02: `jobs`
took 331M updates against 5.6M rows (59 per row) and `companies` 286M against 305k (937 per
row) in eleven days, leaving ~43 GB of a 102 GB database as bloat rather than data.

The write path already computes the signal that says none of this was needed — `UpsertJob`
returns `changed`, and the search push honours it. Postgres does not: by the time the flag
is read, the row has been rewritten.

## What Changes

- Add a `RefreshUnchangedJob` query: a narrow `UPDATE jobs SET last_seen_at = now()` matched
  on `(source, external_id, content_hash, cities)` and guarded by `closed_at IS NULL`. It
  writes no indexed column, so the update is HOT-eligible and maintains no index. `cities` is
  in the key because it is the one written column `jobhash.Of` does not read.
- `cmd/ingest`'s private `save` tries that query first and falls back to `UpsertJob` only
  when it matches nothing. The rest of `save` is unchanged — the role-cluster lookup and the
  index push are already gated on `Inserted`/`Changed`, which the cheap branch reports false.
- `UpsertJob` and `TouchJob` stop stamping `updated_at` on a write that changed nothing, so
  the column comes to mean "content last changed" instead of "last crawled". A reopen still
  stamps it, because the reindex must see it.
- Guard the `company_upsert` CTE inside `UpsertJob` with
  `WHERE companies.name IS DISTINCT FROM EXCLUDED.name`. The two sibling CTEs in
  `UpsertManualJob`/`UpdateManualJob` fire once per moderator action and are left alone.
- Set `fillfactor = 90` and `autovacuum_vacuum_scale_factor = 0.02` on `jobs` (own migration,
  with `lock_timeout` — it takes a brief ACCESS EXCLUSIVE lock).
- Log the cheap-path hit rate per provider, once per ingest run. The saving is proportional
  to that rate and nothing measures it today.

No breaking changes: every observable behaviour above is preserved deliberately, and the
tests named in `design.md` exist to prove it.

## Capabilities

### New Capabilities

- `ingest-write-economy`: what an ingest pass is allowed to write when it re-sees a posting
  that has not changed — the liveness refresh, the columns that must stay untouched, the
  meaning of `updated_at`, the company-row guard, and the visibility rule that makes a
  provider whose rows never match observable rather than silent.

### Modified Capabilities

None. Three neighbouring specs were checked and none of their requirements move:

- `job-lifecycle` — "SHALL stamp `last_seen_at` every time ingest upserts it" and "SHALL
  clear `closed_at` when ingest upserts a job that was previously closed" both still hold;
  the `closed_at IS NULL` guard on the cheap path exists precisely so the reopen keeps
  running through `UpsertJob`.
- `job-search` — "An unchanged re-ingest does not re-push the document" is unchanged; the
  cheap path reports the same `Inserted`/`Changed` the existing gate already reads.
- `source-ingest` — the pipeline still persists every fetched posting through the job write
  path; which statement it uses is not a spec-level fact.

## Impact

- `internal/db/queries/jobs.sql` — new `RefreshUnchangedJob`; `UpsertJob` and `TouchJob`
  amended. Regenerated via `make sqlc`.
- `cmd/ingest/store.go` — `save` gains the try-cheap-first seam, shared by `Save` and
  `SaveWithApplyForm`.
- `migrations/` — one new file for the storage parameters, deployed on its own.
- `internal/jobhash` — a test asserting every `RoleFingerprint` input is also a `jobhash.Of`
  input, so a future field cannot make the fingerprint go stale on the cheap path.
- Downstream, unblocked rather than changed: both `updated_at` columns stop reporting "just
  now" on every crawl, so the jobs and companies sitemaps serve a `<lastmod>` that means
  something, and `ListJobsUpdatedAfter` becomes viable for an incremental reindex. That query
  is dormant today — no caller, and `cmd/reindex` has no `--since` flag despite its comment —
  because a column stamped on every crawl selects the whole catalogue.
- Out of scope, and dependent on this landing first: reclaiming the accreted ~43 GB with
  `pg_repack`, dropping the two ~1 GB indexes with near-zero scans, purging
  `semantic_embedding`, and tuning autovacuum workers.
