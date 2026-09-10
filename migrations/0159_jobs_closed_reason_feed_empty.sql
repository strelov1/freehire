-- migrate: no-transaction
--
-- Adds 'feed_empty' to jobs.closed_reason's allowed values (see 0071, and 0147 for the same
-- widening one value earlier): the empty-feed safety net (cmd/close-chronic-boards' second
-- window, board_health.last_yield_at from 0158) closes a board's jobs once its feed has
-- answered successfully but carried nothing for weeks.
--
-- It needs its own label rather than reusing one of the three that already exist, because each
-- of those asserts something this close cannot:
--   'unseen'            — the ordinary per-run sweep PROVED coverage this run. An empty-feed
--                         board never proves coverage; that is why the sweep cannot reach it.
--   'board_unreachable' — no successful crawl in 60+ days. Here every crawl SUCCEEDED; the
--                         feed simply had nothing in it, which is the opposite diagnosis and
--                         points at the source's inventory rather than at our access to it.
--   'feed_removed'      — a streaming source reported this posting taken down, one posting at
--                         a time. Here nothing was reported about any individual posting; the
--                         whole listing came back empty.
-- Conflating them would cost the one thing a closed_reason is for: telling an operator which
-- mechanism fired, and therefore where to look.
--
-- Same large-table remedy as 0071 and 0147: DROP + re-ADD ... NOT VALID skips the row scan
-- under ACCESS EXCLUSIVE, and the follow-up VALIDATE CONSTRAINT takes only SHARE UPDATE
-- EXCLUSIVE, which blocks neither readers nor writers. Validation cannot fail: every existing
-- row already satisfies the widened list, since it is the old list plus one value.
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
        'board_unreachable'::text,  -- chronic-board safety net: no successful crawl in 60+ days
        'feed_empty'::text          -- empty-feed safety net: crawls succeed, feed carries nothing
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;
