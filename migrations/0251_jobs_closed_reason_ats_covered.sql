-- migrate: no-transaction
--
-- Adds 'ats_covered' to jobs.closed_reason's allowed values (see 0212 for the same widening):
-- an aggregator re-listed a posting whose employer we already crawl first-hand
-- (CloseCoveredJobs), so the aggregator copy is closed instead of kept alive beside the
-- employer's own postings. Reversible by construction: once the coverage lapses, the next
-- aggregator crawl writes the posting again and UpsertJob reopens it.
--
-- Its own label because it is our decision, not the market's: insights' realCloseReasons
-- must not count it as a posting that filled.
--
-- Same large-table remedy as 0212: DROP + re-ADD ... NOT VALID, then VALIDATE.
ALTER TABLE public.jobs
    DROP CONSTRAINT IF EXISTS jobs_closed_reason_check;

ALTER TABLE public.jobs
    ADD CONSTRAINT jobs_closed_reason_check CHECK (closed_reason = ANY (ARRAY[
        ''::text,                     -- closed before the column existed; unknown
        'unseen'::text,               -- dropped off a board we crawl (CloseUnseenJobs / …BySource)
        'feed_removed'::text,         -- a streaming source reported it taken down
        'moderated'::text,            -- a moderator closed it
        'probe_expired'::text,        -- two consecutive expired liveness probes
        'expired'::text,              -- age rule: no close signal exists for this source
        'board_unreachable'::text,    -- chronic-board safety net: no successful crawl in 60+ days
        'feed_empty'::text,           -- empty-feed safety net: crawls succeed, feed carries nothing
        'source_misattributed'::text, -- what we stored about it is wrong and unrepairable in place
        'employer_closed'::text,      -- a verified employer closed their own self-published vacancy
        'board_retired'::text,        -- a curator retired the board it was crawled from
        'ats_covered'::text           -- an aggregator copy of an employer we crawl first-hand
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;
