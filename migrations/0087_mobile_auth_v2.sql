-- BE-2 mobile authentication persistence. This migration is expansive and must be
-- applied before deploying a binary that enables AUTH_V2_ENABLED.

ALTER TABLE user_identities
    ADD COLUMN status text NOT NULL DEFAULT 'active',
    ADD COLUMN status_changed_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE user_identities
    ADD CONSTRAINT user_identities_status_check
        CHECK (status IN ('active', 'revocation_pending'));

ALTER TABLE user_identities
    ADD CONSTRAINT user_identities_provider_user_owner_uniq
        UNIQUE (provider, provider_user_id, user_id);

CREATE TABLE oauth_auth_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    state_hash bytea UNIQUE,
    provider text NOT NULL,
    protocol_version smallint NOT NULL DEFAULT 2 CHECK (protocol_version = 2),
    platform text NOT NULL CHECK (platform IN ('ios', 'android', 'web')),
    callback_target text NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('sign_in', 'reauth')),
    code_challenge text,
    code_challenge_method text,
    nonce_challenge text,
    allowed_audiences text[],
    expected_user_id bigint REFERENCES users(id) ON DELETE CASCADE,
    expected_token_version integer,
    expected_session_hash bytea,
    failure_count integer NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    CHECK ((callback_target = 'native_apple') =
           (provider = 'apple' AND nonce_challenge IS NOT NULL AND allowed_audiences IS NOT NULL)),
    CHECK (callback_target <> 'native_apple' OR cardinality(allowed_audiences) > 0),
    CHECK ((callback_target = 'native_apple') OR
           (code_challenge IS NOT NULL AND code_challenge_method = 'S256')),
    CHECK (platform <> 'web' OR purpose = 'reauth'),
    CHECK ((purpose = 'reauth') =
           (expected_user_id IS NOT NULL AND expected_token_version IS NOT NULL
            AND expected_session_hash IS NOT NULL))
);

CREATE INDEX oauth_auth_attempts_expected_user_id_idx
    ON oauth_auth_attempts(expected_user_id);
CREATE INDEX oauth_auth_attempts_expiry_idx
    ON oauth_auth_attempts(expires_at) WHERE consumed_at IS NULL;

CREATE TABLE oauth_exchange_codes (
    code_hash bytea PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_attempt_id uuid NOT NULL UNIQUE REFERENCES oauth_auth_attempts(id) ON DELETE CASCADE,
    code_challenge text NOT NULL,
    code_challenge_method text NOT NULL CHECK (code_challenge_method = 'S256'),
    purpose text NOT NULL CHECK (purpose IN ('sign_in', 'reauth')),
    expected_user_id bigint REFERENCES users(id) ON DELETE CASCADE,
    expected_token_version integer,
    expected_session_hash bytea,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    CHECK ((purpose = 'reauth') =
           (expected_user_id IS NOT NULL AND expected_token_version IS NOT NULL
            AND expected_session_hash IS NOT NULL))
);

CREATE INDEX oauth_exchange_codes_user_id_idx ON oauth_exchange_codes(user_id);
CREATE INDEX oauth_exchange_codes_expected_user_id_idx ON oauth_exchange_codes(expected_user_id);
CREATE INDEX oauth_exchange_codes_expiry_idx
    ON oauth_exchange_codes(expires_at) WHERE consumed_at IS NULL;

CREATE TABLE recent_auth_proofs (
    proof_hash bytea PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_version integer NOT NULL,
    session_hash bytea NOT NULL,
    purpose_class text NOT NULL DEFAULT 'account_security',
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz
);

CREATE INDEX recent_auth_proofs_user_id_idx ON recent_auth_proofs(user_id);
CREATE INDEX recent_auth_proofs_expiry_idx
    ON recent_auth_proofs(expires_at) WHERE consumed_at IS NULL;

CREATE TABLE apple_grants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    provider text NOT NULL DEFAULT 'apple' CHECK (provider = 'apple'),
    provider_user_id text NOT NULL,
    client_id text NOT NULL,
    refresh_token_ciphertext bytea NOT NULL,
    refresh_token_nonce bytea NOT NULL,
    encryption_key_id text NOT NULL,
    aad_version smallint NOT NULL DEFAULT 1 CHECK (aad_version = 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revocation_requested_at timestamptz,
    UNIQUE (provider, provider_user_id),
    FOREIGN KEY (provider, provider_user_id, user_id)
        REFERENCES user_identities(provider, provider_user_id, user_id) ON DELETE RESTRICT
);

CREATE INDEX apple_grants_user_id_idx ON apple_grants(user_id);

CREATE TABLE apple_revocation_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key text NOT NULL UNIQUE,
    reason text NOT NULL CHECK (reason IN ('unlink', 'account_deletion', 'exchange_compensation')),
    source_user_id bigint REFERENCES users(id) ON DELETE SET NULL,
    source_provider text,
    source_provider_user_id text,
    token_aad_row_id uuid NOT NULL,
    client_id text NOT NULL,
    token_ciphertext bytea,
    token_nonce bytea,
    encryption_key_id text,
    aad_version smallint NOT NULL DEFAULT 1 CHECK (aad_version = 1),
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'retry', 'completed', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error_class text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    CHECK ((status = 'completed' AND token_ciphertext IS NULL
            AND token_nonce IS NULL AND encryption_key_id IS NULL) OR
           (status <> 'completed' AND token_ciphertext IS NOT NULL
            AND token_nonce IS NOT NULL AND encryption_key_id IS NOT NULL))
);

CREATE INDEX apple_revocation_jobs_source_user_id_idx
    ON apple_revocation_jobs(source_user_id);
CREATE INDEX apple_revocation_jobs_claim_idx
    ON apple_revocation_jobs(next_attempt_at, created_at)
    WHERE status IN ('pending', 'retry');
