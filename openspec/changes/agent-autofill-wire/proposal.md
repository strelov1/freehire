## Why

The extension's autofill is deterministic (`form.ts` `matchFieldKey` +
`/me/autofill-profile`) — it only fills a fixed set of known fields and breaks on
anything unusual. We want an **agent-driven** autofill: the agent sees the form
and decides the values. Generalized, this is a **harness-agnostic browser-tool
wire** — the extension is a browser-tool *executor*, the "brain" is pluggable
(hosted Roy today, a local harness later), and the transport runs through hire
(which already owns auth). Autofill is the first capability on that wire.

A throwaway spike (verdict: VALIDATED) proved the wire and the actuation for
standard controls, and established that custom widgets (react-select) need a
separate CDP/trusted-input layer driven by an agent loop — deferred to a later
change. This change lands the **foundation**: the transport, the read/fill-simple
primitives, and an agent filling standard fields end-to-end.

## What Changes

- **hire (transport):** a JWT-authenticated WebSocket relay so a harness and the
  extension exchange browser-tool calls (request/response, correlated by id),
  mirroring the existing Roy `/ws` relay pattern. Raw JSON tool protocol (an MCP
  wrapper is a later concern).
- **extension (executor):** browser-tool primitives over that wire —
  `read_form` (fields with their labels, iframe-aware) and `fill_simple`
  (text / textarea / checkbox / native-select, addressed by **label**, matched
  and filled atomically so React re-renders can't drift the target).
- **agent (driver):** given the form and the user's profile (via a hire API), the
  agent produces a `{label → value}` plan and fills the standard fields through
  the wire.
- **NOT in this change (deferred → next):** CDP / `debugger` permission,
  `combobox.*` primitives, and the react-select agent loop; MCP wrapping; the
  local-harness transport binding.

## Capabilities

### New Capabilities
- `browser-tool-wire`: a JWT-authenticated, harness-agnostic tool channel between
  a harness and the extension, relayed by hire — carrying `read_form` and
  `fill_simple` calls and their results.
- `agent-autofill`: the agent reads the current form, maps the user's profile to
  its fields, and fills the standard controls through the wire.

### Modified Capabilities
<!-- None: this extension repo has one prior spec (panel-agent-chat); this change
     adds new capabilities rather than changing its requirements. The hire relay
     and Roy driver are tracked here as implementation task groups, not as specs
     of this repo. -->

## Impact

- **freehire-extension**: new browser-tool executor module (reuses `form.ts`
  `extractForm`/`applyFills`); the side panel / background owns the wire client
  (JWT auth, reconnect); `read_form`/`fill_simple` primitives.
- **hire** (sibling repo): a WS relay endpoint that authenticates the JWT and
  forwards tool frames between an authenticated harness and that user's extension;
  a profile endpoint the agent uses for values (reuse `/me/autofill-profile`).
- **freehire-agent / harness**: the driver that calls the primitives (the agent
  loop; slice-1 fills standard fields only).
- **Dependencies**: no new services; builds on the unified JWT auth already
  shipped. One new Go module — `github.com/gofiber/contrib/websocket`, since hire
  had no WebSocket transport (see design.md).

Spike verdict + Jobright reverse-engineering recorded in memory
`hire-extension-autofill-wire`.
