## 1. Schema and generated code

- [x] 1.1 Add `migrations/0041_account_security.sql`: `users.email_verified boolean NOT NULL DEFAULT false` (backfilled `true` for existing rows), `users.token_version integer NOT NULL DEFAULT 1`, `api_keys.scope text NOT NULL DEFAULT 'full' CHECK (scope IN ('full','cv'))`, and the `user_email_codes` table with its composite primary key
- [x] 1.2 Add the sqlc queries: `users.sql` (select/set `email_verified`, read/bump `token_version`, set `password_hash`, clear password on seizure), `api_keys.sql` (`scope` on create/list, returned by `AuthenticateAPIKey`), new `email_codes.sql` (upsert, get, bump-attempts, delete)
- [x] 1.3 Regenerate `internal/db` with `make sqlc` and confirm `go build ./...` is clean

## 2. Session revocation

- [x] 2.1 Extend `auth.Issuer` to stamp and verify a `tv` claim: `Issue(userID, tokenVersion)`, `Parse` returning both, rejecting a token with no claim
- [x] 2.2 Add `auth.TokenVersionLoader` and enforce the version in `RequireAuth`, `RequireAuthOrKey`, and `OptionalAuth`; update every call site in `internal/handler`
- [x] 2.3 Add `POST /api/v1/auth/logout-all` (cookie-only): bump the version, clear the cookie
- [x] 2.4 Update `setSession` and the OAuth/extension/mobile-exchange mint paths to read the current version when issuing

## 3. Email verification

- [x] 3.1 Add code generation and hashing in `internal/accounts`: six digits from `crypto/rand`, bcrypt at rest, 15-minute expiry, 5-attempt ceiling, 60-second resend cooldown
- [x] 3.2 Add `emailnotify.AuthMailer` (verification + reset templates, html and text) over the existing `Client`, plus the narrow `CodeMailer` port in `accounts`
- [x] 3.3 Make `Register` create the account unverified and issue+mail a code, without failing registration on a mail error
- [x] 3.4 Add `POST /api/v1/auth/verify/request` and `POST /api/v1/auth/verify/confirm` (cookie-only), returning `503` when no mailer is configured
- [x] 3.5 Surface `email_verified` on the `/auth/me` and register/login user wire shape
- [x] 3.6 Wire the mailer in `handler.New` from the existing `AWS_REGION` + `NOTIFY_EMAIL_FROM` config

## 4. Password recovery

- [x] 4.1 Add `POST /api/v1/auth/password/forgot`: always `202`, work done on a bounded background context, no code for passwordless accounts
- [x] 4.2 Add `POST /api/v1/auth/password/reset`: verify the code, set the new hash, mark verified, bump `token_version`
- [x] 4.3 Add `POST /api/v1/me/password` (cookie-only): check the current password, set the new hash, bump the version, re-issue the caller's cookie
- [x] 4.4 Mount the credential endpoints behind the existing per-IP `authLimiter`

## 5. OAuth merge policy

- [x] 5.1 Extend `Repository.LinkOrCreateByEmail` to resolve the target account's verified state and, for an unverified password account, link + clear `password_hash` + mark verified + bump `token_version` in the same transaction
- [x] 5.2 Mark accounts created from an OAuth sign-in as verified
- [x] 5.3 Update `ResolveOAuthAccount` and its race-retry paths for the new outcome, keeping `ErrIdentityConflict`/`ErrEmailRace` recovery intact

## 6. API-key scopes

- [x] 6.1 Store and return the scope: `CreateAPIKey` takes it (never from client input), `AuthenticateAPIKey` returns it, the key listing exposes it
- [x] 6.2 Add `auth.RequireAuthOrScopedKey` and make `RequireAuthOrKey` full-scope-only, answering `403` for an insufficient scope
- [x] 6.3 Mint the CV-tailoring key with scope `cv` (`mintTailoringKey`) and move the `/me/cvs/*` and `/auth/me` routes onto the scoped middleware, leaving every other route full-scope

## 7. Secondary hardening

- [x] 7.1 Require `TELEGRAM_WEBHOOK_SECRET` in the Telegram enable condition in `handler.New`, logging when a bot token is present without it
- [x] 7.2 Add the per-user limiter (20/hour) on `POST /api/v1/me/contributions`
- [x] 7.3 Add `SERVED_HOSTS` config and make `requestOrigin` match an exact host, defaulting to the frontend origin's host

## 8. Frontend

- [x] 8.1 Show an "confirm your email" banner for an unverified user with a code-entry form calling the verify endpoints
- [x] 8.2 Add the forgot-password and reset-password screens
- [x] 8.3 Add change-password and "sign out everywhere" to account settings

## 9. Verification and release

- [x] 9.1 Run `go build ./...`, `go vet ./...`, `go test ./...`, and the web lint/build gates
- [x] 9.2 Update `AGENTS.md` module docs (`internal/auth`, `internal/accounts`, config env list) for the new columns, endpoints, and env vars
- [x] 9.3 Write the deploy runbook step for the manual migration, `SERVED_HOSTS`, and the one-time forced sign-out
