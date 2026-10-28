-- Lets a signed-in member change their account email: a fourth code purpose,
-- 'change_email', and the address that code was mailed to.
--
-- The other purposes confirm the address the account ALREADY holds (or, for
-- verify_work_email, one kept on company_accounts), so the code row never needed to say
-- where it went. A change confirms an address the account does not hold yet, and the
-- confirm step must apply exactly the address the code proved — not whatever the client
-- sends alongside it. Keeping it on the code row welds the two: a new request overwrites
-- both together (the (user_id, purpose) primary key), so a code can never confirm an
-- address it was not mailed to.
--
-- NULL for every other purpose. user_email_codes is small and short-lived (a row lives at
-- most 15 minutes), so the constraint rewrite is effectively instant — see 0176.
ALTER TABLE public.user_email_codes
    ADD COLUMN IF NOT EXISTS target_email text;

ALTER TABLE public.user_email_codes
    DROP CONSTRAINT IF EXISTS user_email_codes_purpose_check;

ALTER TABLE public.user_email_codes
    -- squawk-ignore constraint-missing-not-valid -- table is small/short-lived (see above), so the table-scan lock this rule warns about is effectively instant here
    ADD CONSTRAINT user_email_codes_purpose_check
    CHECK (purpose IN ('verify_email', 'password_reset', 'verify_work_email', 'change_email'));
