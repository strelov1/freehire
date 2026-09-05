-- Membership of job_lists (0135): a job may belong to any number of lists, and a
-- list may hold any number of jobs — a plain many-to-many join, with no ordering
-- column. Listing sorts by added_at DESC; manual reordering is not supported.

CREATE TABLE public.job_list_items (
    list_id bigint NOT NULL,
    job_id bigint NOT NULL,
    added_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT job_list_items_pkey PRIMARY KEY (list_id, job_id)
);

ALTER TABLE ONLY public.job_list_items
    ADD CONSTRAINT job_list_items_list_id_fkey FOREIGN KEY (list_id) REFERENCES public.job_lists(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.job_list_items
    ADD CONSTRAINT job_list_items_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;

-- A shared list's public read walks this direction: given a list, its jobs newest-added first.
CREATE INDEX job_list_items_list_added_idx ON public.job_list_items (list_id, added_at DESC);
