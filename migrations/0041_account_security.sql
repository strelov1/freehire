-- Account-security hardening: proven email ownership, revocable sessions, scoped API keys.
--
-- email_verified records whether control of the address was ever proven (by an emailed code
-- or by a provider asserting it). Registration no longer implies ownership, so an unverified
-- password account can never be a silent OAuth merge target. Every EXISTING row is
-- grandfathered verified: forcing the whole user base through a code prompt at deploy is a
-- larger, certain harm than the speculative pre-hijack it would catch.
--
-- token_version makes the stateless JWT revocable: the claim rides the token and is compared
-- against this column on every authenticated request, so logout-everywhere, a password change,
-- a reset, and an account seizure all invalidate outstanding tokens. It starts at 1, not 0, so
-- a token minted before this change — which carries no claim, decoding to zero — can never
-- match. That is the one-time forced sign-out the release documents.
--
-- user_email_codes holds the short-lived six-digit codes. The composite primary key IS the
-- "at most one outstanding code per purpose" rule: a resend is an upsert, so no sweeper is
-- needed and a stale code cannot coexist with a fresh one. Codes are bcrypt-hashed by the
-- application; a stolen snapshot must not yield live codes.
--
-- api_keys.scope confines a credential to the surface it was minted for. Existing keys default
-- to 'full', so current integrations are unaffected; the CV-tailoring key is minted 'cv'.
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: initdb runs migrations only on first volume init, so
-- on a persistent volume this does not auto-apply. The new binary SELECTs these columns on
-- every authenticated request, so deploying first makes them fail with 42703 (undefined
-- column) → 500 across the whole authenticated surface. Run it first (same as 0005-0010,
-- 0039, 0040).

ALTER TABLE public.users
    ADD COLUMN email_verified boolean NOT NULL DEFAULT false,
    ADD COLUMN token_version  integer NOT NULL DEFAULT 1;

-- Grandfather every account that existed before ownership was ever asked for.
UPDATE public.users SET email_verified = true;

ALTER TABLE public.api_keys
    ADD COLUMN scope text NOT NULL DEFAULT 'full',
    ADD CONSTRAINT api_keys_scope_check CHECK (scope IN ('full', 'cv'));

CREATE TABLE public.user_email_codes (
    user_id    bigint      NOT NULL REFERENCES public.users (id) ON DELETE CASCADE,
    purpose    text        NOT NULL CHECK (purpose IN ('verify_email', 'password_reset')),
    code_hash  text        NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    attempts   integer     NOT NULL DEFAULT 0,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, purpose)
);
