## Why

The browser-tool wire was built for two brains, and only one of them exists. Its
own contract says so — `lib/tools/wire.ts` describes "a 'brain' (hosted Roy today,
a local harness later)", and hire's relay already accepts a harness authenticating
with the `fhk_…` API key that `freehire-cli` stores in `~/.freehire/creds.json`.

The half that never got built is the harness end. Neither `freehire-cli` nor
`freehire-mcp` can open the wire: both are plain REST clients (search, job, apply,
save, track). So today the only thing that can fill a form is freehire's own hosted
agent — a user who runs their own agent, and would rather it drove their own
browser, has no way to connect it.

Nothing in the backend blocks this. The relay is deliberately opaque: it "parses
nothing but the `id`… `{id,tool,args}` / `{id,result}` pass through verbatim.
Adding a primitive is a change in the extension and the harness, not here." The
extension side already exposes `read_form`, `fill_simple` and the four
`combobox.*` primitives. Only the client is missing.

## What Changes

- **A harness mode in `freehire-cli`:** a long-running command that opens the wire
  as `role=harness` with the stored API key and serves the browser primitives to a
  local agent.
- **The same in `freehire-mcp`,** exposing them as MCP tools, so an MCP host
  (Claude Desktop, Claude Code) *is* the user's harness with no glue of their own.
- **One written contract, two implementations.** The repos are Go and TypeScript,
  so nothing is shared but the frame shapes and the tool list — which are
  therefore written down once, in this change, rather than inferred twice from the
  extension's source.
- **The user's agent owns the loop.** It reads the form, decides, and drives the
  widgets itself. freehire supplies the primitives and the transport, not the
  policy.
- **NOT in this change:** replacing the hosted path (the panel's Autofill button
  keeps working for users who run no harness); a new credential (the existing API
  key is what the relay already accepts); anything that submits a form.

## Capabilities

### New Capabilities
- `harness-wire-client`: what a local harness must do to hold the wire open —
  connect and authenticate with the API key, answer or refuse a call, survive a
  drop, and shut down without stranding the browser end.
- `harness-browser-tools`: the browser primitives as a local agent sees them —
  the tool list, their arguments and their outcomes, identical across the CLI and
  the MCP server.

### Modified Capabilities
<!-- None. The wire's framing, the relay's ownership rules and the extension's
     primitives are all unchanged; this change only builds the end that was
     always specified as "later". -->

## Impact

- **freehire-cli**: a WebSocket client and a long-running command; the first thing
  in that binary that is not a one-shot REST call, so signal handling and
  reconnection appear where there were none.
- **freehire-mcp**: the same client, plus the primitives as MCP tools. Its
  existing tools are stateless request/response; this one holds a socket for the
  session's life.
- **hire**: none. The relay forwards frames verbatim and already resolves an API
  key to its owner.
- **freehire-extension**: none. It answers whichever end is attached.
- **Dependencies**: a WebSocket client in each repo — Go has none in
  `freehire-cli` today; `freehire-mcp` has none either.
- **Guarantee that does not carry over**: "no fabricated values" is enforced by
  `groundedIn` in hire's agent, not by the wire. A local harness writes what it
  likes into the user's own form. That is the user's own agent in the user's own
  browser, so it is theirs to decide — but it is a real difference from the hosted
  path, and the design has to say where, if anywhere, a guard belongs.
