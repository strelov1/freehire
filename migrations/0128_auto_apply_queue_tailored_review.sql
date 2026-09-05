-- Carries a tailoring outcome through an auto-apply attempt to submission. tailored_cv_id is
-- set once tailoring finishes (openspec/changes/auto-apply-tailored-resume); reviewed_at and
-- review_decision are set once by the candidate's review decision — approve makes the entry
-- claimable by cmd/auto-apply, decline parks it via the same Park path an unresolved form
-- field already uses, with a distinct last_error text.
--
-- References cvs(id), which is uuid (0045 swapped it from a bigint identity to a random
-- UUID, per the opaque-cv-ids change) — not bigint.
--
-- ON DELETE SET NULL, matching referral_requests' own precedent for the same shape: an
-- auto_apply_queue row is the candidate's own application-tracking record (approved,
-- declined, or simply abandoned before a decision — it survives until a successful ATS
-- submission deletes it), so losing the CV that was tailored for it must not delete the
-- attempt itself, only forget which CV it was.
ALTER TABLE public.auto_apply_queue
    -- squawk-ignore adding-foreign-key-constraint -- 0116 created this table and nothing on main writes to it yet; the scan and lock this takes are on an empty table
    ADD COLUMN tailored_cv_id uuid REFERENCES public.cvs (id) ON DELETE SET NULL,
    ADD COLUMN reviewed_at timestamptz,
    ADD COLUMN review_decision text
        CONSTRAINT auto_apply_queue_review_decision_check
        CHECK (review_decision IN ('approved', 'declined'));

-- A referencing column needs its own index (0045's own note): Postgres indexes only the
-- referenced side, so without this, deleting a CV scans auto_apply_queue.
-- squawk-ignore require-concurrent-index-creation -- same empty table as above; CONCURRENTLY cannot run inside this file's transaction anyway
CREATE INDEX auto_apply_queue_tailored_cv_id_idx ON public.auto_apply_queue (tailored_cv_id);
