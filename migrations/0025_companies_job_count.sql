-- Denormalized open-job count per company. ListCompanies previously computed this
-- on the fly via a LEFT JOIN + count() over jobs; at ~1.5M jobs that join — and
-- especially ordering all companies by it — is too costly to run on every
-- /companies request and every sidebar company-typeahead keystroke. The count is
-- now stored here and maintained by a periodic recompute (cmd/recount-companies),
-- so it is eventually consistent: it changes both when jobs are ingested and when
-- they are closed (closed_at set by the ingest sweep / liveness worker), which a
-- write-path trigger would have to chase on every upsert. Defaults to 0 so a
-- company with no open jobs reads as 0 between recomputes.
ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS job_count INT NOT NULL DEFAULT 0;

-- Serves the "most active first" ordering (ORDER BY job_count DESC, name) that
-- the company list and the sidebar typeahead's empty-query first page rely on.
CREATE INDEX IF NOT EXISTS companies_job_count_idx
    ON companies (job_count DESC, name);
