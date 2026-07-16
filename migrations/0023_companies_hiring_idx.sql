-- Partial index backing the /companies catalog, which now lists only companies
-- with at least one open job (ListCompanies/CountCompanies filter job_count > 0).
-- Half the companies table is job-less reference rows (~92k of ~226k) imported by
-- the YC and company-info backfills; they carry job_count = 0 and are excluded from
-- the catalog. Without this partial index the default CountCompanies degrades to a
-- seq scan of the whole 2.3 GB heap (~60 s under load), and ListCompanies scans the
-- full non-partial companies_job_count_idx. With it, both reads touch only the ~134k
-- hiring rows: ListCompanies' ORDER BY job_count DESC, name LIMIT is a tiny index
-- range scan, and the default (unfiltered) count is an index-only scan.
--
-- Applied to a fresh volume by initdb after 0022; on an existing prod volume build it
-- CONCURRENTLY out of band (a plain CREATE INDEX would lock the live companies table):
--   CREATE INDEX CONCURRENTLY companies_hiring_job_count_idx
--     ON public.companies (job_count DESC, name)
--     WHERE job_count > 0;
CREATE INDEX IF NOT EXISTS companies_hiring_job_count_idx
    ON public.companies (job_count DESC, name)
    WHERE job_count > 0;
