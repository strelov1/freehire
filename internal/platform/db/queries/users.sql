-- name: CreateUser :one
-- Register a new account. email is stored as given (the handler lowercases it);
-- the unique index on lower(email) rejects duplicates regardless of case. role is
-- returned so the new account's wire shape carries it (always 'user' at creation).
-- email_verified is a parameter, not a default: a password registration starts
-- unverified, while an account created from an OAuth sign-in is verified at birth
-- because the provider already proved the address. timezone is optional
-- (sqlc.narg) — a password registration passes the browser-detected zone; the
-- OAuth account-creation path (LinkOrCreateByEmail) omits it, same as before
-- this column existed.
INSERT INTO users (email, password_hash, email_verified, timezone)
VALUES ($1, $2, $3, sqlc.narg(timezone))
RETURNING id, email, role, beta_tester, email_verified, created_at, timezone, language;

-- name: GetUserByEmail :one
-- Login lookup. Case-insensitive on email; returns password_hash so the handler
-- can verify the password (and reject accounts that have none). role feeds the
-- post-login wire shape. email_verified drives both the "confirm your email" prompt
-- and the OAuth merge policy (an unverified account is seized, not silently joined).
-- language is included so a password login's response carries the account's
-- preference the same as /auth/me, rather than reporting the zero value.
SELECT id, email, role, beta_tester, email_verified, password_hash, created_at, language
FROM users
WHERE lower(email) = lower($1);

-- name: GetUserByID :one
-- Profile lookup for the authenticated user. role is included so /auth/me can tell a
-- client whether to surface moderator-only UI. The password hash itself never leaves
-- the database — only whether one exists, which is what lets the SPA offer a password
-- change to password accounts and explain itself to OAuth-only ones. timezone is NULL
-- until the user sets one on their profile (internal/application/deliverywindow reads NULL as UTC).
-- language is never NULL — it has a NOT NULL DEFAULT, so every account has one from
-- creation. onboarding_completed_at is NULL until the account has been through the
-- wizard, and it rides along here rather than on its own endpoint because the root
-- layout's gate needs it on the same read it already makes to decide anything at all.
SELECT id, email, role, beta_tester, email_verified,
       (password_hash IS NOT NULL)::boolean AS has_password,
       created_at, timezone, language, onboarding_completed_at
FROM users
WHERE id = $1;

-- name: UpdateUserTimezone :one
-- Set (or clear, with NULL) the account's IANA timezone name. The handler validates
-- the name against time.LoadLocation before this runs — the query itself trusts its
-- input, same as every other single-column update in this file.
UPDATE users
SET timezone = $2
WHERE id = $1
RETURNING id, email, role, beta_tester, email_verified,
          (password_hash IS NOT NULL)::boolean AS has_password,
          created_at, timezone, language;

-- name: UpdateUserLanguage :one
-- Set the account's preferred interface language. The handler validates the code
-- against the curated set before this runs (also enforced by the DB CHECK
-- constraint as a second line of defense) — the query itself trusts its input,
-- same as every other single-column update in this file.
UPDATE users
SET language = $2
WHERE id = $1
RETURNING id, email, role, beta_tester, email_verified,
          (password_hash IS NOT NULL)::boolean AS has_password,
          created_at, timezone, language;

-- name: GetUserPasswordHash :one
-- The account's stored password hash, for verifying a current password on change.
-- NULL when the account is passwordless (OAuth-only), which the caller treats the same
-- as a wrong password: there is nothing to verify against.
SELECT password_hash
FROM users
WHERE id = $1;

-- name: GetUserLanguage :one
-- The account's preferred interface language on its own, for a caller that needs
-- nothing else about the user — the assistant's turn loop and the fit-analysis
-- chain both build a language directive from just this column. Never NULL (NOT
-- NULL DEFAULT 'en'), so every account answers.
SELECT language
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

-- name: ListUsersForResumeGeoBackfill :many
-- Users whose stored structured résumé currently describes their stored CV, for the
-- geography reconciler (cmd/backfill-resume-geo). Superseded structures are excluded:
-- deriving geography from one would route around the staleness rule that governs the
-- structure itself. Returns the location line the derivation reads plus the stamp to
-- write under, so the worker needs no second round-trip per user.
-- The location line is coalesced to '' and cast so sqlc types it as a string rather than
-- interface{}. An absent key and an empty string are the same case — the CV stated no
-- location — and both must derive to an ABSENT geography, not to an empty resolved one.
SELECT id, coalesce(resume_structured ->> 'location', '')::text AS location, resume_uploaded_at
FROM users
WHERE resume_structured IS NOT NULL
  AND resume_uploaded_at IS NOT NULL
  AND resume_structured_uploaded_at IS NOT DISTINCT FROM resume_uploaded_at
  AND (sqlc.arg(user_id)::bigint = 0 OR id = sqlc.arg(user_id)::bigint)
ORDER BY id;

