## Context

freehire already has two bearer-credential surfaces that this design sits beside rather than replaces:

- **Session cookies** (`internal/identity/auth`, `auth.Issuer`): a JWT carrying `user_id` and a `token_version` claim. `GetUserTokenVersion`/`BumpUserTokenVersion` on the `users` table is the account's "session generation" — `LogoutAll` and password reset/change bump it, which instantly invalidates every outstanding cookie without a token denylist.
- **API keys** (`internal/api/handler/api_keys.go`, table `api_keys`): opaque, SHA-256-hashed, scoped (`full`/`cv`), minted once and shown once.

There is also an existing anonymous MCP server (`internal/api/mcpapp`, mounted at `/api/v1/mcp`) that exposes four read-only catalogue tools to any caller, and an in-app assistant (`internal/ai/assistant`, wired up in `internal/api/handler/assistant_tools.go` / `assistant_tracking_tools.go`) whose tool set already does everything freehire#3114 asks a signed-in MCP server to do: read the profile, search, read one job/company, market fit, save/unsave, mark applied, set a tracking stage/note, and list tracked jobs.

freehire#3114 asks for a third credential surface — OAuth 2.1 so an MCP client (Claude, Cursor, ChatGPT) can act as an account without a human pasting an API key.

## Goals / Non-Goals

**Goals:**
- A client can register itself, send a human through a consent screen, and receive a bearer token — no client secret, no pre-provisioned client_id, no pasted key.
- A granted token is revocable two ways: explicitly (the "Connected devices" list) and implicitly (password reset / sign-out-everywhere, for free, via the existing `token_version` counter).
- The signed-in MCP server's tools are the *same code path* as the in-app assistant's tools — a result can never disagree between the two surfaces.
- Standard enough that Claude Desktop, Cursor, and ChatGPT's MCP clients can connect without freehire-specific client configuration (RFC 7591 dynamic registration, RFC 8414/9728 metadata discovery, RFC 8252 loopback redirects).

**Non-Goals:**
- Refresh tokens / token rotation. The issue specifies a 30-day expiry, not a renewing one; re-running consent is the renewal path. Revisit only if a real client cannot tolerate a 30-day hard stop.
- Per-tool or per-scope grants. One fixed scope (`mcp:account`) covering every reused assistant tool. A capability matrix is not asked for and multiplies the consent UI and the grant schema for no current requirement.
- Client vetting or an approval queue for dynamic registration. Registration is open (anyone can mint a `client_id`); the security boundary is the human's consent click, not who may request a `client_id` — identical to how any public OAuth client registry works (Linear, GitHub Apps' device flow, etc.).
- Changing the existing anonymous `/api/v1/mcp` server or the API-key surface. Both are untouched; this is a third, additive credential path.

## Decisions

**Grants ride `users.token_version`, not a bespoke revocation list.**
An `oauth_grants` row stores `issued_token_version` — the user's counter value at consent time. `AuthenticateOAuthGrant` requires it still equal the user's *current* counter. `LogoutAll` and password reset/change already bump that counter for cookies; this makes them *also* revoke every MCP grant, with zero new invalidation code. Alternative considered: a separate `revoked_at` cascade triggered from the password-reset transaction — rejected because it is a second revocation mechanism to keep in sync with the first, for a system that already does the job.

**No refresh tokens — a single opaque 30-day access token.**
Alternative considered: `authorization_code` + refresh-token rotation (the common OAuth pattern). Rejected for now: it doubles the token schema and the token endpoint's grant-type branching for a requirement ("expire after 30 days") that does not ask for silent renewal. A client that wants to keep working past 30 days re-runs the consent flow, which is also a periodic reminder of what has access.

