-- name: RegisterOAuthClient :one
-- Dynamic client registration (RFC 7591). client_id is server-generated, random,
-- and not a secret — it identifies the client, it does not authenticate it (PKCE
-- does that). Open to any caller: the security boundary is the user's own consent
-- click on the authorize screen, not who may register a client_id.
INSERT INTO oauth_clients (client_id, client_name, redirect_uris)
VALUES ($1, $2, $3)
RETURNING client_id, client_name, redirect_uris, created_at;

-- name: GetOAuthClient :one
-- Resolve a client_id presented at /authorize or /token. No row means the
-- authorize request names a client that was never registered.
SELECT client_id, client_name, redirect_uris, created_at
FROM oauth_clients
WHERE client_id = $1;