-- name: SetUserResumeGeography :exec
-- Persist only the derived geography for a user, under the same monotonic guard the
-- structure write uses. This is the reconciler's write path: it re-derives from an
-- already-stored structure, so it must not touch the structure or its model stamp, and
-- must still refuse to write against a CV that has been replaced since the row was read.
UPDATE users
SET resume_countries = $2, resume_regions = $3, resume_cities = $4
WHERE id = $1 AND resume_uploaded_at = $5;

-- name: GetUserRole :one
-- Slim role lookup for the RequireRole authorization middleware: it runs on every
-- request to a role-gated endpoint and needs only the role, so it does not drag the
-- full user row (the GetJobIDBySlug precedent for a hot-path read).
SELECT role
FROM users
WHERE id = $1;

-- name: ListUserBlobKeys :many
-- Every object-storage key the account owns, in one read: the stored CV, the headshot,
-- each referral-proof PDF, and the raw MIME of each hosted email. Account deletion
-- collects these BEFORE deleting any row — the mail and proof keys live in the rows
-- themselves, so once those are gone the objects are unreachable and would sit in
-- the bucket forever. Empty keys are filtered out so a caller never asks storage to
-- delete "".
SELECT u.resume_object_key AS key FROM users u
WHERE u.id = $1 AND u.resume_object_key IS NOT NULL AND u.resume_object_key <> ''
UNION
SELECT u.photo_object_key FROM users u
WHERE u.id = $1 AND u.photo_object_key IS NOT NULL AND u.photo_object_key <> ''
UNION
SELECT o.proof_object_key FROM referral_offers o
WHERE o.user_id = $1 AND o.proof_object_key <> ''
UNION
SELECT e.s3_key FROM emails e
WHERE e.user_id = $1 AND e.s3_key IS NOT NULL AND e.s3_key <> '';

-- name: GetUsernameByUser :one
-- The account's own username (NULL until claimed/allocated) and, when set, the
-- time of its last EXPLICIT change via SetUsername — NULL for a lazily
-- allocated default written by SetUsernameIfAbsent, which never touches this
-- column (see the add-username-claim change's design.md, Decision 2).
SELECT username, username_updated_at
FROM users
WHERE id = $1;

-- name: SetUsernameIfAbsent :execrows
-- Claim username for id only if the account has none yet, leaving
-- username_updated_at untouched. Zero affected rows means the account already
-- has a username (a concurrent caller won the race); a unique violation on
-- username means another account already holds it. The caller resolves either
-- case by re-reading GetUsernameByUser.
UPDATE users
SET username = $2
WHERE id = $1
  AND username IS NULL;

-- name: SetUsername :exec
-- Replace the account's username unconditionally and record the change time.
-- The caller (accounts.ClaimUsername) has already validated the format, the
-- reserved list, and the 30-day cooldown against the account's own prior
-- change — this query trusts its input, same as every other single-column
-- update in this file. A unique violation means another account already holds
-- username.
UPDATE users
SET username = $2, username_updated_at = now()
WHERE id = $1;

-- name: GetUserIDByUsername :one
-- Uniqueness/availability lookup: which account (if any) already holds
-- username. Used by the username-check endpoint and by the hosted-mailbox
-- inbound resolver to map a recipient's local-part back to its owning user.
SELECT id
FROM users
WHERE username = $1;

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

-- name: GetTalentNetworkVisibility :one
-- The caller's own Talent Network opt-in state, for the owner-facing settings toggle.
-- talent_handle rides along so the page can render the public URL without a second
-- round-trip. It is NULL until the first join — a non-member has no card to link to —
-- unlike the visibility, which every row carries because 'off' is the column default.
SELECT talent_network_visibility, talent_handle
FROM users
WHERE id = $1;

-- name: ListMembersMissingTalentHandle :many
-- The members who joined before handles existed, and so have no public address.
--
-- Migration 0148 rewrote every 'public' row to 'anonymous' but could not mint a handle —
-- minting reads a job title through a Go dictionary, which SQL cannot do — so those
-- accounts, and any that were already 'anonymous', are members the catalogue cannot list
-- and whose card 404s. cmd/backfill-talent-handle walks this list once and closes it.
--
-- No stamp gate here, unlike the catalogue's own read: a member whose CV extract is stale
-- still needs an address for when it catches up, and withholding one would make the
-- backfill's own result depend on when it happened to run.
SELECT id
FROM users
WHERE talent_network_visibility <> 'off'
  AND talent_handle IS NULL
ORDER BY id;

-- name: SetTalentHandleIfUnset :execrows
-- Claims a freshly minted catalogue handle for a candidate who does not have one yet.
--
-- The `talent_handle IS NULL` predicate is the whole mechanism, and it does two jobs.
-- It makes the mint idempotent — a member who leaves and rejoins keeps the handle they
-- already shared, and a second concurrent join claims nothing — and it makes the
-- statement's own result the answer: 0 rows means somebody already has one, which the
-- caller reads rather than re-querying and racing again.
--
-- A collision with ANOTHER account's handle surfaces as a unique-violation from
-- users_talent_handle_key (migration 0150), not as 0 rows. The caller mints a new suffix
-- and retries — the same shape internal/identity/accounts uses to allocate a username.
UPDATE users
SET talent_handle = $2
WHERE id = $1
  AND talent_handle IS NULL;

-- name: SetTalentNetworkVisibility :exec
-- Owner-scoped write of the caller's Talent Network membership ('off' or 'anonymous'
-- since migration 0148). Does not touch talent_handle: the public URL stays stable
-- across a round trip through 'off', so a candidate who already shared it once — or who
-- leaves and rejoins — never has to reshare a new one.
--
-- The value is NOT validated here. Its authority is the CHECK constraint on the column;
-- the handler mirrors that set for a cheap 400, and this statement is the third place
-- the vocabulary would have to be repeated for no gain.
UPDATE users
SET talent_network_visibility = $2
WHERE id = $1;

