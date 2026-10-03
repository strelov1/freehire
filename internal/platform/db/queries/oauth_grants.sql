-- name: CreateOAuthAuthorizationCode :exec
-- Record a freshly minted authorization code after the user approves consent.
-- code_challenge is stored verbatim (S256 of the client's verifier); the token
-- exchange recomputes and compares it, never trusting the client's say-so twice.
INSERT INTO oauth_authorization_codes
    (code_hash, client_id, user_id, redirect_uri, code_challenge, expires_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ConsumeOAuthAuthorizationCode :one
-- Redeem a code for its grant-building fields, deleting it in the same statement
-- so a second exchange with the same code finds no row — a replayed code is
-- reported exactly like an unknown one. expires_at is enforced here rather than
-- left to the caller, for the same reason AuthenticateAPIKey enforces its own
-- expiry: no call site can forget the check.
DELETE FROM oauth_authorization_codes
WHERE code_hash = $1 AND expires_at > now()
RETURNING client_id, user_id, redirect_uri, code_challenge;

-- name: CreateOAuthGrant :one
-- Issue the long-lived access token for an approved client. issued_token_version
-- is the caller's users.token_version at this moment — see the table comment in
-- the migration for how that makes revocation free.
INSERT INTO oauth_grants (user_id, client_id, access_token_hash, issued_token_version, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, created_at, expires_at;

-- name: AuthenticateOAuthGrant :one
-- Resolve a presented bearer token to its owning user, touching last_used_at in
-- the same statement. A grant is live only while BOTH hold: it has not outlived
-- its own expires_at, AND its issued_token_version still matches the user's
-- current one (a mismatch means LogoutAll or a password change ran since). No row
-- covers unknown, expired, and revoked-by-version-bump alike; the caller reads
-- that as 401, same as AuthenticateAPIKey's contract.
UPDATE oauth_grants
SET last_used_at = now()
FROM users
WHERE oauth_grants.access_token_hash = $1
  AND oauth_grants.user_id = users.id
  AND oauth_grants.expires_at > now()
  AND oauth_grants.issued_token_version = users.token_version
RETURNING oauth_grants.user_id;

-- name: ListOAuthGrantsByUser :many
-- A user's connected devices, newest first. Metadata only — never the token
-- hash. Joined to oauth_clients for the display name shown on the page.
SELECT oauth_grants.id, oauth_clients.client_name, oauth_grants.created_at,
       oauth_grants.expires_at, oauth_grants.last_used_at
FROM oauth_grants
JOIN oauth_clients ON oauth_clients.client_id = oauth_grants.client_id
WHERE oauth_grants.user_id = $1
ORDER BY oauth_grants.created_at DESC;

-- name: DeleteOAuthGrant :execrows
-- Revoke one grant, scoped to its owner so a user can only revoke their own.
-- Returns the affected row count: 0 means the id does not exist or is not the
-- caller's (the handler maps that to 404) — same contract as DeleteAPIKey.
DELETE FROM oauth_grants
WHERE id = $1 AND user_id = $2;
