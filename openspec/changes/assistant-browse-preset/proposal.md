## Why

The assistant can search, judge and track vacancies, but it cannot see the page
the candidate is looking at. That is the one thing a browser extension is
positioned to give it — and the extension already serves a browser-tool relay
(`/api/v1/tools/ws`) that the agentic autofill drives today.

The extension's side panel currently runs its chat on a separate agent service
(Roy). Moving it onto this assistant means one agent runtime instead of two, but
the assistant has to be reachable from an extension and has to have eyes on the
page before that move is possible. This change is the backend half; the panel's
half lives in the `freehire-extension` repository and depends on this shipping
first.

## What Changes

- **BREAKING** The five `/api/v1/assistant/*` routes move from cookie-only
  (`mw.cookie`) to `mw.key` (`RequireAuthOrKey`), so a caller presenting a
  session JWT or a full-scope API key as `Authorization: Bearer` is admitted
  alongside the browser's cookie. Breaking only in the sense that the surface
  widens; no existing caller changes.
- A third preset, `browse`, joins `chat` and `tailor`. It carries its own system
  prompt (the candidate is standing on a page — look at it before guessing; the
  panel is a narrow column — answer short) and the discovery and tracking tools
  plus one tool the other presets do not get.
- A new tool, `read_current_page`, returns what the caller's browser is
  displaying — url, title, headline and text — by calling through the existing
  browser-tool relay as an in-process harness.
- `read_current_page` is deliberately NOT registered for `chat`. A web session
  with no extension attached would carry a tool that always fails, and a tool
  that always fails is noise in the model's context.
- Session creation accepts the preset the client wants (`chat` or `browse`;
  `tailor` stays reachable only through the tailoring bootstrap, which knows the
  CV and vacancy to bind). Omitting it still creates a chat.
- The session list widens to span browsing conversations as well as chats, so a
  conversation begun in the side panel can be picked up at the desk. Tailoring
  sessions stay out, as they are today.

## Capabilities

### New Capabilities

- `assistant-page-awareness`: the assistant reading the page the caller's browser
  is showing — the `browse` preset, the `read_current_page` tool, and what
  happens when no browser is attached.

### Modified Capabilities

- `assistant-sessions`: the session API's authentication widens from "session
  cookie" to "session cookie or Bearer credential", so a browser extension can
  hold a conversation. The rollout gate and owner-scoping are unchanged.

## Impact

**Code.** `internal/handler/assistant.go` (route gates, the hub dependency),
`internal/handler/assistant_tools.go` (preset registry), a new
`internal/handler/assistant_page_tools.go`, `internal/assistant/store.go` (the
preset constant) and `internal/assistant/prompt.go` (the prompt).

**One migration.** `assistant_sessions.preset` is free text but carries a CHECK
pinning it to `('chat','tailor')` (migration 0044), so `browse` cannot be written
until that constraint is widened — `migrations/0047_assistant_sessions_browse_preset.sql`
drops and re-adds it. The rewritten constraint is strictly wider than the one it
replaces, so no existing row can violate it and the change is safe on a live
table. `internal/db/queries/assistant.sql` changes with it (the session list
widens) and `internal/db` is regenerated with `make sqlc`.

**Reuses, does not extend, the relay.** `browsertools.Hub` and its `Caller` are
used exactly as `RunAgentAutofill` uses them. One known limitation is inherited
rather than introduced: a channel has one harness end and the last connection
wins, so an autofill run and a turn calling `read_current_page` would evict each
other. A person clicks "Autofill" or sends a message, not both at once, so this
is left alone; the seam for fixing it is `Hub`.

**Depends on the extension implementing `read_page`.** The tool calls a primitive
the extension does not serve yet. Until the panel ships it, the tool answers with
its "no browser attached" error, which is the same thing it says to a web caller.

**Documentation drift found, not fixed here.** `openspec/specs/extension-auth`
states the connect flow mints an API key; `extension_connect.go` mints a session
JWT (it was unified on the JWT so one token authenticated hire and Roy). The
extension therefore holds a JWT, which is what makes `mw.key` sufficient here.
Correcting that spec is out of this change's scope, but Roy's removal will force
the question.
