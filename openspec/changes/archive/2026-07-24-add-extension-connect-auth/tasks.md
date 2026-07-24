## 1. Config: extension redirect allowlist

- [x] 1.1 Write a failing unit test for parsing `EXTENSION_REDIRECT_ALLOWLIST` (comma-separated extension ids → normalized set; empty/whitespace entries dropped)
- [x] 1.2 Add the field + parsing to `internal/config` to pass the test
- [x] 1.3 Thread the allowlist into the handler App dependencies (wherever config already flows into `internal/handler`)

## 2. Redirect validation (pure)

- [x] 2.1 Write failing unit tests for `validateExtensionRedirect(redirectURI, allowlist)`: accepts `https://<allowlisted-id>.chromiumapp.org/…`; rejects wrong scheme, wrong host, unknown extension id, and malformed URLs
- [x] 2.2 Implement `validateExtensionRedirect` to pass the tests (pure function, no Fiber context)

## 3. Connect handler

- [x] 3.1 Implement `GET /api/v1/auth/extension/connect` behind `RequireAuth`: validate `redirect_uri` (400 on invalid, no key minted), then render a minimal consent page (Allow / Cancel) carrying `redirect_uri` and `state`
- [x] 3.2 Implement `POST /api/v1/auth/extension/connect` behind `RequireAuth`: on Allow, mint a named API key via the existing `CreateAPIKey` facility and `302` to `redirect_uri#token=<plaintext>&state=<state>`; on Cancel, `302` to `redirect_uri#error=access_denied&state=<state>` and mint nothing
- [x] 3.3 Wire both routes into the auth route group in `internal/handler/handler.go` with `RequireAuth`

## 4. Integration tests (end-to-end flow)

- [x] 4.1 Session-only: anonymous request → `401`; `Authorization: Bearer <key>` with no cookie → `401`; neither mints a key
- [x] 4.2 Redirect validation: a non-allowlisted / malformed `redirect_uri` → `400`, no key minted
- [x] 4.3 Approve: signed-in `POST` with allowlisted `redirect_uri` and `state=abc` → `302` whose `Location` fragment carries the plaintext token and `state=abc`; a named key is created for that user
- [x] 4.4 Decline: signed-in Cancel → `302` with an error in the fragment and no token; no key minted
- [x] 4.5 Token is a real key: the minted token authenticates a per-user endpoint via `Authorization: Bearer`; it appears in `/me/api-keys`; revoking it stops it authenticating

## 5. Docs

- [x] 5.1 Document the connect flow and `EXTENSION_REDIRECT_ALLOWLIST` in `internal/auth/AGENTS.md` (and the config env list in the root `CLAUDE.md`/`AGENTS.md` if that is where env vars are catalogued)
