-- migrate: no-transaction
-- Base CVs become visible and editable, so two things the schema never had to say now matter.
--
-- base_cv_id: which base a tailored copy was made from. Until now "the base" was derived on
-- every read (the most recently edited non-tailored CV), which was harmless while nobody could
-- edit a base and wrong the moment someone can: fixing a typo in one base would silently
-- change which base the next tailoring uses. SET NULL, not CASCADE: deleting a base must not
-- delete the copies made from it; they keep working and fall back to the default base.
--
-- hand_edited: the candidate changed this base through the editor. The automatic stale-vs-upload
-- refresh skips such a base, so a new CV upload cannot erase hand edits. A column rather than a
-- scan of cv_revisions because that log is trimmed per CV and a duplicate of an edited base has
-- no history of its own.
--
-- The backfill gives every existing tailored copy the base that is the user's default TODAY
-- (newest-edited non-tailored CV). Without it every existing copy would be NULL, the fallback
-- would decide, and the first hand edit or duplicate in the new editor would silently become
-- the default. Idempotent (base_cv_id IS NULL guarded).
--
-- No-transaction file: NOT VALID and VALIDATE CONSTRAINT in one transaction would hold the
-- validation scan's lock for its whole duration (see 0135). Every statement is idempotent so a
-- half-applied run can simply be repeated.
ALTER TABLE public.cvs ADD COLUMN IF NOT EXISTS base_cv_id uuid;
ALTER TABLE public.cvs ADD COLUMN IF NOT EXISTS hand_edited boolean NOT NULL DEFAULT false;

ALTER TABLE public.cvs
    DROP CONSTRAINT IF EXISTS cvs_base_cv_id_fkey,
    ADD CONSTRAINT cvs_base_cv_id_fkey
        FOREIGN KEY (base_cv_id) REFERENCES public.cvs (id) ON DELETE SET NULL NOT VALID;

ALTER TABLE public.cvs VALIDATE CONSTRAINT cvs_base_cv_id_fkey;

-- A plain CREATE INDEX, not CONCURRENTLY: cvs is small, and CONCURRENTLY waits on every
-- long-running transaction in the database (it stalled a deploy on 2026-11-03, migration 0230).
-- squawk-ignore require-concurrent-index-creation
CREATE INDEX IF NOT EXISTS cvs_base_cv_id_idx ON public.cvs (base_cv_id) WHERE base_cv_id IS NOT NULL;

UPDATE public.cvs t
SET base_cv_id = (
    SELECT b.id
    FROM public.cvs b
    WHERE b.user_id = t.user_id AND NOT b.is_tailored
    ORDER BY b.updated_at DESC, b.id DESC
    LIMIT 1
)
WHERE t.is_tailored AND t.base_cv_id IS NULL;
