-- Removal queue for the facet index. The mirror image of search_outbox: that queue says
-- "index this job", this one says "drop this job's document".
--
-- It exists because nothing removed documents incrementally. ClaimSearchOutboxBatch filters
-- closed_at IS NULL, so a job that closes is never claimed again and its document simply
-- persists until the next full reindex swap replaces the whole index. search.Client.DeleteJobs
-- was written for this and never wired up — its only callers were integration tests. Measured
-- on prod 2026-08-18: 19,827 jobs close per day, 4,897 per 3-hour reindex cycle, and every one
-- of them stayed searchable until the next rebuild.
--
-- NO FOREIGN KEY TO jobs, deliberately — this is the one place this table must NOT mirror
-- search_outbox, which carries ON DELETE CASCADE.
--
-- cmd/prune is the only hard-delete path, and PruneJobs deletes by an explicit id list with no
-- closed_at condition, so it can remove an open, indexed job outright. With a cascading key the
-- sequence "job closes -> removal queued -> prune deletes the job before the drain runs" would
-- delete the queued removal too, leaving that document in the index permanently with nothing
-- left in the database that knows it should not be there.
--
-- The asymmetry is real rather than an oversight: search_outbox NEEDS the row, because the
-- drain reads it to build a document. A removal needs only the primary key, and the row being
-- gone is the ordinary case rather than a corruption to reap. So this queue takes no
-- referential dependency on the table it helps garbage-collect.
--
-- Applied to a fresh volume by initdb after 0112; on an existing prod volume this statement
-- must be run manually BEFORE deploying code that writes to the table.

CREATE TABLE public.search_delete_outbox (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    claimed_at timestamp with time zone,
    failed_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.search_delete_outbox ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.search_delete_outbox_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.search_delete_outbox
    ADD CONSTRAINT search_delete_outbox_pkey PRIMARY KEY (id);

-- One live entry per job: a job closed and re-closed (or closed then pruned) queues once.
ALTER TABLE ONLY public.search_delete_outbox
    ADD CONSTRAINT search_delete_outbox_job_id_key UNIQUE (job_id);

-- Partial index over claimable (not dead-lettered) entries, mirroring
-- search_outbox_claimable_idx. Claim order is insertion order: unlike indexing, where a
-- fresher posting matters more to a searcher, one stale document is as wrong as another.
CREATE INDEX search_delete_outbox_claimable_idx ON public.search_delete_outbox USING btree (id) WHERE (failed_at IS NULL);
