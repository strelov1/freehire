-- migrate: no-transaction
--
-- Adds 'board_retired' to jobs.closed_reason's allowed values (see 0071, 0147, 0159, 0165 and
-- 0175 for the same widening, one value each time): a curator retired the board the posting
-- was crawled from (boardcatalog.Retire, via cmd/add-board --retire), so nothing will crawl
-- it again and nothing else would ever close it (freehire#3796 — aijobs kept 2,569 open
-- postings for eight weeks after its retirement).
--
-- It needs its own label rather than reusing 'board_unreachable': that one is the chronic
-- safety net's verdict after 60 days of failed crawls, this one is a person deciding the
-- board is gone. Conflating them would cost the one thing a closed_reason is for — telling
-- an operator which mechanism fired.
--
-- Same large-table remedy as 0071, 0147, 0159, 0165 and 0175: DROP + re-ADD ... NOT VALID
-- skips the row scan under ACCESS EXCLUSIVE, and the follow-up VALIDATE CONSTRAINT takes only
-- SHARE UPDATE EXCLUSIVE, blocking neither readers nor writers. Validation cannot fail:
-- every existing row already satisfies the widened list, since it is the old list plus one
-- value.
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
        'board_retired'::text         -- a curator retired the board it was crawled from
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;
