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
-- banned drives the sign-in refusal and the silence of the reset-code mail.
SELECT id, email, role, beta_tester, email_verified, password_hash, created_at, language,
       (banned_at IS NOT NULL)::boolean AS banned
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
-- input, same as every other single-column update in this file. Automatic adoption
-- preserves a concurrently saved preference under the row lock.
UPDATE users
SET timezone = CASE WHEN sqlc.arg(only_if_unset)::boolean THEN COALESCE(timezone, sqlc.narg(timezone)::text) ELSE sqlc.narg(timezone)::text END
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
--
-- The same statement stamps last_active_at, at most once a day, because this is the
-- one place every signed-in request passes; mail_recipient (migration 0247) reads it
-- to stop optional mail to people who have not used freehire in 180 days.
--
-- A banned account returns no row, which every caller already reads as "session revoked"
-- (middleware.resolveSession, the introspection endpoint) or, at sign-in, as a refusal
-- (authHandlers.sessionVersion). This is the one place a ban meets every cookie and
-- Bearer-JWT credential, and a ban does not stamp last_active_at.
WITH seen AS (
    UPDATE users AS su SET last_active_at = now()
    WHERE su.id = $1 AND su.banned_at IS NULL AND su.last_active_at < now() - interval '1 day'
)
SELECT u.token_version
FROM users AS u
WHERE u.id = $1 AND u.banned_at IS NULL;

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

-- name: ChangeUserEmail :one
-- Move the account to a new address after a mailed code proved control of it: the
-- address is verified by that same code, and every session is revoked, since the address
-- is a sign-in credential like the password.
--
-- The codes tied to the account's address are destroyed in the same statement. A pending
-- password_reset code was mailed to the OLD address, and ResetPassword looks the account
-- up by address and the code up by account — so once the account answers to the new
-- address, a code sitting in the old mailbox would reset its password. Whoever still
-- reads that mailbox must not keep a way back in. A data-modifying CTE for the same reason
-- ResetUserPassword uses one: the cleanup cannot be separated from the change.
--
-- API keys survive, unlike a reset: the change was made from a session that just proved
-- its credential, so the account has not changed hands.
WITH dropped_codes AS (
    DELETE FROM user_email_codes
    WHERE user_id = $1 AND purpose IN ('verify_email', 'password_reset')
)
UPDATE users
SET email = $2, email_verified = true, token_version = token_version + 1
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

-- name: GetAgentsMayApply :one
-- Whether a connected MCP agent may act on this account's auto-apply attempts (migration 0196).
SELECT agents_may_apply FROM users WHERE id = sqlc.arg(id);

-- name: SetAgentsMayApply :exec
UPDATE users SET agents_may_apply = sqlc.arg(agents_may_apply) WHERE id = sqlc.arg(id);

-- name: BanUser :one
-- Ban an account by hand. Advances token_version so everything bound to a generation
-- (recent-auth proofs, mobile exchange codes, OAuth grants and codes) dies with the
-- sessions, and deletes nothing else. The WHERE refuses an admin and the moderator's own
-- account here rather than in Go, so no call site can ban around the rule. No row means
-- the target is missing, already banned, an admin or the actor itself; the handler tells
-- those apart with GetUserModerationState.
UPDATE users
SET banned_at = now(), ban_reason = sqlc.arg(reason), banned_by = sqlc.arg(actor_id),
    token_version = token_version + 1
WHERE id = sqlc.arg(id) AND banned_at IS NULL AND role <> 'admin' AND id <> sqlc.arg(actor_id)
RETURNING id;

-- name: UnbanUser :execrows
-- Lift a ban. Sessions and grants stay dead (the ban bumped token_version); the person
-- signs in again. 0 rows: the account is missing or was not banned.
UPDATE users
SET banned_at = NULL, ban_reason = NULL, banned_by = NULL
WHERE id = $1 AND banned_at IS NOT NULL;

-- name: FindUserForModeration :one
-- The moderator's lookup: one account by id, exact email (case-insensitive) or username,
-- with its ban state. The query is matched against all three; an id only when numeric.
SELECT id, email, username, role, banned_at, ban_reason
FROM users
WHERE id = sqlc.arg(user_id)::bigint
   OR lower(email) = lower(sqlc.arg(query)::text)
   OR username = sqlc.arg(query)::text
ORDER BY (id = sqlc.arg(user_id)::bigint) DESC
LIMIT 1;

-- name: GetAutoApplyConfirmEach :one
SELECT auto_apply_confirm_each FROM users WHERE id = sqlc.arg(id);

-- name: SetAutoApplyConfirmEach :exec
UPDATE users SET auto_apply_confirm_each = sqlc.arg(auto_apply_confirm_each) WHERE id = sqlc.arg(id);
