## Context

The deterministic autofill (`form.ts` `matchFieldKey` + `/me/autofill-profile`)
only fills a fixed field set. We want an agent-driven autofill, generalized into a
harness-agnostic **browser-tool wire**: the extension executes browser tools; the
brain (hosted Roy today, a local harness later) drives them; hire relays, using
the unified JWT it already issues. A throwaway spike (memory
`hire-extension-autofill-wire`) validated the wire and standard-field actuation,
and showed custom widgets (react-select) need a CDP/trusted-input layer + an agent
loop — deferred to a later change. This change is the foundation.

## Goals / Non-Goals

**Goals:**
- A JWT-authed, harness-agnostic tool channel relayed by hire.
- `read_form` + `fill_simple` primitives that reliably fill standard controls with
  **label** addressing (no positional-index drift).
- An agent that maps profile → form and fills standard fields end-to-end.

**Non-Goals (deferred to the next change):**
- CDP / `debugger` permission, `combobox.*` primitives, the react-select agent
  loop. Multi-select country groups and autocomplete widgets.
- MCP wrapping of the wire (harness-facing).
- The local-harness transport binding (native-messaging host / localhost relay).

## Decisions

**Transport = a hire-hosted WS relay, raw JSON tool frames.** The extension and
the harness each connect to hire (JWT-authed) and hire forwards
`{id,tool,args}` / `{id,result}` between the same user's two ends — mirroring the
Roy `/ws` relay (owner-scoped, per-connection). Alternative — the extension talks
to the harness directly — was rejected: hire already owns auth + connections and
the user wants the link to run through it; a direct channel would re-implement
auth and can't bridge a hosted harness.

**Primitives, not a smart filler.** The spike proved a clever one-shot filler is
brittle; the durable shape is small primitives the agent composes. Slice 1 ships
the two reliable ones (`read_form`, `fill_simple`); the widget primitives
(`combobox.open/options/select/verify`, CDP-backed) come next.

**Label addressing + atomic observe/act.** Fields are addressed by their label and
matched+written inside a single injected pass. The spike showed positional indexes
drift when React re-renders change the control count between read and fill; label
addressing eliminated it.

**Reuse `form.ts`.** `extractForm`/`applyFills` already do observe/act with native
event dispatch; the executor wraps them and adds label-match addressing. No new
DOM-actuation code where the existing primitive suffices.

**Values from the hire profile.** The agent fetches the user's profile via a hire
API (reuse `/me/autofill-profile`) and maps it to fields — never fabricating
values absent from the profile.

**MCP deferred.** The spike showed a raw JSON tool channel is sufficient. A local
Claude-style harness will want MCP; we wrap the wire as MCP in a later change
rather than couple slice 1 to it.

## Risks / Trade-offs

- [Standard fields only — comboboxes unfilled] → Explicitly scoped; the executor
  reports comboboxes as deferred instead of writing wrong values (the spike's
  "Norfolk Island instead of No" failure mode is designed out).
- [Relay is a new cross-user surface in hire] → Owner-scoped like the Roy relay:
  a harness only reaches its own user's extension, enforced on every frame.
- [Two WS ends per user (extension + harness)] → Correlate by connection identity
  (JWT `sub`); a missing counterpart returns an error result, never hangs.
- [Cross-repo change under one OpenSpec home] → hire relay + harness driver are
  task groups here; OpenSpec artifacts stay repo-local to the extension.

## Migration Plan

Additive. The deterministic autofill button stays until the agent path is proven,
then can be replaced. Internal-only until wired to a running hire relay + harness.

## Resolved during implementation

**Relay endpoint = a dedicated `GET /api/v1/tools/ws?role=…`** in a new
`internal/browsertools` package, not a reuse of Roy's relay code (which lives in
another repo and another language). Auth is `auth.RequireAuthWS`: `Bearer` for a
server-side harness, the `freehire-jwt` subprotocol for the extension. Only the
marker is echoed back, never the token.

**The slice-1 driver is an agent inside hire** (`internal/autofillagent`),
triggered by `POST /api/v1/me/autofill/run`. It attaches to the hub as
`RoleHarness` through the same `Join`/`Forward` path a remote harness uses — one
relay path, no in-process shortcut beside it — so the wire stays
harness-agnostic and the panel gets a single endpoint to call. Roy stays the
long-term brain; it needs the deferred MCP wrapping to drive these tools.

**"No fabricated values" is enforced, not prompted.** The planner's output is
filtered against the profile (`groundedIn`): a value survives only if it is
contained in, or contains, one of the profile's own values. The prompt asks for
the same thing, but the guarantee lives in code.

**New Go dependency:** `github.com/gofiber/contrib/websocket` — hire had no
WebSocket transport at all. This contradicts the proposal's "Dependencies: none
new", which meant no new *services*; the relay cannot be built without a
WebSocket library.

## Open Questions

- Multi-node: the hub is in-memory, so both ends of a channel must reach the same
  API instance. Fine single-node; a shared backplane is the seam (`Hub`).
