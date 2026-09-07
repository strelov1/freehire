-- Live "recently added jobs" feed queue (openspec/changes/add-homepage-recent-jobs-feed).
--
-- Unlike search_outbox/enrichment_outbox, this is a pure transit queue: no lease,
-- retry, or dead-letter bookkeeping. It is drained by a ticker goroutine inside the
-- long-lived cmd/server process (internal/job/recentfeed), not a separate cron worker,
-- and its rows are cosmetic — losing one changes nothing but what a homepage visitor
-- happens to see for a few seconds. job_id is the primary key rather than a surrogate
-- id: cmd/ingest's ON CONFLICT (job_id) DO NOTHING enqueue already guarantees at most
-- one live entry per job, exactly like search_outbox's separate UNIQUE constraint.
CREATE TABLE public.recent_feed_outbox (
    job_id     bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT recent_feed_outbox_pkey PRIMARY KEY (job_id)
);

ALTER TABLE ONLY public.recent_feed_outbox
    ADD CONSTRAINT recent_feed_outbox_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;
