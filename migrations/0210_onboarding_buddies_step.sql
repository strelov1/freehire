-- migrate: no-transaction
-- Adds 'buddies' to the onboarding sequence's step vocabulary: a letter about
-- interview buddies, the free anonymous peer network, sent last, on day 14. Like the
-- tailor letter it is conditioned on what the database knows — whether the account
-- already joined the network, and whether the one-off broadcast already pitched it —
-- and the candidate query does that.
--
-- Outside a transaction, and NOT VALID + a separate VALIDATE, for the reason 0135
-- spells out: the two in one transaction hold the validation scan's lock against
-- readers for as long as the scan takes. The existing rows are all drawn from the
-- previous vocabulary, so the validation cannot fail.

ALTER TABLE public.onboarding_emails
    DROP CONSTRAINT IF EXISTS onboarding_emails_step_check;

ALTER TABLE public.onboarding_emails
    ADD CONSTRAINT onboarding_emails_step_check
        CHECK (step IN ('welcome', 'advanced_search', 'no_alert', 'extension', 'open_source', 'tailor', 'buddies'))
        NOT VALID;

ALTER TABLE public.onboarding_emails
    VALIDATE CONSTRAINT onboarding_emails_step_check;
