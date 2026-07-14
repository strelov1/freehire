## 1. Wire protocol + client (ported, unit-tested)

- [ ] 1.1 Port `wire.ts` (control-protocol types: `ClientCommand`, `ServerEvent`,
      `TurnEvent`/`JournalEntry`, `WS_SUBPROTOCOL_MARKER`) from roy-web into
      `web/src/lib/assistant/wire.ts`, adapted to freehire's TS style.
- [ ] 1.2 Port `client.ts` (`RoyClient` WS client: connect/close/call/fire/
      subscribeFrames/onStatus) into `web/src/lib/assistant/client.ts`.

## 2. Event reducer (pure logic, TDD)

- [ ] 2.1 RED: write `web/src/lib/assistant/chat.test.ts` for `reduceTurnEvent`
      — folds a stream of `TurnEvent`s into a message-list view model
      (assistant text accumulates; thinking is separate; terminal `Result`
      closes the turn; unknown kinds are ignored).
- [ ] 2.2 GREEN: implement `reduceTurnEvent` in `web/src/lib/assistant/chat.ts`
      until 2.1 passes.

## 3. Chat page

- [ ] 3.1 Add route `web/src/routes/my/assistant/+page.svelte`: message list +
      composer; on mount login → `ws_token` → open `/assistant-api/ws` with
      `[roy-jwt, ws_token]` → spawn a `claude` session → subscribe frames.
- [ ] 3.2 Wire the composer: submit sends a `Send` command, optimistic user
      message, stream reply via `reduceTurnEvent`; render text/thinking/tool-
      use/result; show a non-fatal error state on backend/WS failure.
- [ ] 3.3 Add the assistant entry to the `/my` navigation surface.

## 4. Dev proxy

- [ ] 4.1 Add `/assistant-api` proxy (with `ws: true`) → `127.0.0.1:8079` in
      `web/vite.config.ts`.

## 5. Unified auth (supersedes the separate agent login)

- [x] 5.1 Backend (`freehire-agent`): verify freehire's `hire_token` cookie
      (shared secret, `ROY_COOKIE_NAME`), WS auth from the handshake cookie,
      single server-side harness credential, shadow-user provisioning.
- [x] 5.2 Frontend: drop the inline agent-login form; WS opens with no
      subprotocol token (freehire cookie carries auth); `createSession` via
      cookie.

## 6. Verify

- [x] 6.1 `npm run check` clean (no new errors); `reduceTurnEvent` tests green.
- [x] 6.2 Local end-to-end: with `roy serve` + `roy management` + a
      `claude-code-acp` on PATH, one freehire login → `/my/assistant` streams a
      real reply (verified: Claude replied "hi"/"pong").
- [ ] 6.3 Port roy-web message rendering (markdown, tool-use cards, polished
      design) into the assistant page.
