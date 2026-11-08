## Context

`hire` is an early backend: a Fiber v2 server over Postgres (sqlc, no ORM),
serving read-only `jobs` and `companies` endpoints to a Svelte SPA. There is no
user concept anywhere. This change introduces the first authenticated surface.
The SPA and API are deployed **same-origin** (a dev Vite proxy forwards `/api`
to the backend, mirroring the production reverse-proxy), which shapes the
session-transport decision below. Per project conventions, sqlc is the only DB
layer, migrations apply via Postgres initdb, and response shapes follow
`{"data": ...}`.

## Goals / Non-Goals

**Goals:**

- A `users` identity with secure password storage (bcrypt).
- Stateless JWT auth delivered in an httpOnly cookie, XSS-safe under the
  same-origin deployment.
- `register`, `login`, `logout`, `me` endpoints plus a reusable "require auth"
  middleware that future protected routes can adopt without further wiring.
- Stay within existing conventions (sqlc, config-from-env, `handler.Register`).

**Non-Goals:**

- Roles/authorization tiers (admin vs user) — not needed until a mutating or
  privileged endpoint exists. Noted as a seam.
- Refresh tokens / token revocation — out of scope; the trade-off is accepted
  below. (Logout *is* in scope: it clears the cookie, though the JWT itself
  stays valid until `exp`.)
- A standalone CSRF-token mechanism — `SameSite=Lax` + same-origin covers the
  current endpoints; revisit when cross-site or stricter needs appear.
- Email verification, password reset, OAuth, magic-link — explicitly excluded
  this iteration (OAuth/magic-link are the announced *next* task; the data model
  is shaped to absorb them additively, but none of their tables or flows are
  built here).
- Gating any existing read endpoint behind auth.

## Decisions

### Stateless JWT (HS256) in an httpOnly cookie

Tokens are signed with a shared secret (`JWT_SECRET`) and carry the user id as
`sub` plus an `exp`. Register/login set the token in an `HttpOnly`,
`SameSite=Lax` cookie (`Secure` configurable via `COOKIE_SECURE`); the browser
attaches it automatically, and the SPA never sees the token.

*Why a cookie over a JS-readable token:* an `HttpOnly` cookie is immune to token
theft via XSS, which a `localStorage`/`Authorization: Bearer` token is not.
*Why this works cleanly here:* the SPA and API are same-origin (Vite proxy in
dev), so `SameSite=Lax` sends the cookie on the app's own requests while
blocking it on cross-site requests — that *is* the CSRF defense, with no CORS
`AllowCredentials` and no separate CSRF token needed for the current endpoints.
A cross-origin SPA would have forced `SameSite=None; Secure` (losing the CSRF
benefit) — hence the same-origin deployment decision.

*Why still JWT, not DB sessions:* stateless validation, no per-request lookup,
no `sessions` table. Trade-off: no server-side revocation (see Risks). The token
remains transport-agnostic (only `sub`), so swapping to opaque session IDs later
would not change handlers.

*Library:* `github.com/golang-jwt/jwt/v5` — the de-facto Go JWT library;
maintained, v5 has the safer parsing API.

### bcrypt for password hashing

`golang.org/x/crypto/bcrypt` with the default cost. The salt and cost are
embedded in the hash string, so the `users` table needs a single
`password_hash` column — no separate salt column. *Alternative:* argon2id
(stronger, PHC winner) but requires hand-tuning salt/time/memory params; bcrypt
is the simpler, proven default for an MVP.

### New `internal/auth` package, separate from handlers

A focused package owns the security primitives behind small interfaces:

- password hashing/verification (`HashPassword`, `CheckPassword`),
- token issue/verify (`Issuer` wrapping secret + TTL: `Issue(userID)` /
  `Parse(token) → userID`),
- the cookie transport (`CookieName`, `SetTokenCookie`, `ClearTokenCookie`) so
  cookie attributes live in one place,
- a Fiber middleware `RequireAuth` that reads and validates the auth cookie and
  stores the user id in `c.Locals`.

Handlers (`internal/handler/auth.go`) stay thin: parse/validate input, call
sqlc + `internal/auth`, set the cookie, shape the `{"data": ...}` response. This
keeps crypto and token logic testable in isolation (unit tests with no DB) and
the security boundary easy to hold in context. `handler.Register` grows
parameters for the JWT secret/TTL and the cookie-secure flag (mirroring how
`frontendOrigin` is already threaded in).

### Data model

`migrations/0005_users.sql`:

