-- Per-(user, job) cover letter draft (see the add-cover-letter-draft change). One row holds
-- the current letter for a single candidate against a single vacancy, written by the
-- three-stage chain in internal/candidate/coverletter and served from here until it goes
-- stale.
--
-- There is exactly ONE row per pair and no history. A CV earns cvedit's revisions because it
-- is a structured document edited over months; a letter is a paragraph regenerated in
-- seconds, and a revision log over it would be a feed of near-identical prose nobody reads.
-- Drafting again overwrites.
--
-- Staleness is double-stamped, and the second stamp is the one that differs from every
-- neighbour. model records the LLM that produced the letter, so an LLM_MODEL upgrade reports
-- the row stale exactly as it does for user_job_analysis. language records the language the
-- letter was WRITTEN in, which comes from jobs.posting_language and not from the candidate's
-- profile: a fit analysis is the candidate reading themselves, a cover letter is read by the
-- employer. Without the stamp, a posting re-detected into a different language would keep
-- serving a letter aimed at the wrong reader.
--
-- There is deliberately no job_content_hash stamp. A letter is aimed at a role; an edit that
-- changes a word of the posting does not make it wrong the way it makes a
-- requirement-by-requirement analysis wrong.
--
-- cited_atom_ids holds the experience_atoms the letter is built on. It carries NO foreign key
-- and none is wanted: an array cannot hold one, and the letter is a snapshot the same way a
-- rendered CV is. An owner who deletes a cited atom leaves a dangling id here, and that is
-- the correct outcome — the letter as sent still said what it said. The read path resolves
-- what it can and shows the rest as gone.
--
-- FKs cascade: deleting the user or the job removes the draft, which has no meaning without
-- both.

CREATE TABLE public.cover_letters (
    user_id        bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    job_id         bigint      NOT NULL REFERENCES public.jobs(id) ON DELETE CASCADE,
    body           text        NOT NULL,
    cited_atom_ids uuid[]      NOT NULL DEFAULT '{}'::uuid[],
    language       text        NOT NULL,
    model          text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, job_id)
);
