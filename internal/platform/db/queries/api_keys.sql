-- name: CreateAPIKey :one
-- Create an API key for a user. The caller passes the SHA-256 token_hash and the
-- display token_prefix; the plaintext token is shown once and never stored.
-- expires_at NULL means the key never expires. scope confines the credential to a
-- surface: 'full' is the default for a user-created key; 'cv' is server-only, never
-- client-requestable (reserved for the tailoring agent); 'embed' is the one scope a
-- client may request for itself, for the separate volunteer-embed-mvp service — the
-- handler validates which scopes a client may request before this query ever runs.
-- Returns display fields only, never the hash.
--
-- The INSERT ... SELECT is the verified-address gate, not a style choice: registration
-- hands out a session before the address is proven, so a squatter on someone else's
-- email could otherwise mint a never-expiring, full-scope bearer credential and keep it
-- past the seizure that hands the account to its real owner. Putting the condition in
-- the statement means no call site can mint around it. Inserting nothing yields no row,
-- which the caller reads as "not allowed" (403).
INSERT INTO api_keys (user_id, name, token_hash, token_prefix, expires_at, scope)
SELECT $1, $2, $3, $4, $5, $6
FROM users
WHERE users.id = $1 AND users.email_verified
RETURNING id, name, token_prefix, scope, created_at, last_used_at, expires_at;

-- name: ListAPIKeysByUser :many
-- A user's API keys, newest first. Metadata only — never the token_hash. scope is
-- included so a user can see what each credential is allowed to do.
SELECT id, name, token_prefix, scope, created_at, last_used_at, expires_at
FROM api_keys
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: AuthenticateAPIKey :one
-- Resolve a presented token (by its SHA-256 hash) to the owning user id and the key's
-- scope, enforcing expiry and touching last_used_at in one atomic statement. No row
-- means the key is unknown, revoked, or expired; the caller treats pgx.ErrNoRows as 401
-- and an insufficient scope as 403.
UPDATE api_keys
SET last_used_at = now()
WHERE token_hash = $1
  AND (expires_at IS NULL OR expires_at > now())
RETURNING user_id, scope;

-- name: ResolveAPIKeyTier :one
-- Resolve a presented token to its owner, the key's scope and how far the owner's API
-- subscription reaches, for the public-read rate limiter. Read-only on purpose: unlike
-- AuthenticateAPIKey it does NOT touch last_used_at, because it runs on the hottest reads on
-- the site and its answer is cached. No row means unknown, revoked or expired.
SELECT k.user_id, k.scope, u.api_until_stripe, u.api_data_until_stripe
FROM api_keys k
JOIN users u ON u.id = k.user_id
WHERE k.token_hash = $1
  AND (k.expires_at IS NULL OR k.expires_at > now());

-- name: GetUserAPIEntitlement :one
-- How far the account's API product subscriptions reach, for its own key-management page.
SELECT api_until_stripe, api_data_until_stripe
FROM users
WHERE id = $1;

-- name: DeleteAPIKey :execrows
-- Revoke (delete) a key, scoped to its owner so a user can only delete their own.
-- Returns the affected row count: 0 means the key does not exist or is not the
-- caller's (the handler maps that to 404).
DELETE FROM api_keys
WHERE id = $1 AND user_id = $2;

-- name: UserHasAgentCredential :one
-- Whether the account holds a live credential an agent connects with: an API key (any
-- scope but `embed`, which grants nothing on this API) or an MCP OAuth grant the
-- account's sessions have not revoked. Existence, not use: a key read only by the
-- public-read limiter never touches last_used_at, so "used" would miss a CLI that only
-- searches. Read by the site's "connect your agent" strip to stop asking.
SELECT (EXISTS (
           SELECT 1 FROM api_keys k
           WHERE k.user_id = $1
             AND k.scope <> 'embed'
             AND (k.expires_at IS NULL OR k.expires_at > now())
       )
    OR EXISTS (
           SELECT 1 FROM oauth_grants g
           JOIN users u ON u.id = g.user_id
           WHERE g.user_id = $1
             AND g.expires_at > now()
             AND g.issued_token_version = u.token_version
       ))::boolean AS connected;
