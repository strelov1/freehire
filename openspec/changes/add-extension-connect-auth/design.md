## Context

The freehire browser extension needs a durable credential to call hire as the
signed-in user, but its `chrome-extension://` side panel cannot read hire's
`HttpOnly; SameSite=Lax` session cookie. hire already has everything needed to
issue such a credential: `api_keys` (hashed at rest, `name` column, revocable),
`GenerateAPIKey`/`HashAPIKey`, the `CreateAPIKey` query, and `RequireAuthOrKey`,
which already authenticates `Authorization: Bearer <key>` on per-user endpoints.

The missing piece is a bridge that runs **in the `freehire.me` origin** (so it
sees the session cookie) and hands a freshly minted key back to the extension.
Chrome's `chrome.identity.launchWebAuthFlow` is built for exactly this: it opens
an auth URL and waits for a redirect to `https://<extension-id>.chromiumapp.org/`,
then returns that URL to the extension.

## Goals / Non-Goals

**Goals:**
- Let the extension obtain a hire credential with no password re-entry when a
  session already exists, and a normal login when it does not.
- Never expose the `HttpOnly` session cookie to extension code.
- Reuse the existing API-key facility; no new credential type, no schema change.
- Bound where tokens can be delivered (no open redirect).

**Non-Goals:**
- The extension client (`launchWebAuthFlow`, token storage, authed fetch) —
  separate repo, driven by this endpoint.
- The agent/chat, page context, form-filling — later specs.
- A polished SPA consent screen — a minimal server-rendered page is enough now.
- Token refresh/rotation — the key is long-lived and revocable, like any key.

## Decisions

### D1: The credential is an ordinary named API key
Reuse `api_keys` + `GenerateAPIKey`/`HashAPIKey`/`CreateAPIKey`. The minted key
is created with a recognizable, constant `name` (e.g. `"Browser extension"`) so
it is obvious in `/me/api-keys`. **Why:** the key is already hashed-at-rest,
revocable, and accepted by `RequireAuthOrKey`; a bespoke "extension token" would
duplicate all of that. Alternative (opaque short-lived + refresh) rejected as
premature — no requirement forces rotation yet, and it multiplies surface area.

### D2: Server-rendered consent, backend-only
Two cookie-only routes under the auth surface:
- `GET /api/v1/auth/extension/connect?redirect_uri=…&state=…` — validates the
  redirect, then renders a minimal consent page ("Allow the freehire extension
  to access your account?" with Allow / Cancel).
- `POST /api/v1/auth/extension/connect` (same query carried through) — on Allow,
  mints the key and `302`s to the redirect with the token in the fragment; on
  Cancel, `302`s with an error and no token.

Both use `RequireAuth` (cookie only), matching the rule that key management is
never reachable by an API key. **Why server-rendered:** keeps this change
hire-backend-only (as scoped) and avoids coupling to the SvelteKit SPA build. A
prettier SPA route can replace the page later without changing the contract.

### D3: Token travels in the URL fragment, never the query
The `302` target is `redirect_uri#token=<plaintext>&state=<state>`. **Why:**
fragments are not sent to servers, not written to access logs, and not leaked via
`Referer`. `launchWebAuthFlow` still delivers the full URL (fragment included) to
the extension, which parses the token client-side.

### D4: Strict `redirect_uri` allowlist
Accept only `https://<extension-id>.chromiumapp.org/…` where `<extension-id>` is
on a server-configured allowlist (`EXTENSION_REDIRECT_ALLOWLIST`, parsed in
`internal/config`). Validation happens **before** consent or minting. **Why:**
prevents an open redirect from turning the mint endpoint into a token-exfiltration
gadget. Alternative (any `chromiumapp.org`) rejected — any published extension
could then harvest tokens for a tricked user.

### D5: CSRF posture inherits hire's model
The consent `POST` is same-origin (form served by hire, submitted to hire), so
the `SameSite=Lax` session cookie rides it; a cross-site forced POST carries no
cookie and gets `401`. No CSRF token is added, consistent with the rest of hire.
The `redirect_uri` allowlist is the second layer: even a coerced consent can only
deliver a token to an extension id the operator explicitly trusts.

## Risks / Trade-offs

- **Open redirect / token exfiltration** → strict `chromiumapp.org` + allowlist
  validation before any minting (D4); token in fragment (D3).
- **Forced-consent (CSRF-style)** → `SameSite=Lax` same-origin defense (D5) plus
  the allowlist bounding the blast radius to trusted extension ids.
- **Key accumulation** — each successful connect mints a new key, so repeated
  re-connects leave stale keys. Trade-off accepted for now: they are named,
  visible, and revocable. A later refinement can revoke-and-replace by name.
- **Token in browser history** — the fragment could linger in the auth popup's
  history, but `launchWebAuthFlow` runs in a throwaway auth window that Chrome
  discards; and the token is revocable regardless.

## Migration Plan

- No schema change (`api_keys.name` already exists).
- Deploy: set `EXTENSION_REDIRECT_ALLOWLIST` to the extension id(s). Until the
  extension calls the endpoint, the feature is inert.
- Rollback: remove the routes / unset the env; existing extension keys keep
  working as ordinary API keys and can be revoked normally.

## Open Questions

- Exact key naming and whether to revoke-and-replace by name on re-connect
  (leaning: constant name now, dedup later if it becomes noisy).
- Whether to later promote the consent page to a styled SPA route at
  `/extension/connect` (contract stays identical).
