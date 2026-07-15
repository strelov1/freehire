# internal/auth/oauth — OAuth Sign-In

Provider registry over the same cookie session as password login.

## Authorization-Code Flow

1. `GET /api/v1/auth/oauth/:provider/start` — sets 10-minute httpOnly CSRF `state` cookie, redirects to provider
2. `.../callback` — verifies state, exchanges code, fetches identity (id + **verified** email), resolves account, sets same JWT session cookie as password login, 302s back to SPA
3. Failures → 302 with `?auth_error=oauth`, never JSON (details go to server log)

## Identity Resolution

`user_identities (provider, provider_user_id) → user_id`; resolution is:
1. **Identity-first** — a later provider-email change never re-keys the account
2. **Verified-email link** to existing account
3. **New passwordless user** (`password_hash` NULL) — last two in one transaction
4. **Never link or create by unverified email** (account-takeover vector)

## Provider Implementations

- Google/LinkedIn: OIDC-userinfo implementation (shared)
- GitHub: reads `/user` + `/user/emails`
- `internal/auth/oauth` owns `Provider` interface, registry (`NewRegistry`), state cookie
- Handlers in `internal/handler/oauth.go`

## Config

- `OAUTH_<PROVIDER>_CLIENT_ID`/`_CLIENT_SECRET` (GOOGLE/GITHUB/LINKEDIN)
- Provider enabled only when both are set
- `GET /api/v1/auth/oauth/providers` lists enabled ones (SPA renders buttons from it)
- Redirect URLs derive from `FRONTEND_ORIGIN` (`<origin>/api/v1/auth/oauth/<p>/callback`)
- Provider tokens used once to fetch identity, never stored

## Limitations

- Identity unlinking/management UI, magic-link sign-in
