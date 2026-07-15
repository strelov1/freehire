# Auth conventions

## Scope
Auth primitives: bcrypt password hashing, JWT cookie transport, API-key hashing/auth, and the `RequireAuth`/`RequireAuthOrKey` Fiber middleware.

## Always true
- **JWT is stateless, HS256, carries only `sub` (user id).** It survives new sign-in methods and a later swap to opaque sessions.
- **Transport is an `HttpOnly; SameSite=Lax` cookie**, never a `Bearer` header or `localStorage` — XSS-safe, same-origin CSRF defense (no CSRF token needed yet).
- **SPA and API must be same-origin.** In dev the Vite proxy (`web/vite.config.ts`) forwards `/api` to the backend.
- `users.password_hash` is **nullable** — passwordless sign-in creates accounts with no password; password login rejects a null hash with a generic `401`.
- `email` is the canonical account key (`UNIQUE (lower(email))`); external providers link via `user_identities`.
- `JWT_SECRET` is required at server startup (fail-fast in `cmd/server`); `COOKIE_SECURE=true` for HTTPS (default false for http://localhost dev).
- Credential endpoints (`register`/`login`) are throttled by a per-instance rate limiter (10/min, keyed on client IP).
- **API keys are hashed at rest:** the row stores only the `HashAPIKey(token)` SHA-256 (i.e. `SHA-256(token)`) plus a short non-secret `token_prefix` (enough to tell keys apart in a list); the plaintext (minted by `GenerateAPIKey`) is shown exactly once at create time and is unrecoverable.
- **Key management is cookie-only (`RequireAuth`)** — a leaked key must not be able to create, list, or revoke keys.
- **Per-user job endpoints and `/auth/me` accept either credential** (`RequireAuthOrKey`).

## How it works

`internal/auth` owns four responsibilities:

1. **Password hashing** (`password.go`): bcrypt between register and login.
2. **JWT Issuer** (`token.go`): issues and verifies HS256 tokens carrying only `sub`.
3. **Cookie transport** (`cookie.go`): `SetTokenCookie`/`ClearTokenCookie` with `HttpOnly; SameSite=Lax; Path=/`.
4. **Middleware** (`middleware.go`): `RequireAuth` reads the JWT cookie and puts `user_id` in `c.Locals`; `RequireAuthOrKey` tries the cookie first, falls through to API-key hash lookup.

OAuth sign-in (`internal/auth/oauth/`) adds a provider registry (Google/GitHub/LinkedIn), each implementing the `Provider` interface. The authorization-code flow redirects to `/start` (sets CSRF state cookie), then `/callback` (verifies state, exchanges code, fetches verified email, resolves account, sets JWT cookie, 302s back to SPA). Resolution is identity-first (keyed `user_identities (provider, provider_user_id)`), then verified-email link to existing account, then new passwordless user — all in one transaction. **Never link or create by an unverified email.**

`internal/handler/oauth.go` owns the OAuth HTTP handlers.

## Limitations
- No token revocation/refresh (logout clears the cookie but the JWT lives until `exp`; modest TTL instead).
- No CSRF token — only `SameSite=Lax` + same-origin defense; a CSRF token is needed only if a future need forces `SameSite=None`.
- Identity unlinking/management UI.
- Magic-link sign-in.
