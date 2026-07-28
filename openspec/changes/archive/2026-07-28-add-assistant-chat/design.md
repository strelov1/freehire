## Context

The `freehire-agent` backend (a trimmed fork of `roy`) runs two processes:
`roy serve` (daemon, Unix socket) and `roy management` (axum HTTP on `:8079`).
Management exposes `POST /auth/login` (returns a JWT cookie **and** a
`ws_token`) and a `GET /ws` WebSocket relay that transparently bridges the
browser to the daemon over the raw roy control protocol
(`ClientCommand` → daemon, `ServerEvent` ← daemon, one JSON per message).
roy-web already implements the browser half of this protocol
(`workspace/src/lib/wire.ts` + `client.ts`) as a plain Svelte SPA. freehire's
web is SvelteKit SSR; the chat is a new authed route consuming the same
protocol.

## Goals / Non-Goals

**Goals:**
- A working, streamed chat at `/my/assistant` against the agent backend.
- Reuse the proven roy control-protocol client rather than reinventing it.
- Keep the transport contract identical to roy-web (subprotocol auth, verbatim
  JSON lines) so the backend needs no changes.
- Pure-logic pieces (wire types, the `RoyClient`, the event→view reducer) are
  unit-testable; components are verified by `svelte-check` + a visual pass.

**Non-Goals:**
- Giving the agent access to freehire data (MCP tools / prompt context).
- Sharing freehire's session cookie with the agent backend (separate login for
  now).
- Production wiring / host-2 deployment.

## Decisions

- **Port, don't rewrite.** Copy `wire.ts` and `client.ts` from roy-web into
  `web/src/lib/assistant/`, adapting imports/style. A separate pure reducer
  (`reduceTurnEvent`) folds streamed `TurnEvent`s into the message-list view
  model, mirroring how freehire isolates `reduceFitEvent` in `jobFit.ts` — so
  the streaming logic is unit-tested without a DOM.
- **Same-origin via dev proxy.** Add `/assistant-api` to `web/vite.config.ts`
  proxying to `127.0.0.1:8079` with `ws: true`, so `/assistant-api/ws` upgrades
  reach the backend `/ws`. The browser connects to a same-origin relative URL;
  prod wiring is a later seam (nginx location), noted but not built.
- **Session lifecycle in the page.** On mount: login to the agent backend
  (MVP: its own `roy-auth`), obtain `ws_token`, open the WS with
  `[roy-jwt, ws_token]`, spawn a `claude` session, subscribe to its frames.
  Submitting a message sends a `Send` command; the reply streams as frames until
  a terminal `Result`.
- **Auth is the backend's own for the MVP.** The page performs a separate login
  against `freehire-agent`. Sharing freehire's JWT (common secret + cookie) is
  deliberately deferred to keep this change frontend-only.
- **Graceful degradation.** A backend/WS failure yields a non-fatal error state;
  the reducer ignores unmodeled event kinds instead of throwing.

## Risks / Trade-offs

- **Separate login is clunky UX.** Two logins (freehire + agent) until shared
  auth lands. Accepted for the MVP; called out as the top follow-up seam.
- **Dev-proxy only.** Local dev works same-origin; production needs an nginx
  location for `/assistant-api` on host-2 (out of scope here).
- **Ported code drift.** `wire.ts`/`client.ts` are copied from roy-web and can
  diverge from the backend protocol over time; the backend protocol is the
  source of truth and the port is thin, so drift risk is low and localized.
- **Component test coverage.** freehire has no component test runner; the chat
  UI relies on `svelte-check` + visual verification, with logic coverage
  concentrated in the pure reducer/client.
