-- Widens user_email_codes.purpose's CHECK to admit 'verify_work_email' — the purpose
-- internal/identity/accounts.PurposeVerifyWorkEmail names for a company-account claim's
-- work-email code (internal/ingest/employer, see migration 0174). The generic
-- Service.IssueCode/ConfirmCode added alongside that purpose (codes.go) share this same
-- table and constraint with the two purposes already here — "at most one outstanding code
-- per (user, purpose)" is exactly why a THIRD purpose needs to be a THIRD allowed value
-- here, not a parallel table.
--
-- user_email_codes is small and short-lived (a code's row lives at most codeTTL, 15
-- minutes, before it is consumed or replaced) — unlike jobs.closed_reason's widenings,
-- there is no large-table remedy to apply here; a plain rewrite is effectively instant.
ALTER TABLE public.user_email_codes
    DROP CONSTRAINT IF EXISTS user_email_codes_purpose_check;

ALTER TABLE public.user_email_codes
    -- squawk-ignore constraint-missing-not-valid -- table is small/short-lived (see above), so the table-scan lock this rule warns about is effectively instant here
    ADD CONSTRAINT user_email_codes_purpose_check
    CHECK (purpose IN ('verify_email', 'password_reset', 'verify_work_email'));
