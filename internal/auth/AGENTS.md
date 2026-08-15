# Auth conventions

## Scope
Auth primitives: bcrypt password hashing, JWT cookie transport, API-key hashing/auth, and the `RequireAuth`/`RequireAuthOrKey` Fiber middleware.

## Always true
- **JWT is HS256 and carries `sub` (user id) plus `tv` (the account's session generation).** It survives new sign-in methods and a later swap to opaque sessions. `tv` is what makes a stateless token revocable: it is compared against `users.token_version` on every authenticated request, so a bump (sign-out-everywhere, password change or reset, an OAuth seizure of an unverified account) strands every outstanding token. Accounts start at generation **1**, so a token minted before versioning — which has no `tv` claim, decoding to zero — can never match. The version load **fails closed**, which is also what terminates a *deleted* account's sessions everywhere: the row is gone, the read errors, every outstanding cookie is 401. Do not soften that error path into "tolerate a hiccup" — it would re-admit deleted accounts, whose writes then hit foreign-key 500s instead of a clean 401.
- **Transport is an `HttpOnly; SameSite=Lax` cookie**, never a `Bearer` header or `localStorage` — XSS-safe, same-origin CSRF defense (no CSRF token needed yet).
- **SPA and API must be same-origin.** In dev the Vite proxy (`web/vite.config.ts`) forwards `/api` to the backend.
- `users.password_hash` is **nullable** — passwordless sign-in creates accounts with no password; password login rejects a null hash with a generic `401`.
- `email` is the canonical account key (`UNIQUE (lower(email))`); external providers link via `user_identities`.
- `JWT_SECRET` is required at server startup (fail-fast in `cmd/server`); `COOKIE_SECURE=true` for HTTPS (default false for http://localhost dev).
- Credential endpoints are throttled by a Redis-backed GCRA rate limiter (`internal/ratelimit`, keyed on client IP, no in-memory fallback; it fails open on Redis errors) — `login` 10/min, `register` 5/min.
- **API keys are hashed at rest:** the row stores only the `HashAPIKey(token)` SHA-256 (i.e. `SHA-256(token)`) plus a short non-secret `token_prefix` (enough to tell keys apart in a list); the plaintext (minted by `GenerateAPIKey`) is shown exactly once at create time and is unrecoverable.
- **Key management is cookie-only (`RequireAuth`)** — a leaked key must not be able to create, list, or revoke keys.
- **Minting a key requires a proven address.** `CreateAPIKey` is an `INSERT ... SELECT ... WHERE users.email_verified`; no row means `403`. The gate lives in the statement because registration hands out a session before the address is proven, so a squatter on someone else's email could otherwise walk away with a never-expiring, full-scope credential.
- **A key does not carry `tv`, so a version bump does not revoke it.** `AuthenticateAPIKey` matches `token_hash` + `expires_at` and never joins `users`. This is deliberate for `logout-all` and a password change — a key is a durable programmatic credential, not a session — but it means a *takeover* must delete the rows: `SeizeUnverifiedAccount` and `ResetUserPassword` do so in the same statement. Do not "fix" this by joining `tv` into `AuthenticateAPIKey`; that would make every key die with every sign-out-everywhere and leave dead rows listed as live.
- **Per-user job endpoints accept either credential** (`RequireAuthOrKey`); `/auth/me` mounts `RequireAuthOrScopedKey`, which additionally admits the narrow `cv` key.

## How it works

`internal/auth` owns four responsibilities:

1. **Password hashing** (`password.go`): bcrypt between register and login.
2. **JWT Issuer** (`token.go`): issues and verifies HS256 tokens carrying `sub`, `tv`, and a `jti` (random session id). `Parse` returns `sub`/`tv` and rejects a claimless (pre-versioning) token with `ErrNoTokenVersion`; `SessionFingerprint` binds a recent-auth proof to one exact session and refuses a legacy no-`jti` token with `ErrNoSessionID`.
3. **Cookie transport** (`cookie.go`): `SetTokenCookie`/`ClearTokenCookie` with `HttpOnly; SameSite=Lax; Path=/`. Both take a `domain` (from `COOKIE_DOMAIN` — `.freehire.me` in prod, so subdomains share one SSO cookie); when a domain is set, clear also expires any leftover host-only cookie.
4. **Middleware** (`middleware.go`): `RequireAuth(iss, versions)` reads the JWT cookie, confirms the token's generation against the database, and puts `user_id` in `c.Locals`; `RequireAuthOrKey(iss, versions, keys)` tries the cookie first, then a **full-scope** API key. `RequireAuthOrScopedKey(..., allowed...)` additionally admits the listed narrow scopes — only `/auth/me` mounts it in production (the CV routes take `mw.key` or `mw.cookie`). A live key on a route its scope does not cover is **403** (insufficient scope), not 401.

   **API-key scopes:** `api_keys.scope` is `full` (every key minted today — `mintAPIKey` hardcodes it and the create body has no scope field, so a client cannot choose) or `cv` (a narrow scope reserved for external agents; nothing currently mints one — the tailoring agent now runs in-process and needs no credential — but the schema CHECK, `RequireAuthOrScopedKey`, and the `/auth/me` mount keep the door open). Full-scope-only is the default on every route, so a new endpoint is out of a leaked narrow credential's reach unless it deliberately opts in.

OAuth sign-in (`internal/auth/oauth/`) adds a provider registry (Google/GitHub/LinkedIn/Apple), each implementing the `Provider` interface. The authorization-code flow redirects to `/start` (sets CSRF state cookie), then `/callback` (verifies state, exchanges code, fetches verified email, resolves account, sets JWT cookie, 302s back to SPA). Resolution is identity-first (keyed `user_identities (provider, provider_user_id)`), then verified-email link to existing account, then new passwordless user — all in one transaction. **Never link or create by an unverified email.**

`internal/handler/oauth.go` owns the OAuth HTTP handlers.

### Browser-extension connect flow

The browser extension signs in with `chrome.identity.launchWebAuthFlow`, which
opens `GET /api/v1/auth/extension/connect` **in the freehire origin** and waits for
a redirect to `https://<extension-id>.chromiumapp.org/`. Handlers live in
`internal/handler/extension_connect.go`, both **cookie-only** like key management —
a leaked key must not mint further keys — but mounted on `optionalCookie`:

- `GET` validates `redirect_uri` (`validateExtensionRedirect`: https +
  `<id>.chromiumapp.org` + `<id>` on the allowlist) and renders a consent page.
- `POST` re-validates, then on `decision=allow` **issues a session JWT**
  (`Issuer.Issue`) and 302s to `redirect_uri#token=…&state=…` — the token rides the
  **fragment**, never the query, so it is not logged or sent in `Referer`. Any
  other decision issues nothing and 302s `#error=access_denied`.

**A sessionless visitor is sent to sign in, not refused.** Chrome's auth window
does not share the browsing profile's cookie jar, so arriving with no session is
the normal first run — and a 401 body there is what Chrome reports as
*"Authorization page could not be loaded"*, with no way forward. Both handlers 302
to the web app's `/extension/connect` page instead, carrying `redirect_uri` and
`state`; that page signs the visitor in through the usual `?auth=required` dialog
and hands the window back with `via=web`. That marker is the loop stop: back here
with it and still no session, the endpoint answers a plain HTML "not signed in"
page rather than bouncing again. `redirect_uri` is validated **before** the bounce,
so a crafted target never rides a sign-in round trip.

The redirect target is bounded by `EXTENSION_REDIRECT_ALLOWLIST` (comma-separated
extension ids, parsed in `internal/config`); an empty allowlist disables the flow.

**Unified credential.** The extension holds one token — the session JWT — and it
authenticates everywhere via the shared HS256 secret: hire endpoints accept it as
`Authorization: Bearer <jwt>` (`RequireAuthOrKey`/`OptionalAuth` try the bearer as a
JWT first, then as an API key; a JWT bearer is a full session, so `ViaAPIKey` is
false), and the agent (Roy) verifies the same JWT (cookie / WS-subprotocol). No
`/me/agent-token` bridge, no Roy change. Trade-off: a JWT is short-lived and not
individually revocable — it is re-minted by re-running connect (the freehire origin
still has the cookie). Cookie-only endpoints (`RequireAuth`: key management, profile
edit) stay cookie-only — the extension's JWT does not reach them.

This replaced the flow's original behaviour, which minted an ordinary named API
key: one credential that both hire and Roy verify is simpler than a key plus a
token bridge, at the cost of per-token revocation.

## Limitations
- No refresh tokens (a session is re-minted on sign-in). Revocation exists: `POST /auth/logout-all` bumps `users.token_version`. The cost is one primary-key lookup per authenticated request; `auth.TokenVersionLoader` is the seam a cache would drop into.
- No CSRF token — only `SameSite=Lax` + same-origin defense; a CSRF token is needed only if a future need forces `SameSite=None`.
- Identity unlinking/management UI.
- Magic-link sign-in.
