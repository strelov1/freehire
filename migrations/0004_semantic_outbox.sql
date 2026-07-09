-- Incremental semantic-embedding queue + per-job embed provenance (see the
-- incremental-semantic-embedding change). Mirrors enrichment_outbox: a
-- reference-only outbox (job_id + target_model + lease/retry bookkeeping), drained
-- by the cmd/embed worker, which embeds each open job and upserts its vector into
-- jobs_semantic in place (no swap) and removes closed jobs. jobs_semantic stays the
-- vector store; jobs stays canonical.
--
-- target_model is the embedder identity (e.g. the e5 model), the staleness key:
-- mirrors users.resume_embedding_model — a model change re-embeds the catalogue by
-- making every jobs.semantic_embedded_model distinct from the new target. The
-- companion jobs.semantic_embedded_hash records the content_hash the stored vector
-- was built from, so a content change re-enqueues the job. Together they are the
-- "done" marker that keeps the idempotent backfill enqueue from re-queuing the whole
-- catalogue after each drain.
--
-- Applied to a fresh volume by initdb after 0003; on an existing prod volume these
-- statements must be run manually BEFORE deploying code that reads the columns.
-- Additive with no backfill — NULL stamps self-heal (every open job reads as
-- "needs embedding" once), exactly like content_hash.

CREATE TABLE public.semantic_outbox (
    id bigint NOT NULL,
    job_id bigint NOT NULL,
    target_model text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    claimed_at timestamp with time zone,
    failed_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.semantic_outbox ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.semantic_outbox_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.semantic_outbox
    ADD CONSTRAINT semantic_outbox_pkey PRIMARY KEY (id);

-- One live entry per (job, target model): the enqueue dedup key, so re-running the
-- backfill never duplicates work.
ALTER TABLE ONLY public.semantic_outbox
    ADD CONSTRAINT semantic_outbox_job_id_target_model_key UNIQUE (job_id, target_model);

ALTER TABLE ONLY public.semantic_outbox
    ADD CONSTRAINT semantic_outbox_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;

-- Partial index over claimable (not dead-lettered) entries, mirroring
-- enrichment_outbox_claimable_idx.
CREATE INDEX semantic_outbox_claimable_idx ON public.semantic_outbox USING btree (id) WHERE (failed_at IS NULL);

ALTER TABLE public.jobs
    ADD COLUMN semantic_embedded_model text,
    ADD COLUMN semantic_embedded_hash text;
