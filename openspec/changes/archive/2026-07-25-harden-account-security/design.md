## Context

The auth surface is `internal/accounts` (service + repository), `internal/auth` (bcrypt, JWT
`Issuer`, API-key hashing, cookie transport, middleware), and `internal/handler` (routes). It
has no notion of email ownership and no notion of session state: `Issuer.Parse` verifies a
signature and an expiry, nothing else, and `users` has no column that could invalidate a
token. Outbound mail already exists for the notify worker (`internal/emailnotify.Client` over
SES, `Send(ctx, from, to, subject, html, text)`), but the API server only wires it for
referral pings.

Constraints that shape the design:

- Migrations run through Postgres initdb (single-run on volume init); prod applies them by
  hand. One new migration file, additive, backfilling in place.
- `RequireAuth` is on every authenticated route and is currently DB-free. Adding revocation
  means adding a query to the hot path.
- The Telegram, contribution, key-scope and origin fixes are small and independent; they ride
  along because they touch the same middleware seam, not because they are coupled.

## Goals / Non-Goals

**Goals:**

- Make account pre-hijacking impossible: an unverified, password-backed account cannot be a
  silent OAuth merge target.
- Give a user a recovery path: verify email, reset a forgotten password, change a known one,
  sign out everywhere.
- Make sessions revocable at all.
- Close the four secondary findings (webhook fail-open, unbounded contribution fetches,
  unscoped API keys, suffix-trusted Host).

**Non-Goals:**

- MFA/TOTP, magic-link sign-in, account deletion, admin-side session management.
- Per-endpoint scope grammar for API keys. Two scopes (`full`, `cv`) is what the finding
  requires; a general scope system would be infrastructure ahead of need.
- Changing the 30-day JWT TTL. Revocation is the missing property, not lifetime.
- Reworking cookie-domain SSO (`.freehire.me` wildcard). Noted as a separate risk below.

## Decisions

### Email verification is a flag on `users` plus a single-row code table

`users.email_verified boolean NOT NULL DEFAULT false`, backfilled `true` for every existing
row. Codes live in their own table, one outstanding code per (user, purpose):

```sql
CREATE TABLE user_email_codes (
    user_id    bigint      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose    text        NOT NULL CHECK (purpose IN ('verify_email', 'password_reset')),
    code_hash  text        NOT NULL,
    expires_at timestamptz NOT NULL,
    attempts   integer     NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, purpose)
);
```

The composite primary key *is* the "at most one outstanding code" rule — a resend is an
upsert, so no cleanup job is needed and a stale code cannot coexist with a fresh one.
Alternative considered: a rows-accumulate table with `used_at`, which needs a sweeper and an
"is this the newest?" query. Rejected: more moving parts for no gain.

**Codes are bcrypt-hashed**, reusing `auth.HashPassword`/`CheckPassword` rather than the
SHA-256 used for API keys. A six-digit code under an unsalted SHA-256 is recoverable from a
database snapshot by inspection; bcrypt makes a stolen snapshot useless for the 15 minutes the
code is alive. The cost (~100 ms, at most 5 attempts) is irrelevant at this volume.

### Grandfathering: existing accounts are verified, new ones are not

The migration sets `email_verified = true` for all existing rows. This does legitimise a
pre-hijack that already happened, which is accepted: the alternative — forcing the whole user
base through a code prompt at deploy — is a much larger, certain harm than a speculative one.

### An unverified account is seized, not blocked, on OAuth merge

When a provider-verified email resolves to an unverified, password-backed account, the
identity is linked *and* `password_hash` is set to NULL, `email_verified` to true, and
`token_version` bumped, in the same transaction as the link. The squatter's password and any
session they hold stop working; the victim signs in with Google and never sees a wall.

Alternative considered: refuse the sign-in with `auth_error=account_exists`. Rejected — it is
equally safe but hands the victim a dead end they cannot resolve (they do not know the
password and, at that point, resetting it would mail the *attacker's*... no: it would mail
them, but the flow is a puzzle the user did not ask for). Seizure is the Microsoft-recommended
resolution for this exact attack and is the better product outcome.

### Session revocation: `token_version` claim, checked per request

`users.token_version integer NOT NULL DEFAULT 1`. The JWT carries it as a `tv` claim; every
authenticated request loads the user's current version and rejects a mismatch.

**Default 1, not 0** — a pre-change token has no `tv` claim, which decodes to zero, so
starting the counter at 1 makes "missing claim" fail the comparison without any pointer or
presence gymnastics. Every user is signed out once at deploy; that is the intended, stated
break.

The cost is one primary-key lookup per authenticated request on the cookie path (the key path
already queries). `RequireRole` established this pattern, and the pool is local. A cache is
the obvious later optimisation and is deliberately not built now; the seam is
`auth.TokenVersionLoader`, so a caching implementation drops in without touching handlers.

