-- migrate: no-transaction
--
-- Adds 'board_unreachable' to jobs.closed_reason's allowed values (see 0071): the
-- chronic-board safety net (openspec change close-chronically-unreachable-boards, issue
-- #2017) closes a board's jobs once it has proven unreachable for weeks, and that close
-- must carry its own mechanism label — not 'unseen', which means the ordinary per-run
-- sweep actually proved coverage this run, which is exactly what a chronic board never has.
--
-- Same large-table remedy as 0071: DROP + re-ADD ... NOT VALID skips the row scan under
-- ACCESS EXCLUSIVE, and the follow-up VALIDATE CONSTRAINT takes only SHARE UPDATE
-- EXCLUSIVE, which blocks neither readers nor writers. Validation cannot fail: every
-- existing row already satisfies the widened list, since it is the old list plus one value.
ALTER TABLE public.jobs
    DROP CONSTRAINT IF EXISTS jobs_closed_reason_check;

ALTER TABLE public.jobs
    ADD CONSTRAINT jobs_closed_reason_check CHECK (closed_reason = ANY (ARRAY[
        ''::text,                   -- closed before the column existed; unknown
        'unseen'::text,             -- dropped off a board we crawl (CloseUnseenJobs / …BySource)
        'feed_removed'::text,       -- a streaming source reported it taken down
        'moderated'::text,          -- a moderator closed it
        'probe_expired'::text,      -- two consecutive expired liveness probes
        'expired'::text,            -- age rule: no close signal exists for this source
        'board_unreachable'::text   -- chronic-board safety net: no successful crawl in 60+ days
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;
