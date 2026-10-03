Full TDD steps (failing test → implementation → passing test, with real code) for every task below are already written out in `docs/superpowers/plans/2026-10-03-mcp-oauth.md` — follow that file's matching "Task N" section for exact file contents; this list is the tracked checklist, not a restatement.

## 1. Database schema

- [x] 1.1 Add migration `0177_mcp_oauth.sql` (`oauth_clients`, `oauth_authorization_codes`, `oauth_grants`) and the sqlc queries (`oauth_clients.sql`, `oauth_grants.sql`); generate and verify with an integration test that a grant stops authenticating once `BumpUserTokenVersion` runs

## 2. OAuth2 server primitives (`internal/identity/auth/oauth2server`)

- [x] 2.1 `VerifyPKCE(challenge, verifier string) bool` — S256 only
- [x] 2.2 `GenerateAccessToken`/`GenerateAuthorizationCode`/`HashToken` — opaque, prefixed (`fhm_`/`fhc_`), SHA-256 at rest
- [x] 2.3 `RedirectURIAllowed(registered []string, candidate string) bool` — exact match, with the loopback-port exception (RFC 8252 §7.3)

## 3. Dynamic client registration

- [x] 3.1 `POST /api/v1/oauth/register` (RFC 7591): public, validates `client_name`/`redirect_uris`, returns a server-generated `client_id`

## 4. Metadata discovery

- [x] 4.1 Go: render both documents at internal `/api/v1/oauth/metadata/authorization-server` (RFC 8414) and `/api/v1/oauth/metadata/protected-resource` (RFC 9728) paths
- [x] 4.2 SvelteKit: `/.well-known/oauth-authorization-server` and `/.well-known/oauth-protected-resource` proxy routes, following `web/src/routes/.well-known/ojcp.json/+server.ts`'s exact pattern — nginx sends `/.well-known/*` to the Node process, not the Go backend

## 5. Consent authorize flow

- [x] 5.1 `GET /api/v1/oauth/authorize`: validates the request, 400s an unregistered redirect_uri, shows consent for a signed-in visitor, redirects a sessionless one to sign in and back
- [x] 5.2 `POST /api/v1/oauth/authorize`: re-validates, issues a single-use authorization code (hashed at rest) on `decision=allow`, redirects `access_denied` otherwise — both as a 302 carrying `state`

## 6. Token exchange

- [x] 6.1 `POST /api/v1/oauth/token`: consumes the code exactly once, checks client_id/redirect_uri match, verifies PKCE, stamps the new grant with the user's current `token_version`, returns a 30-day bearer token (no refresh token)

## 7. Connected devices

- [x] 7.1 `GET /api/v1/me/oauth-grants`: cookie-only, lists the caller's own grants (client name, created/expires/last-used — never the token)
- [x] 7.2 `DELETE /api/v1/me/oauth-grants/:id`: cookie-only, recent-auth-gated, owner-scoped delete, 404 for another user's id

## 8. Signed-in MCP account server

- [ ] 8.1 `internal/api/mcpapp/account.go`: `NewAccountServer`/`AccountHandler` adapting `assistant.Tool` → `mcp.Tool`/`mcp.ToolHandler`, scoping every call to the resolved `userID`
- [ ] 8.2 Wire `/api/v1/mcp/account` (bearer-gated via `AuthenticateOAuthGrant`), reusing `assistantDiscoveryTools()` + `assistantTrackingTools()` verbatim — no new tool logic

## 9. Frontend — Connected devices list

- [ ] 9.1 `OAuthGrant` type (`web/src/lib/types.ts`) and `listOAuthGrants`/`revokeOAuthGrant` (`web/src/lib/api.ts`)
- [ ] 9.2 `ConnectedDevicesView.svelte` + messages + spec test, following `ApiKeysView.svelte`'s list/`ConfirmDialog`/revoke pattern exactly
- [ ] 9.3 Mount `ConnectedDevicesView` under `ApiKeysView` on `web/src/routes/my/api-keys/+page.svelte`