```text
id            bigint generated always as identity primary key
email         text not null
password_hash text                       -- NULLABLE: passwordless users have none
created_at    timestamptz not null default now()
unique (lower(email))   -- case-insensitive uniqueness
```

Queries in `internal/db/queries/users.sql`: `CreateUser`, `GetUserByEmail`,
`GetUserByID`. The `password_hash` column is selected only where needed
(login/registration) and never serialized into a handler response — the API
user type omits it.

`password_hash` is **nullable** on purpose (see "Forward-compatibility" below):
the announced next iteration adds Google OAuth and magic-link sign-in, where a
user has no password at all. Relaxing `NOT NULL` later would mean a second
migration — and this project has no migration runner yet (changing `0005` does
not re-apply to a live volume), so a `NOT NULL → NULL` change on a persistent DB
is genuinely painful. Getting the nullability right now is free. The password
login path treats a row with a null hash as "this account has no password" and
rejects it with the same generic `401` as a wrong password.

### Email as the canonical account key

`UNIQUE (lower(email))` makes email the one identity per account. This is the
deliberate foundation for "one account, multiple ways to authenticate":
password today; Google and magic-link later all resolve to (or link against) the
same email-keyed user. The JWT carries only the user id (`sub`), so it is
already provider-agnostic — no token, middleware, or `/me` change is needed when
new sign-in methods land.

### Forward-compatibility for OAuth / magic-link (seam, not built here)

The model is shaped so the next iteration is purely additive — nothing in
`users` needs reworking:

- **Google OAuth / external identities** → a future `user_identities` table
  (`user_id`, `provider`, `provider_user_id`, ...). Linking, not a `users`
  change.
- **Magic link** → a future short-lived login-token table; passwordless, so it
  relies on the nullable `password_hash` above.
- **Email verification** → a future additive `email_verified` column (or derived
  from a verified identity).

Building any of these now would be infrastructure ahead of need — explicitly
deferred (see Non-Goals). The only thing done *now* is removing the single
narrowing barrier (`password_hash NOT NULL`); everything else is added later
without touching existing rows.

### Config

Add `JWTSecret`, `JWTTTL`, and `CookieSecure` to `config.Settings`. `JWT_SECRET`
has no safe default — startup MUST fail fast if it is empty, so a server never
boots with a guessable signing key. `JWT_TTL` defaults to a sensible value (e.g.
24h). `COOKIE_SECURE` defaults to `false` so the cookie works over
`http://localhost` in dev; set it `true` in any HTTPS deployment.

## Risks / Trade-offs

- **No token revocation** (stateless JWT) → logout clears the cookie, but the
  JWT itself stays valid until `exp` if it was captured. Mitigation: keep TTL
  modest (24h) and the cookie `HttpOnly` (so capture needs more than XSS); the
  refresh/revocation design is a known seam — a short access TTL + a
  `refresh_tokens` table can be added without changing the public contract.
- **CSRF** → cookies are auto-sent by the browser. Mitigation: `SameSite=Lax`
  plus the same-origin deployment blocks the cookie on cross-site requests, so
  state-changing requests can't be forged from other origins. If a future need
  forces `SameSite=None` (cross-site) or stricter guarantees, add a CSRF token.
- **Single shared secret** → rotating it invalidates all live tokens.
  Acceptable at MVP; mitigation is fail-fast on empty secret so it is always set
  deliberately via env.
- **No rate limiting on login** → brute-force surface. Out of scope here;
  generic `401` (no email/password distinction) and bcrypt's cost slow attacks.
  Note the seam for a future rate-limit middleware.
- **No migration runner yet** (existing project gotcha) → `0005_users.sql`
  applies only on a fresh Postgres volume; an existing dev volume needs
  `docker compose down -v && make up`. Same constraint as all current
  migrations; documented, not solved here.

## Migration Plan

1. Add `0005_users.sql`; recreate the dev DB volume to apply it.
2. Add the new go deps, `users.sql` queries, run `make sqlc`, commit generated
   code.
3. Implement `internal/auth`, then handlers, then wire `Register`.
4. Set `JWT_SECRET` (and `COOKIE_SECURE=true` on HTTPS) before running the
   server; in dev, the Vite proxy forwards `/api` so the SPA is same-origin.

Rollback: the change is additive (new table, new routes, new package). Reverting
the code and dropping the `users` table fully removes it; no existing data or
endpoint is touched.

## Open Questions

- None blocking. Role-based authorization and refresh tokens are deferred by
  decision, not left open.
