-- migrate: no-transaction
--
-- Adds 'employer_closed' to jobs.closed_reason's allowed values (see 0071, 0147, 0159 and
-- 0165 for the same widening, one value each time): a verified employer closed a vacancy
-- they published themselves through the new self-service flow (see
-- openspec/changes/add-employer-company-accounts).
--
-- It needs its own label rather than reusing 'moderated', the existing "a person closed it"
-- reason: 'moderated' means staff judged the posting, which is a different actor and a
-- different trust level than the employer closing their own vacancy. Conflating the two
-- would cost the one thing a closed_reason is for — telling an operator which mechanism
-- fired and which actor to hold accountable for it.
--
-- Same large-table remedy as 0071, 0147, 0159 and 0165: DROP + re-ADD ... NOT VALID skips
-- the row scan under ACCESS EXCLUSIVE, and the follow-up VALIDATE CONSTRAINT takes only
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
        'employer_closed'::text       -- a verified employer closed their own self-published vacancy
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;
