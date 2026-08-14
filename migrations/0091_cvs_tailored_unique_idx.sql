-- Store.Tailor was a non-transactional check-then-insert: look up the user's existing tailored
-- copy for a vacancy, and if none, insert one. Nothing in the schema stopped two concurrent
-- calls for the same (user, job) from both missing the check and both inserting — the
-- production incident TestStoreTailorReturnsTheExistingCopyForTheSameVacancy documents. This
-- index turns the second insert into a unique violation, which Store.Tailor now catches and
-- resolves by re-fetching the row the first call created.
--
-- Partial on is_tailored, not job_id IS NOT NULL: a tailored CV whose vacancy is later pruned
-- keeps is_tailored true but loses job_id (see 0058), and by then this index no longer applies
-- to it anyway — job_id NULL cannot collide with anything.
CREATE UNIQUE INDEX IF NOT EXISTS cvs_user_id_job_id_tailored_uniq_idx
    ON public.cvs (user_id, job_id) WHERE is_tailored;
