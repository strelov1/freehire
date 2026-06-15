-- name: CreateUser :one
-- Register a new account. email is stored as given (the handler lowercases it);
-- the unique index on lower(email) rejects duplicates regardless of case. role is
-- returned so the new account's wire shape carries it (always 'user' at creation).
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING id, email, role, created_at;

-- name: GetUserByEmail :one
-- Login lookup. Case-insensitive on email; returns password_hash so the handler
-- can verify the password (and reject accounts that have none). role feeds the
-- post-login wire shape.
SELECT id, email, role, password_hash, created_at
FROM users
WHERE lower(email) = lower($1);

-- name: GetUserByID :one
-- Profile lookup for the authenticated user. Never selects password_hash. role is
-- included so /auth/me can tell a client whether to surface moderator-only UI.
SELECT id, email, role, created_at
FROM users
WHERE id = $1;

-- name: GetUserRole :one
-- Slim role lookup for the RequireRole authorization middleware: it runs on every
-- request to a role-gated endpoint and needs only the role, so it does not drag the
-- full user row (the GetJobIDBySlug precedent for a hot-path read).
SELECT role
FROM users
WHERE id = $1;
