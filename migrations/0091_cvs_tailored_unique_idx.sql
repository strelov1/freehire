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
--
-- Because the race this closes is real and has already happened at least once in production,
-- an existing (user_id, job_id) duplicate among is_tailored rows is expected, not hypothetical
-- — CREATE UNIQUE INDEX would otherwise fail outright on such a volume. The block below
-- reconciles any duplicates first: for each (user_id, job_id) group, the row
-- GetTailoredCVForJob already serves today (ORDER BY updated_at DESC, id DESC LIMIT 1) is kept
-- as canonical, and every dependent reference on a loser row is redirected to it rather than
-- dropped — cv_revisions and assistant_sessions cascade-delete on cvs, so deleting a loser
-- outright would silently erase real revision history and strand a live chat session, exactly
-- the harm the underlying race already did once.
-- Plain TEMP tables, not ON COMMIT DROP: this file has no explicit BEGIN/COMMIT of its own
-- (matching every other migration here), and a fresh initdb volume applies it by feeding the
-- whole file to psql, which autocommits each statement as its own transaction absent one — an
-- ON COMMIT DROP table would then vanish right after the statement that created it, before the
-- next statement could read it. Session-scoped is enough: both are gone once this connection
-- closes, and are dropped explicitly below regardless.
CREATE TEMP TABLE cv_tailor_dupes_ranked AS
SELECT id, user_id, job_id,
       row_number() OVER (PARTITION BY user_id, job_id ORDER BY updated_at DESC, id DESC) AS rn
FROM public.cvs
WHERE is_tailored AND job_id IS NOT NULL;

CREATE TEMP TABLE cv_tailor_dupes_canonical AS
SELECT loser.id AS loser_id, canonical.id AS canonical_id
FROM cv_tailor_dupes_ranked loser
JOIN cv_tailor_dupes_ranked canonical
    ON canonical.user_id = loser.user_id
   AND canonical.job_id = loser.job_id
   AND canonical.rn = 1
WHERE loser.rn > 1;

-- Redirect the tailoring conversation itself, so it keeps talking about the CV that survives
-- instead of one about to be deleted.
UPDATE public.assistant_sessions a
SET cv_id = d.canonical_id
FROM cv_tailor_dupes_canonical d
WHERE a.cv_id = d.loser_id;

-- Redirect the loser's edit history onto the canonical row rather than losing it.
UPDATE public.cv_revisions r
SET cv_id = d.canonical_id
FROM cv_tailor_dupes_canonical d
WHERE r.cv_id = d.loser_id;

-- referral_requests.cv_id is ON DELETE SET NULL, so this is not strictly required for the
-- delete below to succeed, but redirecting preserves the reference instead of silently
-- clearing it.
UPDATE public.referral_requests rr
SET cv_id = d.canonical_id
FROM cv_tailor_dupes_canonical d
WHERE rr.cv_id = d.loser_id;

-- cv_tracer_links carries its own UNIQUE (cv_id, source_path, destination_hash): two duplicate
-- tailored copies of the same document plausibly minted an identical link at the same
-- source_path, so redirecting every loser link would collide with the canonical's own. Drop
-- only the ones that would collide; the rest redirect same as everything else above.
DELETE FROM public.cv_tracer_links t
USING cv_tailor_dupes_canonical d
WHERE t.cv_id = d.loser_id
  AND EXISTS (
      SELECT 1 FROM public.cv_tracer_links existing
      WHERE existing.cv_id = d.canonical_id
        AND existing.source_path = t.source_path
        AND existing.destination_hash = t.destination_hash
  );

UPDATE public.cv_tracer_links t
SET cv_id = d.canonical_id
FROM cv_tailor_dupes_canonical d
WHERE t.cv_id = d.loser_id;

-- Every dependent reference now points at the canonical row; the losers are safe to drop.
DELETE FROM public.cvs c
USING cv_tailor_dupes_canonical d
WHERE c.id = d.loser_id;

DROP TABLE cv_tailor_dupes_canonical;
DROP TABLE cv_tailor_dupes_ranked;

-- APPLY TO PROD MANUALLY BEFORE DEPLOY (SET ROLE hire), per this repo's convention for a
-- migration that writes existing rows rather than only adding schema — see 0045/0053's own
-- notes. This one in particular deletes rows and redirects foreign keys; run it against a
-- current backup or during a maintenance window, not casually.
CREATE UNIQUE INDEX IF NOT EXISTS cvs_user_id_job_id_tailored_uniq_idx
    ON public.cvs (user_id, job_id) WHERE is_tailored;
