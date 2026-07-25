## Why

A security review of the auth surface found an account **pre-hijacking** path: `Register`
creates an account for any email without proving ownership (there is no `email_verified`
anywhere in the schema), and OAuth sign-in links a provider identity to an existing account
purely on an email match. An attacker who registers `victim@gmail.com` first keeps password
access to everything the victim later does — CV, tracking, API keys — once the victim signs
in with Google. Compounding it, the API has **no password change, no password reset, and no
session revocation**: a 30-day stateless JWT cannot be invalidated, so a user who learns
their account is compromised has no way out at all.

The same review surfaced four smaller defects worth fixing in the same pass, since they
touch the same auth/middleware seams: a fail-open Telegram webhook when its secret is unset,
unlimited server-side fetches from the contribution endpoint, API keys with no scope (a
2-hour CV-tailoring key can read other people's referral CVs), and a suffix-matched Host
trusted for OAuth redirect origins.

## What Changes

- **Email verification by code.** Registration keeps issuing a session, and additionally
  emails a 6-digit code (SES, via `internal/emailnotify`). `POST /auth/verify/request` resends
  it; `POST /auth/verify/confirm` marks the account verified. Existing accounts are
  grandfathered verified by the migration, so no live user sees a change.
- **OAuth merge no longer trusts an unverified local account.** When a provider-verified
  email resolves to an account that is unverified and password-backed, the account is handed
  to the email's proven owner: the identity is linked, `password_hash` is cleared, and
  `token_version` is bumped (killing the squatter's password and sessions). Verified accounts
  link exactly as they do today.
- **Password recovery.** `POST /auth/password/forgot` (always `202`, never reveals whether
  the email exists) mails a 6-digit code; `POST /auth/password/reset` sets a new password,
  proves email ownership (marks verified), and revokes every existing session.
- **Password change.** `POST /me/password` (cookie-only) requires the current password,
  revokes other sessions, and re-issues the caller's own cookie.
- **Session revocation.** `users.token_version` rides the JWT as a `tv` claim and is verified
  against the database on every authenticated request; `POST /auth/logout-all` bumps it.
  **BREAKING** for tokens minted before the deploy: they carry no `tv` claim and are rejected,
  so every user is signed out once at release.
- **Telegram webhook cannot fail open.** The feature refuses to enable without
  `TELEGRAM_WEBHOOK_SECRET`, so "bot on, webhook unauthenticated" becomes unrepresentable.
- **Per-user rate limit on contributions.** `POST /me/contributions` is capped per user,
  bounding the attacker-directed outbound fetches it performs.
- **API-key scopes.** Keys carry a scope; the CV-tailoring key is minted narrow (`cv`) and is
  rejected on endpoints that expose third-party data (referral CVs) or spend credits.
- **Exact-host allowlist for OAuth origins.** `requestOrigin` accepts only explicitly served
  hosts instead of any suffix of a cookie domain.

## Capabilities

### New Capabilities

- `email-verification`: proving control of an account's email address with a short-lived
  emailed code — issuing, resending, confirming, expiry/attempt limits, and what an
  unverified account may and may not be used for.
- `password-recovery`: resetting a forgotten password with an emailed code, and changing a
  known password while signed in — including the session-revocation side effects.

### Modified Capabilities

- `user-auth`: registration additionally sends a verification code; sessions become
  revocable (`token_version` claim checked per request, `logout-all`); OAuth identity
  resolution no longer merges into an unverified password account without seizing it; OAuth
  redirect origins are taken from an exact host allowlist.
- `api-keys`: keys carry a scope, and a narrow-scoped key is refused on sensitive endpoints.
- `cv-tailoring`: the bootstrap mints a `cv`-scoped key rather than a full-account one.
- `telegram-notify`: the webhook is disabled (not fail-open) when its shared secret is unset.
- `link-contributions`: board submission is rate-limited per user.

## Impact

- **Schema:** `users.email_verified`, `users.token_version`; new `user_email_codes` table;
  `api_keys.scope`. One migration, backfilling every existing user as verified.
- **Go:** `internal/accounts` (verification, recovery, merge policy), `internal/auth`
  (`Issuer` claims, `RequireAuth`/`RequireAuthOrKey` DB round-trip, key scopes),
  `internal/handler` (new auth routes, contribution limiter, telegram enable gate,
  `requestOrigin`), `internal/emailnotify` (transactional send for auth codes),
  `internal/db/queries` + regenerated sqlc.
- **Config:** `TELEGRAM_WEBHOOK_SECRET` becomes required to enable Telegram; new
  `SERVED_HOSTS` for the OAuth origin allowlist. Auth email needs `AWS_REGION` +
  `NOTIFY_EMAIL_FROM`, already used by the notify worker.
- **Performance:** every authenticated request gains one primary-key lookup for
  `token_version`. `RequireAuthOrKey` already queries on the key path; the cookie path is new.
- **Frontend (`web/`):** verification banner + code entry, forgot/reset password screens,
  change-password and sign-out-everywhere in settings.
- **Users:** one forced sign-out at deploy (JWT claim change).
