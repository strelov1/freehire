-- The onboarding tour (openspec/changes/onboarding-tour).
--
-- 1. The challenge question takes several answers. `biggest_challenge` was single-valued so
--    the survey learned the ONE biggest difficulty; the tour instead shows what helps with
--    each thing in the way, and a candidate held back by both visas and language should
--    be shown both. Members of vocab.JobChallengeValues, validated in Go like every survey
--    enum (0134), in the order picked. Nullable, and null means unstated — an empty array
--    is folded into null before it is ever written.
--
--    `biggest_challenge` is superseded and no longer written. It stays rather than being
--    dropped in the same change that stops writing it, so a rollback of the code still
--    finds the column it reads.
ALTER TABLE public.candidate_survey
    ADD COLUMN biggest_challenges text[];

-- Every stored single answer becomes a one-element list, so nobody who answered is asked
-- again for what they already told us. ~10.5k rows on 2026-10-26, so one statement.
UPDATE public.candidate_survey
SET biggest_challenges = ARRAY[biggest_challenge]
WHERE biggest_challenge IS NOT NULL;

-- 2. Every account goes through onboarding once more. 0134 left this exact seam ("re-asking
--    everyone when a future wave of questions lands ... is one more migration"). The tour
--    itself skips what an account has already answered and marks an account with nothing
--    left to ask complete without showing a screen, so this re-asks only for what is
--    missing.
UPDATE public.users
SET onboarding_completed_at = NULL
WHERE onboarding_completed_at IS NOT NULL;
