-- name: CreateUser :one
-- Register a new account. email is stored as given (the handler lowercases it);
-- the unique index on lower(email) rejects duplicates regardless of case. role is
-- returned so the new account's wire shape carries it (always 'user' at creation).
-- email_verified is a parameter, not a default: a password registration starts
-- unverified, while an account created from an OAuth sign-in is verified at birth
-- because the provider already proved the address.
INSERT INTO users (email, password_hash, email_verified)
VALUES ($1, $2, $3)
RETURNING id, email, role, beta_tester, email_verified, created_at;

-- name: GetUserByEmail :one
-- Login lookup. Case-insensitive on email; returns password_hash so the handler
-- can verify the password (and reject accounts that have none). role feeds the
-- post-login wire shape. email_verified drives both the "confirm your email" prompt
-- and the OAuth merge policy (an unverified account is seized, not silently joined).
SELECT id, email, role, beta_tester, email_verified, password_hash, created_at
FROM users
WHERE lower(email) = lower($1);

-- name: GetUserByID :one
-- Profile lookup for the authenticated user. role is included so /auth/me can tell a
-- client whether to surface moderator-only UI. The password hash itself never leaves
-- the database — only whether one exists, which is what lets the SPA offer a password
-- change to password accounts and explain itself to OAuth-only ones.
SELECT id, email, role, beta_tester, email_verified,
       (password_hash IS NOT NULL)::boolean AS has_password,
       created_at
FROM users
WHERE id = $1;

-- name: GetUserPasswordHash :one
-- The account's stored password hash, for verifying a current password on change.
-- NULL when the account is passwordless (OAuth-only), which the caller treats the same
-- as a wrong password: there is nothing to verify against.
SELECT password_hash
FROM users
WHERE id = $1;

-- name: GetUserTokenVersion :one
-- The account's current session generation, read on every authenticated request to
-- decide whether a correctly-signed token was revoked. One primary-key lookup; this
-- is the price of making a stateless JWT revocable at all.
SELECT token_version
FROM users
WHERE id = $1;

-- name: BumpUserTokenVersion :one
-- Invalidate every token issued for this account (sign out everywhere) by advancing
-- the generation, returning the new value so the caller can immediately mint a
-- replacement session for whoever asked.
UPDATE users
SET token_version = token_version + 1
WHERE id = $1
RETURNING token_version;

-- name: SetUserEmailVerified :exec
-- Record that control of the address was proven. Idempotent — confirming twice is a
-- no-op rather than an error, so a double-submitted code does not fail the request.
UPDATE users
SET email_verified = true
WHERE id = $1;

-- name: SetUserPassword :one
-- Change a known password. Revokes every other session in the same statement, so a
-- stolen token cannot outlive the password it was minted under. Does NOT touch
-- email_verified: knowing the current password proves nothing about the address.
UPDATE users
SET password_hash = $2, token_version = token_version + 1
WHERE id = $1
RETURNING token_version;

-- name: ResetUserPassword :one
-- Complete a reset-by-emailed-code: set the new hash, revoke every session, and mark
-- the address verified — receiving the code IS proof of control, so a reset doubles as
-- verification and an unverified account stops being a merge target.
--
-- The account's API keys are destroyed in the same statement. A reset is how an owner
-- takes an account back, so whoever knew the old password must lose every credential
-- minted under it — and a key authenticates against api_keys alone, so bumping
-- token_version does not reach it. The DELETE is a data-modifying CTE rather than a
-- second statement on purpose: Postgres runs it exactly once and to completion whether
-- or not the primary query reads its output, which welds the revocation to the reset so
-- no call site can perform one without the other.
WITH revoked_keys AS (
    DELETE FROM api_keys WHERE user_id = $1
)
UPDATE users
SET password_hash = $2, email_verified = true, token_version = token_version + 1
WHERE users.id = $1
RETURNING token_version;

-- name: SeizeUnverifiedAccount :one
-- Hand an unverified, password-backed account to the proven owner of its address when a
-- provider-verified OAuth identity arrives for it: the password is destroyed, every
-- session revoked, and every API key deleted, so a squatter who registered the address
-- first loses all three ways in. The keys matter most of the three: they are the only
-- credential that survives a token_version bump (a key is matched against api_keys by
-- its own hash and never consults users), and one minted with expires_at NULL would
-- otherwise outlive the takeover forever. Same data-modifying-CTE reasoning as
-- ResetUserPassword — the revocation cannot be separated from the seizure.
-- Returns the new generation for the session the sign-in is about to mint.
WITH revoked_keys AS (
    DELETE FROM api_keys WHERE user_id = $1
)
UPDATE users
SET password_hash = NULL, email_verified = true, token_version = token_version + 1
WHERE users.id = $1
RETURNING token_version;

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

-- name: ListUserBlobKeys :many
-- Every object-storage key the account owns, in one read: the stored CV, each
-- referral-proof PDF, and the raw MIME of each hosted email. Account deletion
-- collects these BEFORE deleting any row — the mail and proof keys live in the rows
-- themselves, so once those are gone the objects are unreachable and would sit in
-- the bucket forever. Empty keys are filtered out so a caller never asks storage to
-- delete "".
SELECT u.resume_object_key AS key FROM users u
WHERE u.id = $1 AND u.resume_object_key IS NOT NULL AND u.resume_object_key <> ''
UNION
SELECT o.proof_object_key FROM referral_offers o
WHERE o.user_id = $1 AND o.proof_object_key <> ''
UNION
SELECT e.s3_key FROM emails e
WHERE e.user_id = $1 AND e.s3_key IS NOT NULL AND e.s3_key <> '';

-- name: DeleteUser :exec
-- Erase the account. Every user-owned table declares ON DELETE CASCADE, so this one
-- statement is the whole database side of account deletion; the trails that outlive
-- the member (jobs.created_by, *.reviewed_by, referral decisions, thread authorship)
-- are ON DELETE SET NULL by design. Objects in storage are NOT reachable from here —
-- the caller deletes them first (see ListUserBlobKeys).
DELETE FROM users WHERE id = $1;

-- name: UserEmail :one
-- Slim email lookup for the delete-account confirmation, which compares the typed
-- address against the caller's own. A primitive so the handler needs no full user row
-- (same shape as GetUserRole).
SELECT email
FROM users
WHERE id = $1;
