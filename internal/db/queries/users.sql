-- name: CreateUser :one
-- Register a new account. email is stored as given (the handler lowercases it);
-- the unique index on lower(email) rejects duplicates regardless of case. role is
-- returned so the new account's wire shape carries it (always 'user' at creation).
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING id, email, role, beta_tester, created_at;

-- name: GetUserByEmail :one
-- Login lookup. Case-insensitive on email; returns password_hash so the handler
-- can verify the password (and reject accounts that have none). role feeds the
-- post-login wire shape.
SELECT id, email, role, beta_tester, password_hash, created_at
FROM users
WHERE lower(email) = lower($1);

-- name: GetUserByID :one
-- Profile lookup for the authenticated user. Never selects password_hash. role is
-- included so /auth/me can tell a client whether to surface moderator-only UI; points is
-- the contribution reward balance shown on the account.
SELECT id, email, role, beta_tester, created_at, points
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
-- Also clears any cached ATS review so a new CV is never scored with a stale one.
UPDATE users
SET resume_object_key = $2, resume_uploaded_at = now(), resume_ats_analysis = NULL
WHERE id = $1;

-- name: ClearUserResume :exec
-- Clear the user's résumé pointer (after deleting the object from storage), any
-- cached ATS review, the derived CV embedding (no CV → no recommendations), and the
-- derived structured résumé (the structure must not outlive the CV it describes).
UPDATE users
SET resume_object_key = NULL, resume_uploaded_at = NULL, resume_ats_analysis = NULL,
    resume_embedding = NULL, resume_embedding_model = NULL,
    resume_structured = NULL, resume_structured_model = NULL,
    resume_structured_uploaded_at = NULL
WHERE id = $1;

-- name: SetUserResumeEmbedding :exec
-- Persist the user's derived CV embedding vector plus the identity of the embedder
-- that produced it (so a model change can mark the vector stale). Never the raw CV text.
UPDATE users
SET resume_embedding = $2, resume_embedding_model = $3
WHERE id = $1;

-- name: GetUserResumeEmbedding :one
-- The user's persisted CV embedding and the embedder identity that produced it, or
-- NULLs when none is stored. The caller ignores a vector whose model no longer matches
-- the current embedder (stale) — see the cv-recommendations change.
SELECT resume_embedding, resume_embedding_model
FROM users
WHERE id = $1;

-- name: GetUserATSAnalysis :one
-- The user's cached CV ATS qualitative review (content-quality + findings), or NULL
-- when none has been computed. Derived only — never the raw CV text.
SELECT resume_ats_analysis
FROM users
WHERE id = $1;

-- name: SetUserATSAnalysis :exec
-- Cache the derived CV ATS review for the user (keyed to their stored CV).
UPDATE users
SET resume_ats_analysis = $2
WHERE id = $1;

-- name: GetUserResumeStructured :one
-- The user's derived structured résumé plus its provenance stamps (the LLM model and
-- the résumé upload time it was derived from), alongside the current résumé upload time
-- so the caller can tell whether the structure still describes the stored CV (served
-- only when resume_structured_uploaded_at equals resume_uploaded_at). NULLs when none.
SELECT resume_structured, resume_structured_model, resume_structured_uploaded_at, resume_uploaded_at
FROM users
WHERE id = $1;

-- name: SetUserResumeStructured :exec
-- Persist the user's derived structured résumé, stamped with the producing LLM model
-- and the résumé upload time it was derived from (passed in, not now(), so the stamp
-- matches the CV the background extraction actually read). Never the raw CV text.
-- The `resume_uploaded_at = $4` guard makes the write monotonic: a slow background
-- extraction for a since-superseded CV (its stamp no longer equals the current upload
-- time) matches no row and is dropped, so a late writer can't clobber a newer CV's
-- structure with an already-stale stamp (which Store.Structured would then hide forever).
UPDATE users
SET resume_structured = $2, resume_structured_model = $3, resume_structured_uploaded_at = $4
WHERE id = $1 AND resume_uploaded_at = $4;

-- name: GetUserRole :one
-- Slim role lookup for the RequireRole authorization middleware: it runs on every
-- request to a role-gated endpoint and needs only the role, so it does not drag the
-- full user row (the GetJobIDBySlug precedent for a hot-path read).
SELECT role
FROM users
WHERE id = $1;

-- name: IsBetaTester :one
-- Slim beta-membership lookup for the RequireModeratorOrBeta middleware — a
-- primitive bool so the auth package stays free of a db import (same shape as GetUserRole).
SELECT beta_tester
FROM users
WHERE id = $1;
