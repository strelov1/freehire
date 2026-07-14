## Why

freehire has no in-app conversational surface. A trimmed `roy` backend
(`freehire-agent`) now exposes a chat over a `/ws` WebSocket relay, but there is
no way for a user to reach it from the freehire web app. This change adds the
front-end chat page so a logged-in user can talk to a Claude agent inside
freehire. It is the MVP slice: prove a working streamed chat. What the agent can
*access* (freehire data, tools) is intentionally deferred.

## What Changes

- Port the roy-web control-protocol wire types (`wire.ts`) and WebSocket client
  (`client.ts` → `RoyClient`) into `web/src/lib`, adapted to freehire's style.
- Add a `/my/assistant` route: a simple chat (message list + composer) that logs
  into the `freehire-agent` backend, opens `/ws` with the
  `[roy-jwt, ws_token]` subprotocol, spawns a `claude` session, streams
  `TurnEvent`s, and renders assistant text / thinking / tool-use / result.
- Add a Vite dev proxy `/assistant-api` → `127.0.0.1:8079` (the agent backend),
  including WebSocket upgrade support for `/assistant-api/ws`.
- Auth is **unified with freehire**: the agent backend verifies the same
  `hire_token` session cookie (shared JWT secret, stateless `sub` trust) and
  provisions a passwordless shadow user for the FK — so there is **no separate
  agent login**. The WebSocket authenticates from that cookie on the
  same-origin upgrade (not a subprotocol token). The single claude credential is
  a server-side env var, so there is no per-user harness onboarding.

Out of scope (later seams): giving the agent access to freehire data
(MCP/context), the prod cookie `Domain=.freehire.dev` for `agent.freehire.dev`,
and host-2 deployment.

## Capabilities

### New Capabilities
- `assistant-chat`: an in-app chat page that streams a conversation with a
  Claude agent served by the separate `freehire-agent` backend over a WebSocket
  relay.

### Modified Capabilities
<!-- none: no existing freehire capability's requirements change -->

## Impact

- **Frontend only** (the `hire` repo): new `web/src/lib/assistant/` (wire +
  client + event reducer), new route `web/src/routes/my/assistant/`, and a
  `web/vite.config.ts` proxy entry. No Go backend changes.
- Depends on the separately-built `freehire-agent` service (roy `serve` +
  `management` on `:8079`); not wired into freehire's deploy yet.
- New dev-only config: the agent backend base URL (proxied at `/assistant-api`).
