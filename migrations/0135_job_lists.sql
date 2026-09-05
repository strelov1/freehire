-- A user-owned, named set of specific jobs — independent of the single-flag
-- user_jobs.saved_at "save" and unrelated to the code-owned job/collections registry
-- (company-level Big Tech/YC/visa-sponsor tags). See the
-- replace-board-sharing-with-collections change: this replaces public saved-search
-- sharing ("boards"), which shared a live query rather than a fixed set of jobs.
--
-- public_slug mirrors saved_searches' board-sharing scheme (a partial unique index,
-- so private lists — the overwhelming majority — carry NULL without colliding). No
-- author_label: unlike a board, a shared list already carries its own free-text
-- description, so a second attribution field would be redundant.

CREATE TABLE public.job_lists (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    public_slug text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT job_lists_pkey PRIMARY KEY (id),
    CONSTRAINT job_lists_name_check
        CHECK ((length(TRIM(BOTH FROM name)) >= 1) AND (length(TRIM(BOTH FROM name)) <= 100)),
    CONSTRAINT job_lists_description_check CHECK (length(description) <= 2000)
);

ALTER TABLE public.job_lists ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.job_lists_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.job_lists
    ADD CONSTRAINT job_lists_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.job_lists
    ADD CONSTRAINT job_lists_user_id_name_key UNIQUE (user_id, name);

-- The "My lists" ordering: most recently updated first, per user.
CREATE INDEX job_lists_user_updated_idx ON public.job_lists (user_id, updated_at DESC);

-- Public read by slug. A plain unique index (not a constraint) tolerates the many
-- NULLs a mostly-private feature produces.
CREATE UNIQUE INDEX job_lists_public_slug_idx ON public.job_lists (public_slug);