`RequireAuth(iss)` becomes `RequireAuth(iss, versions)` — a mechanical change at ~6 call
sites.

### Auth mail lives in `emailnotify`, behind a port in `accounts`

`emailnotify.AuthMailer` (sibling of the existing `Notifier`) renders the two transactional
mails over the existing `Client`. `accounts` depends on a narrow `CodeMailer` interface, so
the service stays free of AWS. Absent SES config the mailer is nil: registration still
succeeds (unverified), and the code endpoints answer `503`.

`POST /auth/password/forgot` **always answers `202` immediately** and does the lookup, bcrypt
and send on a bounded background context — the same shape as the Telegram contribution
handler. That removes the timing side-channel for free, instead of trying to equalise it with
dummy work.

### API-key scope: two named middlewares, safe by default

`api_keys.scope text NOT NULL DEFAULT 'full' CHECK (scope IN ('full','cv'))`;
`AuthenticateAPIKey` returns it alongside `user_id`.

Rather than sprinkling scope checks over 65 routes, there are two composed middlewares:

- `auth.RequireAuthOrKey` — cookie or **full-scope** key. This keeps the existing variable
  name and stays the default for every route, so a new endpoint is confined by default.
- `auth.RequireAuthOrScopedKey(iss, keys, "cv")` — cookie or a key whose scope is `full` or
  `cv`. Applied only to `/me/cvs/*` and `/auth/me`.

A narrow key on a full-scope route is `403` (insufficient scope), distinct from `401`.
Alternative considered: a scope column holding a set of route prefixes. Rejected as
infrastructure ahead of need.

### Contribution limiter is keyed on the user

`limiter.New` with a `KeyGenerator` returning the authenticated user id, `Max: 20`,
`Expiration: time.Hour`, mounted only on `POST /me/contributions` after `keyAuth` (so the user
id is resolved). In-memory per instance, matching the existing `authLimiter` precedent —
single-node deployment, and the point is to bound abuse, not to be exact.

### `requestOrigin` takes an exact host allowlist

New `SERVED_HOSTS` (csv). `requestOrigin` honours `c.Hostname()` only on an exact match,
otherwise falls back to `frontendOrigin`. When unset it defaults to the frontend origin's
host, so dev is unchanged. The cookie-domain suffix helper stays as it is — it governs cookie
scope, not redirect targets, and conflating the two is what produced the finding.

### Telegram: the secret joins the enable condition

`handler.New` requires `TelegramBotToken != "" && JWTSecret != "" && TelegramWebhookSecret != ""`,
logging when a token is present but the secret is not. The constant-time compare then can
never be reached with an empty expected secret, so "bot live, webhook open" is unrepresentable
rather than merely guarded against.

## Risks / Trade-offs

- **Every user is signed out at deploy** (the `tv` claim) → stated in the proposal as
  BREAKING; ship it with the changelog entry, and it is a one-time re-login, not data loss.
- **A DB lookup on every authenticated request** → one PK read on a local pool; the loader is
  an interface so a cache can land later without handler churn. Measure `/api/v1/jobs` p95
  before/after.
- **Grandfathering hides an existing pre-hijack** → accepted, documented above; a
  `logout-all` + password change is available to any user who suspects one.
- **SES becomes a dependency of account recovery** → recovery degrades to `503`, never to a
  silent failure; registration and sign-in never depend on it.
- **`SERVED_HOSTS` unset in prod** → OAuth on `apply.freehire.me` would redirect back to the
  canonical origin instead of failing. Degradation, not breakage; the deploy step sets it.
- **The `.freehire.me` wildcard session cookie is still readable by any subdomain** → out of
  scope here; the exact-host allowlist narrows the OAuth half only. Worth its own change.
- **Seizing an unverified account is destructive** (a real user who never verified and signs
  in with Google loses their password) → they keep the account and every row in it, and can
  set a password again through the reset flow. The mail explains it.

## Migration Plan

1. Apply `migrations/0041_account_security.sql` by hand on prod (`email_verified` +
   `token_version` on `users`, `scope` on `api_keys`, `user_email_codes`), backfilling
   `email_verified = true` and `token_version = 1`.
2. Set `SERVED_HOSTS=freehire.me,apply.freehire.me` and confirm `TELEGRAM_WEBHOOK_SECRET` is
   present (it is — verified against prod) before rolling the image; a missing secret now
   disables Telegram instead of opening the webhook.
3. Deploy. All sessions end; the SPA's `401` handling already routes to sign-in.
4. Rollback: the previous image ignores the new columns entirely, so rolling back is safe
   without a down-migration (users would be signed out a second time).

## Open Questions

- Should the verification banner block anything in the SPA after N days unverified? Deferred:
  ship the banner, watch the verified rate, decide later.
