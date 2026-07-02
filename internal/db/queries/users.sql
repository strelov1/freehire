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

-- name: GetUserResume :one
-- The authenticated user's résumé pointer (object key + upload time), or NULLs when
-- no résumé is stored. The blob lives in S3 under the key; this is just the pointer.
SELECT resume_object_key, resume_uploaded_at
FROM users
WHERE id = $1;

-- name: SetUserResume :exec
-- Record (or replace) the user's stored-résumé pointer, stamping the upload time.
-- Owner-scoped by id; the object key is derived from the id, never client input.
UPDATE users
SET resume_object_key = $2, resume_uploaded_at = now()
WHERE id = $1;

-- name: ClearUserResume :exec
-- Clear the user's résumé pointer (after deleting the object from storage).
UPDATE users
SET resume_object_key = NULL, resume_uploaded_at = NULL
WHERE id = $1;

-- name: GetUserRole :one
-- Slim role lookup for the RequireRole authorization middleware: it runs on every
-- request to a role-gated endpoint and needs only the role, so it does not drag the
-- full user row (the GetJobIDBySlug precedent for a hot-path read).
SELECT role
FROM users
WHERE id = $1;
