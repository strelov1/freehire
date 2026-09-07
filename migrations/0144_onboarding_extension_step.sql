-- migrate: no-transaction
-- Adds 'extension' to the onboarding sequence's step vocabulary: a letter about the
-- browser extension, which sits between the no-alert nudge and the open-source
-- letter. It goes to everyone rather than only to people without the extension —
-- nothing in this database records whether an account has installed it, and a
-- letter conditioned on a fact we cannot read would be a guess wearing a WHERE
-- clause.
--
-- Outside a transaction, and NOT VALID + a separate VALIDATE, for the reason 0135
-- spells out at length: the two in one transaction hold the validation scan's lock
-- against readers for as long as the scan takes, which is the anti-pattern the split
-- exists to avoid. The existing rows are all drawn from the previous vocabulary, so
-- the validation cannot fail.

ALTER TABLE public.onboarding_emails
    DROP CONSTRAINT IF EXISTS onboarding_emails_step_check;

ALTER TABLE public.onboarding_emails
    ADD CONSTRAINT onboarding_emails_step_check
        CHECK (step IN ('welcome', 'advanced_search', 'no_alert', 'extension', 'open_source'))
        NOT VALID;

ALTER TABLE public.onboarding_emails
    VALIDATE CONSTRAINT onboarding_emails_step_check;
