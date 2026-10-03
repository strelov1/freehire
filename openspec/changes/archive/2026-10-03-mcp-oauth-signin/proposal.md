## Why

MCP clients (Claude, Cursor, ChatGPT) can only act as a freehire account today by having a human mint an API key and paste it into a config file — an insecure, manual step that also burns an agent's turns figuring out how to authenticate (freehire#3114). A browser-based OAuth 2.1 consent flow removes the pasted secret entirely: the agent opens a login window, the human clicks "Allow," and the connection is both easier and more secure (narrower, revocable, visible in an account-level list) than a bearer key with no expiry UI.

## What Changes

- Add an OAuth 2.1 authorization server (dynamic client registration per RFC 7591, PKCE-only authorization_code grant, no client secret) so an MCP client can register and connect without a human provisioning it a client_id in advance.
- Add authorization-server and protected-resource metadata discovery (`/.well-known/oauth-authorization-server`, `/.well-known/oauth-protected-resource`) so a client finds the right endpoints without them being hardcoded.
- Add a consent page (`GET` shows it, `POST` decides) that issues a single-use authorization code on approval, modeled on the existing browser-extension connect flow.
- Add a token endpoint that exchanges a code + PKCE verifier for a 30-day bearer access token. No refresh tokens — re-running consent renews it.
- Tie every grant to the account's existing session-generation counter (`users.token_version`, the same one `LogoutAll` and password reset already bump), so a password reset or sign-out-everywhere revokes every MCP grant with no new cascade logic.
- Add a "Connected devices" list on the account's API-keys page: each grant's client name, created/expires/last-used, and a revoke action — mirroring the existing API-key management UI.
- Add a signed-in MCP server at `/api/v1/mcp/account`, gated by the new bearer tokens, that acts as the account: who-you-are, search, read one posting, read one employer, market fit, save/unsave a job, mark applied, set a tracking stage or note, and list tracked jobs. These tools are not reimplemented — the server reuses the in-app assistant's existing tool set verbatim.

## Capabilities

### New Capabilities
- `mcp-oauth-authorization`: the OAuth 2.1 authorization server — dynamic client registration, metadata discovery, the consent-page authorize flow, the PKCE token exchange, and the Connected devices list/revoke.
- `mcp-account-server`: the signed-in MCP tool surface at `/api/v1/mcp/account`, bearer-gated by `mcp-oauth-authorization`'s grants, exposing the account-acting tools (search, job/company reads, market fit, save/unsave, mark applied, stage/note, list tracked jobs).

### Modified Capabilities
(none — `api-keys` is not changing requirements; the new grant-management surface is a sibling feature on the same page, captured entirely under `mcp-oauth-authorization`.)

## Impact

- **Database:** three new tables (`oauth_clients`, `oauth_authorization_codes`, `oauth_grants`), migration `0177`.
- **New Go package:** `internal/identity/auth/oauth2server` (PKCE verification, opaque token/code generation, redirect-URI matching).
- **New/modified handlers:** `internal/api/handler/oauth_register.go`, `oauth_metadata.go`, `oauth_authorize.go`, `oauth_token.go`, `oauth_grants.go`, `mcpapp_account.go`; `auth.go` gains new routes.
- **New MCP package code:** `internal/api/mcpapp/account.go` (adapts the existing `assistant.Tool` registry into an MCP server).
- **Reused, unmodified:** `internal/api/handler/assistant_tools.go` and `assistant_tracking_tools.go` (the tool implementations), `internal/identity/auth` session/token-version machinery.
- **Frontend:** one new Svelte component (`ConnectedDevicesView.svelte`) mounted on the existing `/my/api-keys` page; no new route (the consent page is server-rendered HTML, not SPA).
- **No breaking changes:** existing API-key auth, the public anonymous `/api/v1/mcp` server, and session-cookie auth are all unaffected.
