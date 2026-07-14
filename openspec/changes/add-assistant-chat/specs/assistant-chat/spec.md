## ADDED Requirements

### Requirement: Assistant chat page

The freehire web app SHALL provide a `/my/assistant` route that lets a
logged-in user hold a streamed conversation with a Claude agent served by the
`freehire-agent` backend. The page SHALL present a message list and a composer
input, and SHALL render assistant replies as they stream.

#### Scenario: Opening the assistant page

- **WHEN** an authenticated user navigates to `/my/assistant`
- **THEN** the page connects to the agent backend, establishes a chat session,
  and shows a ready-to-type composer with an empty (or resumed) message list

#### Scenario: Sending a message and streaming the reply

- **WHEN** the user submits a message in the composer
- **THEN** the message appears immediately in the list and the assistant's
  reply is appended incrementally as `TurnEvent`s arrive, ending when a
  terminal `Result` event is received

#### Scenario: Backend unreachable

- **WHEN** the agent backend cannot be reached or the WebSocket fails to open
- **THEN** the page shows a non-fatal error state and the rest of freehire keeps
  working (the failure does not break navigation or other `/my` pages)

### Requirement: Agent-backend WebSocket transport

The web app SHALL communicate with the agent backend over a WebSocket that
speaks the roy control protocol verbatim: the client sends `ClientCommand`
JSON and receives `ServerEvent` JSON, one JSON value per message. The client
SHALL authenticate by offering the `Sec-WebSocket-Protocol` values
`roy-jwt` (marker) and the login-issued `ws_token`, and SHALL never place the
token anywhere the browser would expose it beyond that subprotocol slot.

#### Scenario: Authenticated upgrade

- **WHEN** the client opens the WebSocket after a successful agent-backend login
- **THEN** it offers `[roy-jwt, <ws_token>]` as subprotocols and, on a successful
  upgrade, streams commands and events over the socket

#### Scenario: Connection lost mid-session

- **WHEN** the WebSocket closes or errors while a session is open
- **THEN** the client surfaces a disconnected status and any awaiting command
  calls reject rather than hanging indefinitely

### Requirement: Event rendering vocabulary

The chat SHALL render the streamed `TurnEvent` kinds it understands —
assistant text, assistant thinking, tool use, and the terminal result — and
SHALL degrade gracefully for unmodeled events (showing nothing or a raw
fallback) rather than crashing.

#### Scenario: Assistant text and thinking

- **WHEN** `AssistantText` and `AssistantThought` events stream in
- **THEN** assistant text is rendered as the reply and thinking is shown as
  distinct, secondary content (not mixed into the final answer)

#### Scenario: Unknown event kind

- **WHEN** an event of an unmodeled kind arrives
- **THEN** the reducer ignores it (or shows a raw fallback) without throwing

### Requirement: Development proxy to the agent backend

The web dev server SHALL proxy `/assistant-api` to the agent backend at
`127.0.0.1:8079`, including WebSocket upgrades for `/assistant-api/ws`, so the
SPA and agent backend are same-origin in local development.

#### Scenario: Proxied WebSocket in dev

- **WHEN** the SPA opens `/assistant-api/ws` during local development
- **THEN** the Vite dev server upgrades and forwards the connection to the agent
  backend's `/ws` endpoint
