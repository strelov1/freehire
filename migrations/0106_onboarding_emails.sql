-- onboarding_emails is the send ledger for the signup sequence: three mails from
-- the founder, sent over a new account's first days.
--
-- The primary key is (user_id, step). That is the whole idempotency story: one row
-- per pair, ever. A row is written even when the send failed (with the error kept in
-- `error`), because a person who was mailed a broken greeting is not helped by being
-- mailed it again on the next pass — and an unbounded retry against a bouncing
-- address is how a sending domain gets its reputation burned. Re-arming a single
-- (user, step) is a deliberate DELETE, not an automatic retry.
--
-- Deliberately simpler than application_nudges (0083): no claim lease, no attempt
-- counter, no dead-letter state. Those exist there because a lifecycle nudge races
-- a changing application; nothing here races anything — eligibility is a function of
-- the account's age and whether it ever created an alert, and neither can flip back.

CREATE TABLE public.onboarding_emails (
    user_id bigint NOT NULL,
    step text NOT NULL,
    sent_at timestamp with time zone DEFAULT now() NOT NULL,
    error text NOT NULL DEFAULT '',
    CONSTRAINT onboarding_emails_pkey PRIMARY KEY (user_id, step),
    CONSTRAINT onboarding_emails_step_check CHECK (step IN ('welcome', 'no_alert', 'open_source')),
    CONSTRAINT onboarding_emails_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES public.users(id) ON DELETE CASCADE
);

COMMENT ON TABLE public.onboarding_emails IS
    'Send ledger for the founder signup sequence. PK (user_id, step) = one send per pair, ever.';
COMMENT ON COLUMN public.onboarding_emails.error IS
    'Transport error if the send failed. The row still blocks a retry; re-arm by deleting it.';
