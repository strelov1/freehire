-- The company slug with its hyphens removed, stored as a real column.
--
-- WHY A COLUMN AND NOT THE EXPRESSION IT DUPLICATES. The aggregator-suppression pass
-- filters batches of companies on `replace(company_slug, '-', '')`. That is an
-- EXPRESSION over a column, and when the batch arrives as a query parameter the planner
-- has no usable selectivity estimate for it: measured on prod 2026-08-16 it expected
-- 1.4M rows and got 734, so it drove each batch off the source index instead of
-- jobs_open_company_slug_folded_idx and read ~927k rows per batch of 500 companies —
-- 271s each, ~23h for the pass, against a 12h unit timeout the reindex never survived
-- (zero successful reindexes in the three days before this was found).
--
-- Nothing about the QUERY fixes that. Measured, same batch: array parameter 259s, JOIN
-- over unnest 315s, LATERAL per company 300s. Nor does better statistics: raising the
-- functional index's statistics target moved its n_distinct from 16,817 to 147,101 and
-- the query stayed at 298s, because the planner does not consult it for
-- `expression = ANY($param)` at all. Only literals inlined in the query text are fast
-- (1.8s) — i.e. only when the planner can see the values.
--
-- A plain column has none of that problem. Control measurement on prod, the SAME
-- predicate shape and the same parameter passing, against the existing company_slug
-- column (~236k distinct): Index Scan, estimate off by 6x rather than 2000x, **491ms**.
-- That is the entire justification for storing a value we can compute.
--
-- WHY NOT `GENERATED ALWAYS AS ... STORED`, which is what this obviously wants to be:
-- adding a generated column rewrites the whole table, and jobs is 7.4M rows / 19 GB heap
-- / 95 GB with indexes. That is many minutes under ACCESS EXCLUSIVE — 20 seconds of it
-- on the much smaller companies table already cost 83 timed-out user requests earlier
-- the same day. A nullable column with no default is a catalog-only change: instant, no
-- rewrite, no lock worth naming.
--
-- The cost of that choice is that the value is maintained by the write paths rather than
-- by the engine. Every statement that writes jobs.company_slug must write this too, and
-- a test enforces exactly that (internal/db/folded_slug_rule_test.go) precisely because
-- "remember to update the other column" is not a thing to leave to memory.
--
-- BACKFILL AND INDEX ARE NOT IN THIS FILE. Both are online operations that must not run
-- inside a migration transaction:
--
--   -- backfill in chunks, so no single statement holds a long transaction and
--   -- autovacuum can keep up with the dead rows it creates:
--   UPDATE jobs SET company_slug_folded = replace(company_slug, '-', '')
--   WHERE id BETWEEN $1 AND $2 AND company_slug_folded IS DISTINCT FROM replace(company_slug, '-', '');
--
--   -- then, from a file under systemd-run — never `psql -c` over ssh, where a dropped
--   -- connection aborts CONCURRENTLY and leaves an INVALID index that the planner
--   -- ignores while it still costs every write:
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_open_company_slug_folded_col_idx
--       ON public.jobs (company_slug_folded)
--       WHERE closed_at IS NULL AND company_slug <> '';
--   SELECT indisvalid FROM pg_index WHERE indexrelid = 'jobs_open_company_slug_folded_col_idx'::regclass;
--
-- Until the backfill completes, rows carry NULL here and the suppression pass simply
-- does not match them — it degrades to suppressing less, never to suppressing wrongly.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS company_slug_folded text;

COMMENT ON COLUMN jobs.company_slug_folded IS
    'company_slug with hyphens removed. Maintained by every write path that sets '
    'company_slug (enforced by a test); exists so the aggregator-suppression pass can '
    'filter on a column the planner can estimate instead of an expression it cannot.';
