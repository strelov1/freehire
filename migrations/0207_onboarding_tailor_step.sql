-- migrate: no-transaction
-- Adds 'tailor' to the onboarding sequence's step vocabulary: a letter about CV
-- tailoring, the Pro feature, sent last. Unlike the extension letter this one CAN be
-- conditioned on what the database knows — whether the account already pays — and
-- the candidate query does that.
--
-- Outside a transaction, and NOT VALID + a separate VALIDATE, for the reason 0135
-- spells out: the two in one transaction hold the validation scan's lock against
-- readers for as long as the scan takes. The existing rows are all drawn from the
-- previous vocabulary, so the validation cannot fail.

ALTER TABLE public.onboarding_emails
    DROP CONSTRAINT IF EXISTS onboarding_emails_step_check;

ALTER TABLE public.onboarding_emails
    ADD CONSTRAINT onboarding_emails_step_check
        CHECK (step IN ('welcome', 'advanced_search', 'no_alert', 'extension', 'open_source', 'tailor'))
        NOT VALID;

ALTER TABLE public.onboarding_emails
    VALIDATE CONSTRAINT onboarding_emails_step_check;
