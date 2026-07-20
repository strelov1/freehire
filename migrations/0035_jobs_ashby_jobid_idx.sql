-- A partial index for the ashby job-id lookup in link contributions (internal/contribution).
-- Many companies embed Ashby on their own careers domain via the ashby_jid widget param
-- (company.com/careers?ashby_jid=<uuid>); the board slug is JS-rendered, so it never appears in
-- the URL or page — only the Ashby job id. external_id is "<board>:<uuid>", so we find the board
-- by the id component: split_part(external_id, ':', 2) = '<uuid>'.
--
-- Partial on source='ashby', so the index is small (~ashby rows only). The expression must match
-- the query's exactly (literal ':', not chr(58)) for the planner to use it. Mirrors
-- 0027_jobs_greenhouse_jobid_idx.
--
-- On a fresh initdb volume this plain CREATE INDEX is fine; on the live prod DB apply it
-- manually as CREATE INDEX CONCURRENTLY.
CREATE INDEX jobs_ashby_jobid_idx
    ON public.jobs (split_part(external_id, ':', 2))
    WHERE source = 'ashby';
