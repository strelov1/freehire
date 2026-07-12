-- Partial index supporting the role-duplicate recompute's per-company aggregation
-- (MIN(id)/COUNT(*) GROUP BY company_slug, role_fingerprint over OPEN fingerprinted
-- jobs). Without it the recompute seq-scans the whole jobs table and spills the
-- HashAggregate to disk; with it each company's cluster read is an index range scan.
-- The existing jobs_company_role_fingerprint_idx is NOT partial (no closed_at filter),
-- so the planner falls back to a seq scan for the open-only aggregation.
--
-- Applied to a fresh volume by initdb after 0012; on an existing prod volume build it
-- CONCURRENTLY out of band (a plain CREATE INDEX would lock the live jobs table):
--   CREATE INDEX CONCURRENTLY jobs_open_role_cluster_idx
--     ON public.jobs (company_slug, role_fingerprint)
--     WHERE closed_at IS NULL AND role_fingerprint <> '';
CREATE INDEX IF NOT EXISTS jobs_open_role_cluster_idx
    ON public.jobs (company_slug, role_fingerprint)
    WHERE closed_at IS NULL AND role_fingerprint <> '';
