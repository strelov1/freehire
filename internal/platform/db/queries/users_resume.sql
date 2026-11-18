-- name: GetUserResume :one
-- The authenticated user's résumé pointer (object key + upload time), or NULLs when
-- no résumé is stored. The blob lives in S3 under the key; this is just the pointer.
SELECT resume_object_key, resume_uploaded_at
FROM users
WHERE id = $1;

-- name: LockUserResumeUpload :exec
-- Serializes one user's CV pointer writes (uploads and deletes) for the transaction, so a
-- first-CV-only upload's presence check and pointer write are one step. Never held across
-- the object upload itself.
SELECT pg_advisory_xact_lock(hashtextextended('resume_upload:' || sqlc.arg(user_id)::bigint::text, 0));

-- name: SetUserResume :exec
-- Record (or replace) the user's stored-résumé pointer, stamping the upload time.
-- Owner-scoped by id; the object key is derived from the id, never client input.
-- Also clears any cached ATS review so a new CV is never scored with a stale one,
-- and marks structured extract pending for this upload (background work must catch up).
UPDATE users u
SET resume_object_key = $2,
    resume_uploaded_at = t.ts,
    resume_ats_analysis = NULL,
    resume_extract_status = 'pending',
    resume_extract_detail = NULL,
    resume_extract_for = t.ts
FROM (SELECT now() AS ts) t
WHERE u.id = $1;

-- name: ClearUserResume :exec
-- Clear the user's résumé pointer (after deleting the object from storage), any
-- cached ATS review, the derived CV embedding (no CV → no recommendations), the
-- derived structured résumé (the structure must not outlive the CV it describes), and
-- the geography derived from that structure (which must not outlive it either — a
-- country left behind here would keep answering "where is this candidate" from a CV
-- that no longer exists). Candidate contacts are intentionally kept: they are
-- owner-edited identity, not an extract artifact.
UPDATE users
SET resume_object_key = NULL, resume_uploaded_at = NULL, resume_ats_analysis = NULL,
    resume_embedding = NULL, resume_embedding_model = NULL,
    resume_structured = NULL, resume_structured_model = NULL,
    resume_structured_uploaded_at = NULL,
    resume_countries = NULL, resume_regions = NULL, resume_cities = NULL,
    resume_extract_status = NULL, resume_extract_detail = NULL, resume_extract_for = NULL
WHERE id = $1;

-- name: GetUserPhoto :one
-- The authenticated user's headshot pointer (object key + upload time), or NULLs when
-- no headshot is stored. The image lives in S3 under the key; this is just the pointer.
SELECT photo_object_key, photo_uploaded_at
FROM users
WHERE id = $1;

-- name: SetUserPhoto :exec
-- Record (or replace) the user's headshot pointer, stamping the upload time. Owner-scoped
-- by id; the object key is derived from the id, never client input. Nothing derived hangs
-- off the image, so — unlike SetUserResume — there is no cached artefact to invalidate.
UPDATE users
SET photo_object_key = $2, photo_uploaded_at = now()
WHERE id = $1;

-- name: ClearUserPhoto :exec
-- Clear the user's headshot pointer, after deleting the object from storage.
UPDATE users
SET photo_object_key = NULL, photo_uploaded_at = NULL
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
-- Also returns candidate contacts and last extract status for Profile / seed composition.
SELECT resume_structured, resume_structured_model, resume_structured_uploaded_at, resume_uploaded_at,
       candidate_contacts, resume_extract_status, resume_extract_detail, resume_extract_for
FROM users
WHERE id = $1;

-- name: GetUserResumeStructuredOnly :one
-- Just the structured résumé, with none of the provenance stamps and none of the
-- contacts GetUserResumeStructured returns beside it.
--
-- It exists so the Talent Network's handle mint can read one job title without also
-- holding the candidate's phone number and email in memory. The stamp is deliberately
-- not applied here: a handle derived from a slightly stale title is still a fine
-- handle — it is frozen at mint and opaque afterwards — whereas refusing to mint over
-- an in-flight extraction would block the join itself.
SELECT resume_structured
FROM users
WHERE id = $1;

-- name: SetUserResumeStructured :execrows
-- Persist the user's derived structured résumé, stamped with the producing LLM model
-- and the résumé upload time it was derived from (passed in, not now(), so the stamp
-- matches the CV the background extraction actually read). Never the raw CV text.
-- The `resume_uploaded_at = $4` guard makes the write monotonic: a slow background
-- extraction for a since-superseded CV (its stamp no longer equals the current upload
-- time) matches no row and is dropped, so a late writer can't clobber a newer CV's
-- structure with an already-stale stamp (which Store.Structured would then hide forever).
--
-- The candidate's geography rides in this same statement rather than a second one, so it
-- inherits that guard for free and can never end up describing a different CV than the
-- structure it was derived from. It is deterministic and costs no I/O, so there is
-- nothing to gain by deferring it — and a separate write would have to duplicate the
-- guard, which is exactly how invariants drift apart.
-- On success, extract status is marked ok for this upload stamp.
UPDATE users
SET resume_structured = $2, resume_structured_model = $3, resume_structured_uploaded_at = $4,
    resume_countries = $5, resume_regions = $6, resume_cities = $7,
    resume_extract_status = 'ok', resume_extract_detail = NULL, resume_extract_for = $4
WHERE id = $1 AND resume_uploaded_at = $4;

-- name: GetUserCandidateContacts :one
SELECT candidate_contacts
FROM users
WHERE id = $1;

-- name: SetUserCandidateContacts :exec
UPDATE users
SET candidate_contacts = $2
WHERE id = $1;

-- name: SetUserResumeExtractFailed :exec
-- Record that structured extract failed for the current upload. The for-stamp guard
-- drops the write when a newer upload already superseded this attempt.
UPDATE users
SET resume_extract_status = 'failed',
    resume_extract_detail = $2,
    resume_extract_for = $3
WHERE id = $1 AND resume_uploaded_at = $3;

-- name: SetUserResumeExtractPending :exec
-- Mark a retry of structured extract as running, clearing the stale status (typically
-- 'failed') a previous attempt left — the same transition a fresh upload makes in
-- SetUserResume, done here without touching the object key or re-stamping the upload
-- time, since a retry re-reads the SAME stored résumé rather than replacing it. The
-- for-stamp guard drops the write when a newer upload already superseded this retry.
UPDATE users
SET resume_extract_status = 'pending',
    resume_extract_detail = NULL,
    resume_extract_for = $2
WHERE id = $1 AND resume_uploaded_at = $2;

-- name: GetUserResumeGeography :one
-- The geography derived from the user's structured résumé, alongside the two stamps the
-- caller needs to judge freshness (the derivation's own stamp and the current résumé
-- upload time) — the same stamp-and-compare the structure read uses, so a geography
-- derived from a superseded CV reads as absent rather than being served.
-- NULL arrays mean "not known"; an empty array means the CV named a place the dictionary
-- could not resolve. The two are deliberately different answers.
SELECT resume_countries, resume_regions, resume_cities,
       resume_structured_uploaded_at, resume_uploaded_at
FROM users
WHERE id = $1;
