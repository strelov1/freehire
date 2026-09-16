# internal/identity/auth/oauth — OAuth Sign-In

Provider registry over the same cookie session as password login.

## Authorization-Code Flow

1. `GET /api/v1/auth/oauth/:provider/start` — sets 10-minute httpOnly CSRF `state` cookie (`SameSite=None` when secure — Apple's callback is a cross-site POST, which never carries a Lax cookie; see `state.go`), redirects to provider
2. `.../callback` — verifies state, exchanges code, fetches identity (id + **verified** email), resolves account, sets same JWT session cookie as password login, 302s back to SPA
3. Failures → 302 with `?auth_error=oauth`, never JSON (details go to server log)

**One registered callback, two flows.** `Provider` (sign-in) and `ProviderV2`
(re-authentication, and the verifier-bound native flow) build the SAME redirect URL,
`<origin>/api/v1/auth/oauth/<name>/callback`, and a test asserts they never drift apart.
A provider console has room for one callback URL per application, so a second path is not
a second option — it is a redirect the provider has never been told about. `ProviderV2`
used to answer `/api/v2/...`: GitHub refused it outright ("The redirect_uri is not
associated with this application"), and only Google worked, because somebody had listed
both there and nowhere else. The failure was invisible for months because the buttons
that start that flow only appeared after the server had already refused an action.

`OAuthCallback` decides which flow a return belongs to by **which state cookie the browser
brings back** — `hire_oauth_state` for sign-in, `hire_oauth_v2_state` for the other — and
hands over to `OAuthCallbackV2` when the second matches the state the provider returned.
The dispatch is gated on `AUTH_V2_ENABLED`, the same flag that registers the v2 routes:
with it off no such attempt can exist, so a cookie of that name proves nothing. The
`/api/v2/.../callback` routes stay registered so an attempt started before a deploy still
completes.

## Mobile Flow

`/start?platform=mobile` sets a short-lived platform cookie (`state.go`); the callback then redirects to the app's custom scheme carrying a one-time code instead of setting the session cookie, and the app redeems it at `POST /api/v1/auth/oauth/exchange` for a session. See [docs/auth-mobile-v2-runbook.md](../../../../docs/auth-mobile-v2-runbook.md).

## Identity Resolution

`user_identities (provider, provider_user_id) → user_id`; resolution is:
1. **Identity-first** — a later provider-email change never re-keys the account
2. **Verified-email link** to existing account
3. **New passwordless user** (`password_hash` NULL) — last two in one transaction
4. **Never link or create by unverified email** (account-takeover vector)

## Provider Implementations

- Google/LinkedIn: OIDC-userinfo implementation (shared)
- GitHub: reads `/user` + `/user/emails`
- Apple: no userinfo endpoint and no static client secret — see below
- `internal/identity/auth/oauth` owns `Provider` interface, registry (`NewRegistry`), state cookie
- Handlers in `internal/api/handler/oauth.go`

## Config

- `OAUTH_<PROVIDER>_CLIENT_ID`/`_CLIENT_SECRET` (GOOGLE/GITHUB/LINKEDIN)
- Apple instead: `OAUTH_APPLE_CLIENT_ID` (its Services ID) + `OAUTH_APPLE_TEAM_ID`/`_KEY_ID`/`_PRIVATE_KEY`; enabled only when all four are set. `_PRIVATE_KEY` is the `.p8` key **base64-encoded** (a multi-line PEM does not survive a systemd `EnvironmentFile` reliably — same convention as `GMAIL_TOKEN_KEY`); `config.loadOAuth` decodes it
- `GET /api/v1/auth/oauth/providers` lists enabled ones (SPA renders buttons from it)
- Redirect URLs are per-request: an exact `SERVED_HOSTS` match on the request Host wins, `FRONTEND_ORIGIN` is only the fallback (`requestOrigin` in `internal/api/handler/oauth.go`); the registry builds the provider for that origin via `Registry.Provider(name, origin)` → `<origin>/api/v1/auth/oauth/<p>/callback`
- Provider tokens used once to fetch identity, never stored

## Apple's Different Trust Model

- No static client secret: `apple.go` signs a short-lived (5-minute) ES256 JWT fresh for every token exchange, from Team ID + Key ID + the private key — never cached, nothing to rotate on Apple's 6-month schedule
- No userinfo call: identity comes only from the token exchange's `id_token`. Its signature is verified against Apple's JWKS (`appleid.apple.com/auth/keys`), and its `aud`/`iss` are checked against our client id / Apple's issuer — this is the one place in the package that verifies a token signature itself, since every other provider's userinfo GET is its own trust boundary
- Requesting the `email` scope forces `response_mode=form_post`, so Apple's callback arrives as a `POST` with a form-encoded body, not the `GET` query string every other provider uses — `internal/api/handler/oauth.go`'s callback route must accept both

## Limitations

- Identity unlinking/management UI, magic-link sign-in
