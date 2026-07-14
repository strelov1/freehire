-- Email → application linking + status classification (see the
-- mail-to-application-linking change). Adds the link/classification columns to
-- emails and a reference-only outbox mirroring enrichment_outbox / semantic_outbox,
-- drained by the cmd/classify-mail worker.
--
-- emails.job_id            the resolved application (auto or user-confirmed link)
-- emails.suggested_job_id  a pending suggestion awaiting the caller's confirmation
-- emails.link_source       how job_id was set: 'auto' | 'manual' (NULL = unlinked)
-- emails.match_confidence  the matcher's confidence, for display/debug
-- emails.status_signal     the classified status (mailclassify controlled vocabulary)
-- emails.classified_at     provenance stamp: the classification "done" marker, so the
--                          enqueue-pending sweep never re-queues an already-classified email
-- emails.classification_model  which model produced the classification
--
-- job_id / suggested_job_id use ON DELETE SET NULL: a job row removed out from under
-- an email must not cascade-delete the mail — the link just clears.
--
-- Applied to a fresh volume by initdb after 0001; on an existing prod volume these
-- statements must be run manually (as role hire, per the live-table discipline)
-- BEFORE deploying code that reads the columns. Additive with no backfill — a NULL
-- classified_at reads as "needs classification" once, then self-heals.

ALTER TABLE public.emails
    ADD COLUMN job_id               bigint,
    ADD COLUMN suggested_job_id     bigint,
    ADD COLUMN link_source          text,
    ADD COLUMN match_confidence     real,
    ADD COLUMN status_signal        text,
    ADD COLUMN classified_at        timestamp with time zone,
    ADD COLUMN classification_model text;

ALTER TABLE ONLY public.emails
    ADD CONSTRAINT emails_job_id_fkey
        FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE SET NULL,
    ADD CONSTRAINT emails_suggested_job_id_fkey
        FOREIGN KEY (suggested_job_id) REFERENCES public.jobs(id) ON DELETE SET NULL;

-- Fetching an application's linked emails filters on job_id; partial (linked rows only).
CREATE INDEX emails_job_id_idx ON public.emails USING btree (job_id) WHERE (job_id IS NOT NULL);

CREATE TABLE public.email_classification_outbox (
    id         bigint NOT NULL,
    email_id   bigint NOT NULL,
    attempts   integer DEFAULT 0 NOT NULL,
    claimed_at timestamp with time zone,
    failed_at  timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.email_classification_outbox ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.email_classification_outbox_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.email_classification_outbox
    ADD CONSTRAINT email_classification_outbox_pkey PRIMARY KEY (id);

-- One live entry per email: the enqueue dedup key, so re-running the pending sweep
-- never duplicates work.
ALTER TABLE ONLY public.email_classification_outbox
    ADD CONSTRAINT email_classification_outbox_email_id_key UNIQUE (email_id);

ALTER TABLE ONLY public.email_classification_outbox
    ADD CONSTRAINT email_classification_outbox_email_id_fkey
        FOREIGN KEY (email_id) REFERENCES public.emails(id) ON DELETE CASCADE;

-- Partial index over claimable (not dead-lettered) entries, mirroring
-- enrichment_outbox_claimable_idx.
CREATE INDEX email_classification_outbox_claimable_idx
    ON public.email_classification_outbox USING btree (id) WHERE (failed_at IS NULL);
