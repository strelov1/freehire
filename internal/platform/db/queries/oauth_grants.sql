-- name: CreateOAuthAuthorizationCode :exec
-- Record a freshly minted authorization code after the user approves consent.
-- code_challenge is stored verbatim (S256 of the client's verifier); the token
-- exchange recomputes and compares it, never trusting the client's say-so twice.
-- issued_token_version is the generation the approving SESSION was minted under
-- (the tv claim, already validated against the account at request entry) — never
-- a fresh read of users.token_version, so a revocation landing between the
-- session check and this insert cannot capture its new generation. 0 is the
-- unknown-generation sentinel (generations start at 1): dead on arrival, the
-- fail-closed shape legacy rows take after 0233. NULL — what the old binary
-- writes during rollout — never satisfies the exchange's equality latch either.
-- squawk-ignore prefer-bigint-over-int -- mirrors users.token_version (int, 0041_account_security.sql), which this column is compared against
INSERT INTO oauth_authorization_codes
    (code_hash, client_id, user_id, redirect_uri, code_challenge, issued_token_version, expires_at, resource)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ConsumeOAuthAuthorizationCode :one
-- Redeem a code for its grant-building fields, deleting it in the same statement
-- so a second exchange with the same code finds no row — a replayed code is
-- reported exactly like an unknown one. expires_at is enforced here rather than
-- left to the caller, for the same reason AuthenticateAPIKey enforces its own
-- expiry: no call site can forget the check.
--
-- The generation latch is part of the redemption, not a check beside it: the
-- code's issued_token_version must equal the account's CURRENT
-- users.token_version (the same comparison AuthenticateOAuthGrant applies to
-- grants), and the row is deleted ONLY when it holds. A code left pending over
-- a revocation is therefore refused AND left standing dead — no retry revives
-- it, and it never upgrades into a grant of the new generation. 0 (the legacy
-- sentinel) and NULL (old-binary rollout writes) never match a live account,
-- so those codes are dead for the same reason.
DELETE FROM oauth_authorization_codes c
USING users u
WHERE c.code_hash = $1
  AND c.expires_at > now()
  AND c.user_id = u.id
  AND c.issued_token_version = u.token_version
RETURNING c.client_id, c.user_id, c.redirect_uri, c.code_challenge,
          c.issued_token_version, c.resource;

-- name: CreateOAuthGrant :one
-- Issue the long-lived access token for an approved client. issued_token_version
-- is the caller's users.token_version at this moment — see the table comment in
-- the migration for how that makes revocation free.
-- resource is the RFC 8707 MCP resource the grant is bound to; NULL is unbound
-- and valid on every MCP endpoint.
INSERT INTO oauth_grants (user_id, client_id, access_token_hash, issued_token_version, expires_at, resource)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at, expires_at;

-- name: AuthenticateOAuthGrant :one
-- Resolve a presented bearer token to its owning user, touching last_used_at in
-- the same statement. A grant is live only while BOTH hold: it has not outlived
-- its own expires_at, AND its issued_token_version still matches the user's
-- current one (a mismatch means LogoutAll or a password change ran since). No row
-- covers unknown, expired, and revoked-by-version-bump alike; the caller reads
-- that as 401, same as AuthenticateAPIKey's contract. It also keeps
-- users.last_active_at current, as AuthenticateAPIKey does.
--
-- endpoint is the MCP resource PATH the caller serves, or NULL for a caller that
-- is not an MCP endpoint (REST). A grant bound to a different path does not
-- match, so a wrong-endpoint token is refused like an unknown one and never
-- touches last_used_at / last_active_at; an unbound grant (resource NULL) matches
-- every endpoint.
WITH g AS (
    UPDATE oauth_grants
    SET last_used_at = now()
    FROM users
    WHERE oauth_grants.access_token_hash = sqlc.arg('access_token_hash')
      AND oauth_grants.user_id = users.id
      AND oauth_grants.expires_at > now()
      AND oauth_grants.issued_token_version = users.token_version
      AND (sqlc.narg('endpoint')::text IS NULL
           OR oauth_grants.resource IS NULL
           OR oauth_grants.resource = sqlc.narg('endpoint')::text)
    RETURNING oauth_grants.user_id, oauth_grants.resource
), seen AS (
    UPDATE users AS su SET last_active_at = now()
    FROM g
    WHERE su.id = g.user_id AND su.last_active_at < now() - interval '1 day'
)
SELECT g.user_id, g.resource FROM g;

-- name: ResolveOAuthGrantIdentity :one
-- Read-only credential identity and entitlements with the same liveness rules as
-- AuthenticateOAuthGrant, including the account's current session generation.
SELECT g.user_id, g.expires_at, u.api_until_stripe, u.api_data_until_stripe, u.pro_until, u.ultra_until
FROM oauth_grants g
JOIN users u ON u.id = g.user_id
WHERE g.access_token_hash = $1
  AND g.expires_at > now()
  AND g.issued_token_version = u.token_version;

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

-- name: DeleteOAuthGrantsByUser :exec
-- Wipe every MCP grant a user holds. A password reset is exactly the moment a
-- user suspects someone else is signed in — the same reasoning ResetPassword
-- already applies to every session cookie — so it deletes the rows outright
-- rather than leaving them to the issued_token_version/users.token_version
-- mismatch alone: that comparison already stops a stale grant authenticating,
-- but a deleted row is also what "Connected devices" stops listing, and what a
-- reset should visibly answer for.
DELETE FROM oauth_grants
WHERE user_id = $1;
