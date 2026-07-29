-- name: CreateAPIKey :one
-- Create an API key for a user. The caller passes the SHA-256 token_hash and the
-- display token_prefix; the plaintext token is shown once and never stored.
-- expires_at NULL means the key never expires. scope confines the credential to a
-- surface ('full' for a user-created key, 'cv' for the tailoring agent's); it comes
-- from the server, never from client input. Returns display fields only, never the hash.
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

-- name: DeleteAPIKey :execrows
-- Revoke (delete) a key, scoped to its owner so a user can only delete their own.
-- Returns the affected row count: 0 means the key does not exist or is not the
-- caller's (the handler maps that to 404).
DELETE FROM api_keys
WHERE id = $1 AND user_id = $2;
