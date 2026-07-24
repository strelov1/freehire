## Why

The freehire browser extension must act as the signed-in hire user — read their
profile, CV, and job pipeline — but a `chrome-extension://` side panel is a
foreign origin that cannot read hire's `HttpOnly` session cookie. It needs a way
to obtain a durable credential of its own without ever handling the cookie, and
without the user re-typing a password when they already have a live hire session.

## What Changes

- Add a **cookie-authenticated connect flow** the extension drives via
  `chrome.identity.launchWebAuthFlow`. Because it runs in the `freehire.me`
  origin, an already-signed-in user reaches it with their session intact.
- On explicit user consent, the server **mints a named API key** (reusing the
  existing `api_keys` infrastructure — `GenerateAPIKey` + `HashAPIKey`, stored
  hashed, plaintext shown once) and **302-redirects** to the extension's
  `redirect_uri` with the plaintext token in the URL **fragment**.
- The `redirect_uri` is **validated**: it must be an
  `https://<extension-id>.chromiumapp.org/...` URL whose extension id is on a
  configured allowlist. An invalid `redirect_uri` is rejected before any key is
  minted.
- The extension then authenticates existing per-user endpoints (`/auth/me`,
  `/me/...`) with `Authorization: Bearer <token>` — already accepted by
  `RequireAuthOrKey`. The `HttpOnly` cookie is never exposed to extension code.
- Minted keys are ordinary revocable API keys with a recognizable name, so they
  appear in `/me/api-keys` and can be revoked like any other key.

## Capabilities

### New Capabilities
- `extension-auth`: the browser-extension connect flow — a cookie-only endpoint
  that validates an extension `redirect_uri`, obtains user consent, mints a
  named API key, and redirects the token back to the extension via the URL
  fragment; plus the redirect-target allowlist that bounds it.

### Modified Capabilities
<!-- None. api-keys already supports named keys, hashed-at-rest storage, and
     Bearer authentication (RequireAuthOrKey); the connect flow only creates a
     normal key, so no api-keys requirement changes. -->

## Impact

- **New code:** one HTTP handler (connect + consent + mint + redirect) wired
  under the auth surface in `internal/handler`, behind `RequireAuth` (cookie
  only, like other key management); reuses `internal/auth` `GenerateAPIKey` /
  `HashAPIKey` and the existing `CreateAPIKey` query.
- **Config:** an allowlist of permitted extension ids (or exact
  `chromiumapp.org` redirect origins) via env, consumed by `internal/config`.
- **No schema change:** `api_keys` already has a `name` column; no migration.
- **No change to** `RequireAuthOrKey`, `/auth/me`, or the cookie transport.
- **Out of scope (later specs / separate repo):** the extension client code
  (`launchWebAuthFlow`, token storage, authed fetch), the agent/chat, page
  context, and form-filling.