**The signed-in MCP server adapts the assistant's existing `assistant.Tool` registry rather than re-declaring tools.**
`assistant.Tool{Name, Description, Schema map[string]any, Run func(ctx, userID, json.RawMessage) (any, error)}` already matches the MCP Go SDK's untyped path: `(*mcp.Server).AddTool(t *mcp.Tool, h mcp.ToolHandler)` takes `InputSchema any` (accepts a raw `map[string]any`) and a handler reading `req.Params.Arguments json.RawMessage` — the same shape `Tool.Run` wants. A thin adapter (`internal/api/mcpapp/account.go`) closes over a per-request resolved `userID` and calls `Run` directly. Alternative considered: hand-declare each tool with the SDK's typed generic `AddTool[In, Out]` (as the anonymous `/api/v1/mcp` server already does) — rejected because it would duplicate every schema and handler the assistant tools already define, and the two copies would drift.

**Redirect URI matching treats loopback specially.**
Exact string match, except `127.0.0.1`/`[::1]`/`localhost` redirects match on scheme+host+path only, ignoring port (RFC 8252 §7.3) — native MCP clients bind an ephemeral local port per run and cannot pre-register it.

**Consent is server-rendered HTML, not a Svelte page.**
Mirrors the existing browser-extension connect flow (`internal/api/handler/extension_connect.go`): `GET` renders a minimal HTML form directly from the Go handler, `POST` (cookie-gated) acts on the decision. This is a one-screen, low-traffic, security-sensitive surface — round-tripping it through the SPA build buys nothing a `fmt.Sprintf` template doesn't already give.

**New package name is `oauth2server`, not `oauth`.**
`internal/identity/auth/oauth` already names the *social-login client* package (resolving Google/GitHub identities inbound). The new package is the opposite direction — freehire acting as an authorization server — and shares no code with it; a distinct name avoids import confusion at every call site.

## Risks / Trade-offs

- **[Open dynamic client registration is abusable as a client-id mint]** → Mitigated by design: a minted `client_id` grants nothing by itself. Every token still requires a human to complete the consent screen for their own account; an attacker who registers a client still cannot obtain a token without a victim's affirmative click, which is the same trust boundary Linear's and other MCP OAuth implementations rely on.
- **[No refresh tokens means a 30-day hard cliff could surprise a long-running integration]** → Mitigated by `expires_at` being visible in the Connected devices list before it lapses; revisit if real usage shows this is disruptive rather than a reasonable security boundary.
- **[Reusing `assistant.Tool` ties the MCP account server's behavior to the assistant's]** → This is the intended coupling (goal: no divergent second implementation), but means a future assistant-only tool change must consider this second caller. Mitigated by both call sites sharing one registry-building function (`assistantDiscoveryTools`/`assistantTrackingTools`), so a signature change is a single compile-time break, not a silent drift.
- **[A stored `code_challenge` without length/charset validation could be abused to smuggle oversized input]** → Mitigated at the token endpoint: `VerifyPKCE` does a fixed-size SHA-256 comparison regardless of input length, and the column is `TEXT` with no application-level ceiling needed for a base64url-encoded 32-byte hash.

## Migration Plan

1. Ship migration `0177` (new tables only — additive, no existing table touched, safe to deploy ahead of the handler code).
2. Ship the Go handlers and the `oauth2server` package behind no feature flag — every new route is net-new (`/api/v1/oauth/*`, `/api/v1/mcp/account`, `/.well-known/oauth-*`) and additive endpoints on `/me`; nothing existing changes behavior.
3. Ship the frontend `ConnectedDevicesView` last, once the backend list/revoke endpoints are live, so the page never shows a 404 while loading.
4. No rollback complexity beyond a normal revert: the new tables have no foreign keys pointing *into* them from pre-existing tables, so dropping them back out (if ever needed) cannot orphan unrelated data.

## Open Questions

- Should the consent page's copy enumerate the exact tool list (as `design.md`'s current draft text does) or stay generic ("search jobs and manage your tracked applications")? Leaning generic to avoid the copy rotting every time a tool is added to the assistant's registry — revisit during Task 5's implementation.
- Does freehire want to log grant creation/revocation as an account-security event (the way login/logout might be)? Not currently wired; flag for a follow-up change if audit visibility is wanted beyond the Connected devices list itself.
