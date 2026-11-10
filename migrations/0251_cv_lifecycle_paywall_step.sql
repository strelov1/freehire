-- migrate: no-transaction
-- Adds step 4 to the CV lifecycle step ledger: the paywall letter, sent about an hour after
-- a free user was refused by the paid-AI gate (paywall_hits, migration 0248). It lives in
-- the same ledger as the three CV letters so one row per (account, step) still means "this
-- letter was claimed, ever", which is what makes it at most once per account, and so the
-- one-letter-a-day report that already reads cv_lifecycle_sends sees it.
--
-- Step 4 is outside the day offsets of steps 1..3: it is due from a paywall hit, not from
-- the CV upload, and nothing about it drops or supersedes the CV letters.
--
-- Outside a transaction, and NOT VALID + a separate VALIDATE, for the reason 0135 spells
-- out. The existing rows are all 1..3, so the validation cannot fail.

ALTER TABLE public.cv_lifecycle_sends
    DROP CONSTRAINT IF EXISTS cv_lifecycle_sends_step_check;

ALTER TABLE public.cv_lifecycle_sends
    ADD CONSTRAINT cv_lifecycle_sends_step_check CHECK (step BETWEEN 1 AND 4) NOT VALID;

ALTER TABLE public.cv_lifecycle_sends
    VALIDATE CONSTRAINT cv_lifecycle_sends_step_check;

COMMENT ON TABLE public.cv_lifecycle_sends IS
    'CV lifecycle letters step ledger. PK (user_id, step) = one claim per pair, ever; error is the transport failure that burned the step. Steps 1-3 are the CV letters, step 4 the paywall letter.';
