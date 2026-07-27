-- Account deletion timed out in production: DELETE FROM users took longer than the
-- proxy's read timeout, so the member saw a 504 while Postgres went on and erased the
-- account anyway — the one request in the API where a lost response is worst.
--
-- The cost was not the cascade. jobs.created_by and jobs.updated_by reference users
-- ON DELETE SET NULL, and Postgres creates no index for a foreign key on the
-- referencing side, so each delete ran two
--     UPDATE jobs SET <col> = NULL WHERE <col> = $1
-- referential-integrity queries as sequential scans over the whole catalogue — 5.7M
-- rows, 19 GB of heap, twice.
--
-- Every remaining unindexed reference to users is indexed here as well. Those tables
-- are small today, which is the only reason they were not also part of the timeout —
-- deleting an account must not get slower as the site grows, and the same RI query
-- runs against each of them.
--
-- Partial on NOT NULL because that is exactly the set an RI query can match, and none
-- of these columns is widely populated: authorship and moderation stamps are the
-- exception, not the rule (pg_stats reports null_frac = 1 for both jobs columns). The
-- indexes therefore cost kilobytes, not gigabytes.
--
-- On a fresh initdb volume these plain CREATE INDEX statements are fine; on the live
-- prod DB they are applied manually, the two on jobs as CREATE INDEX CONCURRENTLY.
CREATE INDEX jobs_created_by_idx ON public.jobs (created_by) WHERE created_by IS NOT NULL;
CREATE INDEX jobs_updated_by_idx ON public.jobs (updated_by) WHERE updated_by IS NOT NULL;

CREATE INDEX job_reports_reviewed_by_idx ON public.job_reports (reviewed_by) WHERE reviewed_by IS NOT NULL;
CREATE INDEX job_submissions_reviewed_by_idx ON public.job_submissions (reviewed_by) WHERE reviewed_by IS NOT NULL;
CREATE INDEX referral_offers_decided_by_idx ON public.referral_offers (decided_by) WHERE decided_by IS NOT NULL;
CREATE INDEX referral_requests_acted_by_idx ON public.referral_requests (acted_by) WHERE acted_by IS NOT NULL;
