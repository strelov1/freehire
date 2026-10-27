-- Gives the onboarding tour back to the accounts that lost it to a race at sign-in.
--
-- From the release of 0207 (2026-10-27 12:33 UTC) until this fix, signing in through
-- /signin fired the layout's onboarding redirect while the sign-in page was navigating to
-- its own returnTo. The tour opened for an instant, marked the account complete — the
-- tour records itself as shown on arrival — and the sign-in page's navigation took the
-- visitor away. 18 accounts on 2026-10-27.
--
-- Re-asking an account that DID see the tour costs nothing: the tour skips every step the
-- account has answered, and an account with nothing left to ask is marked complete again
-- without a screen.
UPDATE public.users
SET onboarding_completed_at = NULL
WHERE onboarding_completed_at >= '2026-10-27 12:33:00+00';