-- name: ListTalentNetworkMembers :many
-- Every member the public catalogue may show, in one read. The caller projects each row
-- through talentnetwork.ProjectCard and holds the result as a snapshot — see that
-- package's doc for why the whole set is read at once rather than filtered in SQL: the
-- category and seniority a card is filtered by do not exist as columns, they are derived
-- from the job title by a dictionary that changes weekly.
--
-- The predicate is the membership rule and nothing else:
--
--   * not 'off' — the candidate asked to be found;
--   * a minted handle — without one there is no URL to link the card to, so a row in
--     this state is mid-join, not a member;
--   * the stamp gate (resume_structured_uploaded_at = resume_uploaded_at, both set) —
--     the same "this structure still describes the CV on file" rule the rest of the
--     product applies. Without it most cards would be somebody's previous CV.
--
-- LEFT JOIN, because a candidate can join before ever saving a profile: a missing
-- user_profiles row is empty facets, not a missing member.
--
-- Ordered here rather than by the caller so the snapshot arrives sorted, and TOTALLY:
-- two members sharing a timestamp would otherwise order arbitrarily, and an arbitrary
-- order across pages silently drops some people and repeats others.
SELECT u.talent_handle,
       u.timezone,
       COALESCE(u.resume_cities, '{}')::text[] AS cities,
       u.resume_structured,
       u.resume_structured_uploaded_at,
       COALESCE(p.specializations, '{}')::text[] AS specializations
FROM users u
LEFT JOIN user_profiles p ON p.user_id = u.id
WHERE u.talent_network_visibility <> 'off'
  AND u.talent_handle IS NOT NULL
  AND u.resume_uploaded_at IS NOT NULL
  AND u.resume_structured_uploaded_at = u.resume_uploaded_at
ORDER BY u.resume_structured_uploaded_at DESC, u.talent_handle DESC;

-- name: GetTalentNetworkMemberByHandle :one
-- One member's card, by the handle in the public URL. Same predicate as the list, so a
-- handle nobody holds, a member who has left, and one whose extract has gone stale all
-- come back as pgx.ErrNoRows — which the handler renders as the one 404. Deciding it
-- here rather than in the caller is deliberate: three ways to be absent and one way to
-- say so is a rule that cannot be half-applied.
--
-- Read against the DATABASE, never the snapshot the list is served from. A candidate who
-- leaves must stop resolving immediately, not when the snapshot next refreshes.
SELECT u.talent_handle,
       u.timezone,
       COALESCE(u.resume_cities, '{}')::text[] AS cities,
       u.resume_structured,
       u.resume_structured_uploaded_at,
       COALESCE(p.specializations, '{}')::text[] AS specializations
FROM users u
LEFT JOIN user_profiles p ON p.user_id = u.id
-- The ::text cast is load-bearing, not decoration: talent_handle is nullable, so without
-- it sqlc types the argument as pgtype.Text and every caller has to wrap a plain string
-- it already knows is present.
WHERE u.talent_handle = sqlc.arg(handle)::text
  AND u.talent_network_visibility <> 'off'
  AND u.resume_uploaded_at IS NOT NULL
  AND u.resume_structured_uploaded_at = u.resume_uploaded_at;

-- name: GetUserExperienceRequireContext :one
-- Whether interactive atom creates require a non-empty context. Kept off /auth/me on
-- purpose — only the experience write path and get_profile's bank summary need it.
SELECT experience_require_context
FROM users
WHERE id = $1;

-- name: SetUserExperienceRequireContext :exec
-- Chat-opt-in (or opt-out) for requiring context on interactive experience creates.
UPDATE users
SET experience_require_context = $2
WHERE id = $1;

-- name: MarkOnboardingComplete :exec
-- Record that this account has been through the onboarding wizard, so it is never routed
-- there again. Guarded on IS NULL rather than written unconditionally: the useful fact is
-- WHEN the account first finished, and a second call (a re-submit, a double click, a
-- decline after a finish) must not overwrite it. That guard is also what makes the
-- endpoint idempotent — a repeat call affects no rows and is still a success.
UPDATE users
SET onboarding_completed_at = now()
WHERE id = $1 AND onboarding_completed_at IS NULL;
