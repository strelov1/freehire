-- 0128 added auto_apply_queue.tailored_cv_id REFERENCES cvs(id) with no ON DELETE clause,
-- which defaults to RESTRICT — the one FK to cvs(id) in the whole schema that does. 0128 is
-- already applied (production ran it on 2026-09-04), so per this repo's own rule it is never
-- edited in place; this is the follow-up fix instead. Idempotent by shape (DROP+ADD the same
-- constraint), so it is safe to run against any environment regardless of exactly which
-- version of 0128 it happened to apply.
--
-- SET NULL, not CASCADE: unlike cv_revisions/cv_tracer_links (which describe the CV being
-- deleted and have no reason to survive it), an auto_apply_queue row is the candidate's own
-- application-tracking record — losing the CV that was tailored for it must not delete the
-- attempt itself, only forget which CV it was. Matches referral_requests' own precedent for
-- exactly this "record survives, pointer clears" shape.
ALTER TABLE public.auto_apply_queue
    DROP CONSTRAINT IF EXISTS auto_apply_queue_tailored_cv_id_fkey,
    ADD CONSTRAINT auto_apply_queue_tailored_cv_id_fkey
        FOREIGN KEY (tailored_cv_id) REFERENCES public.cvs (id) ON DELETE SET NULL;
