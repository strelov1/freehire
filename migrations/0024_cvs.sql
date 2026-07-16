-- CV builder: a signed-in user owns N editable CVs (see the add-cv-builder change).
-- Each CV is a structured document stored as JSON (data), rendered to PDF on demand
-- via Typst; metadata (title, template_id) lives in columns. The first CV is seeded
-- from the user's resume_structured extraction. This table is additive and independent
-- of the single stored résumé (users.resume_object_key) and of the job-fit/verdict paths.
--
-- job_id is a nullable seam for the follow-up per-vacancy tailoring phase (a CV bound to
-- a job); unused in phase 1. ON DELETE SET NULL so a CV outlives the vacancy it targeted.
--
-- Applied to a fresh volume by initdb after 0023; on an existing prod volume these
-- statements must be run manually BEFORE deploying code that reads the table.

CREATE TABLE public.cvs (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    title text NOT NULL,
    template_id text DEFAULT 'classic-ats'::text NOT NULL,
    data jsonb NOT NULL,
    job_id bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.cvs ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.cvs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.cvs
    ADD CONSTRAINT cvs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.cvs
    ADD CONSTRAINT cvs_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.cvs
    ADD CONSTRAINT cvs_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE SET NULL;

-- The list query: a user's CVs, newest edit first.
CREATE INDEX cvs_user_id_updated_at_idx ON public.cvs USING btree (user_id, updated_at DESC);
