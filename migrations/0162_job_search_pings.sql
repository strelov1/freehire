-- Which job URLs have been announced to which external search engine, so a bounded
-- daily budget is never spent twice on the same posting.
--
-- Deliberately a LEDGER, not an outbox. Every other queue in this schema
-- (search_outbox, recent_feed_outbox, enrichment_outbox) can drain faster than it
-- fills; this one cannot. Google's Indexing API grants 200 publish calls a day
-- against a catalogue of ~690k job pages, so an outbox fed by cmd/ingest would grow
-- without bound forever and the oldest row would never be reached. Recording what was
-- SENT instead lets cmd/search-ping choose the best candidates each run — newest
-- first — and lets the choice change without a backlog to unwind.
--
-- (job_id, engine) rather than a column on jobs (the shape jobs.hydrated_at uses)
-- because the engines have different budgets: Google is capped at 200/day while
-- IndexNow has no published quota, so one stamp cannot mean both. One row per
-- (posting, engine) also makes the send idempotent under ON CONFLICT DO NOTHING,
-- which is what keeps a re-run after a partial failure from double-spending.
--
-- engine is text and not an enum: this table is written by one worker against a list
-- of endpoints that is expected to change (IndexNow alone fronts Bing, Yandex, Seznam
-- and Naver), and an enum would make adding one a migration.
CREATE TABLE public.job_search_pings (
    job_id    bigint NOT NULL,
    engine    text NOT NULL,
    pinged_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT job_search_pings_pkey PRIMARY KEY (job_id, engine)
);

ALTER TABLE ONLY public.job_search_pings
    ADD CONSTRAINT job_search_pings_job_id_fkey
    FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;

-- The candidate query is "newest eligible postings this engine has not been told
-- about", an anti-join against this table. The primary key already serves the
-- per-(job, engine) probe that anti-join makes; this second index serves the other
-- direction, reporting what a given engine has covered and how recently, which is the
-- only way to see the budget actually being spent.
CREATE INDEX job_search_pings_engine_pinged_at_idx
    ON public.job_search_pings (engine, pinged_at DESC);
