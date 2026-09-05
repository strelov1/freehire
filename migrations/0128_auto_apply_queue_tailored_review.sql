-- Carries a tailoring outcome through an auto-apply attempt to submission. tailored_cv_id is
-- set once tailoring finishes (openspec/changes/auto-apply-tailored-resume); reviewed_at and
-- review_decision are set once by the candidate's review decision — approve makes the entry
-- claimable by cmd/auto-apply, decline parks it via the same Park path an unresolved form
-- field already uses, with a distinct last_error text.
--
-- References cvs(id), which is uuid (0045 swapped it from a bigint identity to a random
-- UUID, per the opaque-cv-ids change) — not bigint.
ALTER TABLE public.auto_apply_queue
    ADD COLUMN tailored_cv_id uuid REFERENCES public.cvs (id),
    ADD COLUMN reviewed_at timestamptz,
    ADD COLUMN review_decision text
        CONSTRAINT auto_apply_queue_review_decision_check
        CHECK (review_decision IN ('approved', 'declined'));

-- A referencing column needs its own index (0045's own note): Postgres indexes only the
-- referenced side, so without this, deleting a CV scans auto_apply_queue.
CREATE INDEX auto_apply_queue_tailored_cv_id_idx ON public.auto_apply_queue (tailored_cv_id);
