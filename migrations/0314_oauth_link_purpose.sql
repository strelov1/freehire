-- A third OAuth v2 purpose, 'link': a signed-in member attaches a sign-in provider to the
-- account they are already in. Until now the only way a provider joined an existing account
-- was sign-in resolving it by a matching verified email; a member whose provider email
-- differs from their account's got a second account instead.
--
-- 'link' carries the session binding exactly like 'reauth' (the exchange refuses a changed
-- session), so every rule that said "reauth" about the binding and the web platform now
-- says "reauth or link". Its exchange code also carries the provider identity the callback
-- verified, which the exchange inserts into user_identities after spending a recent-auth
-- proof (mobileauth.Store.LinkIdentity).
--
-- The constraints being replaced were declared unnamed in 0087, so their names are whatever
-- Postgres chose. They are found by what they say rather than by a guessed name: every
-- CHECK on these two tables that mentions `purpose` is dropped and re-added, named.
--
-- Both tables hold only short-lived rows (minutes; mobileauth.Store.Cleanup deletes the
-- expired ones), so the checks are added and validated in place: the scan is instant.

ALTER TABLE oauth_exchange_codes
    ADD COLUMN link_provider text,
    ADD COLUMN link_provider_user_id text;

DO $$
DECLARE
    c record;
BEGIN
    FOR c IN
        SELECT conrelid::regclass AS tbl, conname
        FROM pg_constraint
        WHERE conrelid IN ('oauth_auth_attempts'::regclass, 'oauth_exchange_codes'::regclass)
          AND contype = 'c'
          AND pg_get_constraintdef(oid) LIKE '%purpose%'
    LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', c.tbl, c.conname);
    END LOOP;
END
$$;

-- Each check is validated in place: see the note at the top on why the scan is instant.
ALTER TABLE oauth_auth_attempts
    -- squawk-ignore constraint-missing-not-valid
    ADD CONSTRAINT oauth_auth_attempts_purpose_check
        CHECK (purpose IN ('sign_in', 'reauth', 'link'));

ALTER TABLE oauth_auth_attempts
    -- squawk-ignore constraint-missing-not-valid
    ADD CONSTRAINT oauth_auth_attempts_web_purpose_check
        CHECK (platform <> 'web' OR purpose IN ('reauth', 'link'));

ALTER TABLE oauth_auth_attempts
    -- squawk-ignore constraint-missing-not-valid
    ADD CONSTRAINT oauth_auth_attempts_binding_check
        CHECK ((purpose IN ('reauth', 'link')) =
               (expected_user_id IS NOT NULL AND expected_token_version IS NOT NULL
                AND expected_session_hash IS NOT NULL));

ALTER TABLE oauth_exchange_codes
    -- squawk-ignore constraint-missing-not-valid
    ADD CONSTRAINT oauth_exchange_codes_purpose_check
        CHECK (purpose IN ('sign_in', 'reauth', 'link'));

ALTER TABLE oauth_exchange_codes
    -- squawk-ignore constraint-missing-not-valid
    ADD CONSTRAINT oauth_exchange_codes_binding_check
        CHECK ((purpose IN ('reauth', 'link')) =
               (expected_user_id IS NOT NULL AND expected_token_version IS NOT NULL
                AND expected_session_hash IS NOT NULL));

ALTER TABLE oauth_exchange_codes
    -- squawk-ignore constraint-missing-not-valid
    ADD CONSTRAINT oauth_exchange_codes_link_identity_check
        CHECK ((purpose = 'link') =
               (link_provider IS NOT NULL AND link_provider_user_id IS NOT NULL));
