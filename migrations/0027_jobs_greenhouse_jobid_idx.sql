-- A partial index for the greenhouse job-id lookup in link contributions
-- (internal/contribution). Many companies embed Greenhouse on their own careers domain
-- server-side (company.com/careers/…/<gh_id>/), so the board token never appears in the URL
-- or page — only the Greenhouse job id. external_id is "<board>:<id>", so we find the board by
-- the id component: split_part(external_id, ':', 2) = '<gh_id>'.
--
-- Partial on source='greenhouse' (numeric ids are Greenhouse's; other ATS use UUIDs), so the
-- index is small (~greenhouse rows only). The expression must match the query's exactly
-- (literal ':', not chr(58)) for the planner to use it.
--
-- On a fresh initdb volume this plain CREATE INDEX is fine; on the live prod DB it was applied
-- manually as CREATE INDEX CONCURRENTLY.
CREATE INDEX jobs_greenhouse_jobid_idx
    ON public.jobs (split_part(external_id, ':', 2))
    WHERE source = 'greenhouse';
