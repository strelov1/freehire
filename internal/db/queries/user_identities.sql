-- name: GetUserByIdentity :one
-- OAuth sign-in fast path: resolve a provider identity straight to its user.
SELECT u.id, u.email, u.created_at
FROM user_identities ui
JOIN users u ON u.id = ui.user_id
WHERE ui.provider = $1 AND ui.provider_user_id = $2;

-- name: GetActiveUserByIdentity :one
-- Authentication accepts only active identities. A pending Apple revocation
-- must not sign the user back in and silently cancel an unlink request.
SELECT u.id, u.email, u.created_at
FROM user_identities ui
JOIN users u ON u.id = ui.user_id
WHERE ui.provider = $1 AND ui.provider_user_id = $2
  AND ui.status = 'active';

-- name: CreateUserIdentity :exec
-- Link a provider identity to an account (first OAuth sign-in). The composite
-- primary key rejects a duplicate identity.
INSERT INTO user_identities (provider, provider_user_id, user_id)
VALUES ($1, $2, $3);
