-- The CV revision log (see the cv-revision-history change): one row per change to a stored
-- CV, whoever made it — the candidate's editor, the tailoring agent, the CLI, the template
-- picker, or the system seeding a tailored copy. It replaces cvs.autopilot_undo, which held
-- a single pre-run snapshot and could therefore answer only "put the whole run back".
--
-- ops is what the change did; inverse is what would undo it, computed while the previous
-- value was still in hand. Undo applies the inverse to the CURRENT document as a NEW
-- revision (reverts_id points at the one being undone, and the undone row gets reverted_at),
-- so edits made after it survive and the log is never rewritten.
--
-- batch_id groups the revisions of one agent turn or autopilot run, which is how a whole run
-- is reverted now that there is no snapshot to restore.
--
-- base_version records the document's updated_at before the change, for the journal rather
-- than for locking: the differ always compares against the state just read, and an agent's
-- operations address paths, so a path that has since disappeared is refused when applied.
--
-- user_id is denormalised from cvs so every read is owner-scoped in one predicate.
--
-- The log is capped per CV (trimmed by the same statement that inserts), so these two jsonb
-- columns cannot grow without bound on the table behind every CV page.
--
-- Applied to a fresh volume by initdb after 0059; on an existing prod volume run this file
-- manually (SET ROLE hire) BEFORE deploying code that reads it.

CREATE TABLE public.cv_revisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    cv_id uuid NOT NULL,
    user_id bigint NOT NULL,
    actor text NOT NULL,
    origin text NOT NULL,
    batch_id uuid,
    title text NOT NULL,
    note text,
    ops jsonb NOT NULL,
    inverse jsonb NOT NULL,
    base_version timestamp with time zone NOT NULL,
    reverts_id uuid,
    reverted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.cv_revisions
    ADD CONSTRAINT cv_revisions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.cv_revisions
    ADD CONSTRAINT cv_revisions_cv_id_fkey FOREIGN KEY (cv_id) REFERENCES public.cvs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.cv_revisions
    ADD CONSTRAINT cv_revisions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- A revert points at what it reverted. SET NULL rather than CASCADE: the trim drops the
-- oldest rows first, and losing the undo along with the edit it undid would remove a change
-- from the log that the document still reflects.
ALTER TABLE ONLY public.cv_revisions
    ADD CONSTRAINT cv_revisions_reverts_id_fkey FOREIGN KEY (reverts_id) REFERENCES public.cv_revisions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.cv_revisions
    ADD CONSTRAINT cv_revisions_actor_check CHECK (actor IN ('candidate', 'agent', 'system'));

-- The feed, and the coalescing lookup that reads only the newest row, are the same order.
CREATE INDEX cv_revisions_cv_id_created_at_idx ON public.cv_revisions (cv_id, created_at DESC);

-- Reverting a run reads every revision of one batch.
CREATE INDEX cv_revisions_batch_id_idx ON public.cv_revisions (batch_id) WHERE batch_id IS NOT NULL;
