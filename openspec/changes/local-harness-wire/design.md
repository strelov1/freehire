## Context

The wire has three parts and two are built. The extension answers `read_form`,
`fill_simple` and `combobox.open/options/select/verify` against whatever page the
user is on. hire's relay (`internal/browsertools`) keys a channel by the
authenticated user id, holds up to two ends per channel (`RoleHarness`,
`RoleExtension`), and forwards frames verbatim — it "parses nothing but the `id`",
and accepts either a session JWT or an API key, "the same credential
`freehire-cli` uses".

The third part, the harness end, does not exist. `freehire-cli` is a one-shot REST
client over `~/.freehire/creds.json`; `freehire-mcp` mirrors it as MCP tools. Neither
opens a socket.

## Goals / Non-Goals

**Goals:**
- A user's own agent can read and fill the form in the user's own browser, using
  the API key they already have.
- The same tool list and outcomes whichever client they use, so a loop written
  against one works against the other.
- Failures the user can act on: "your browser is not connected" must not surface
  as a raw frame or a timeout.

**Non-Goals:**
- Replacing the hosted path. The panel's Autofill button keeps working for users
  who run no harness.
- A new credential, scope, or endpoint. The relay already resolves an API key to
  its owner.
- Shared *code* between the two clients — one is Go, the other TypeScript. Only
  the contract is shared.
- A multi-node relay backplane (the hub is per-process; that seam is hire's).
- Submitting a form, uploading a file, or anything irreversible.

## Decisions

**Each client keeps its own shape; only the contract is common.** `freehire-cli`
exposes one-shot subcommands — `freehire browser read-form`, `… combobox open`,
and so on — because that repo's whole premise is "a single static binary any agent
can run", and shelling out per call is how an agent uses a CLI. `freehire-mcp`
holds one socket for the session and exposes the primitives as MCP tools, because
an MCP server is already long-running. Same tools, same arguments, same outcomes;
different lifetimes, each idiomatic where it lives.

**One-shot calls work because the state is the page's, not the connection's.**
This is what makes the CLI shape viable at all: `combobox.open` leaves the widget
open *in the DOM*, so a later call over a fresh connection reads that same open
widget. The wire carries no session, and the extension keeps none — every
primitive is a question about the live page. A harness that reconnects between
steps therefore loses nothing.

**The contract is written here, not inferred twice.** The tool names, their
arguments, and every status they can return are specified in this change's
`harness-browser-tools` spec. Two implementations reading one spec is the only way
they stay in step; two implementations reading the extension's TypeScript is how
they drift.

**No new guard on the local path, and the reason is not laziness.** The
extension already enforces what is *mechanically* honest: it selects only an
option the widget offers, and it reports a commit only when the widget confirms
it. What it does not enforce is whether a value is justified by the user's profile
— that is `groundedIn`, and it lives in hire's agent because it is *policy*.
Policy belongs to whoever owns the loop. On the hosted path that is freehire, and
the filter stays. On a local path the owner is the user, running their own agent
against their own browser; interposing our judgement there would be both
unenforceable and presumptuous. The spec says this out loud so it reads as a
decision rather than an omission.

**A per-call timeout above the extension's own waiting.** `combobox.open` and
`combobox.select` wait for the widget to re-render before answering — up to a
second each. A harness that times out faster than the extension answers would
report failures that did not happen, so the client's timeout is 15s: far above the
widget budget, far below a human's patience.

**Errors are rendered, not forwarded.** The relay answers `{id, error}` rather
than dropping a call when no extension is attached — precisely so the caller is
not left hanging. The clients turn that into one sentence naming the fix ("open
the freehire side panel in Chrome"), because "no extension attached" is a state
the user resolves, not a fault.

## Risks / Trade-offs

- [A handshake per call] → The CLI's one-shot shape costs a WebSocket connect per
  primitive, and driving one widget is four. On a 27-widget form that is ~100
  connects. Accepted for now because the alternative is a long-running daemon in a
  binary that has never had one; if it proves slow, a persistent `serve` mode is
  additive and the subcommands can speak to it.
- [Last connection wins] → The relay replaces the previous socket when an end
  re-joins in the same role. So a CLI one-shot call *evicts an attached MCP
  harness*, and two harnesses fight silently. The MCP server must therefore
  reconnect rather than assume it stays attached, and this interaction has to be
  documented for the user who runs both.
- [No grounding on the local path] → A user's harness can write anything into the
  form. Deliberate, per the decision above, but it means the hosted and local
  paths do not offer the same guarantee — and a user who assumes otherwise could
  submit an answer their profile never supported. The mitigation is that nothing
  here submits: the human still reviews the form.
- [The contract drifts anyway] → Two implementations and a prose spec is weaker
  than one implementation. Nothing stops the extension adding a status neither
  client handles. The narrow defence is that an unrecognised status must be
  surfaced verbatim rather than mapped to a guess, so drift shows up as an
  unfamiliar word instead of a wrong outcome.
- [`ambiguous` needs a human decision] → The extension refuses a label that
  addresses several widgets (a repeated education row). A harness cannot resolve
  that by retrying; it has to tell its agent the question is unaddressable by
  label. Both clients must pass that through rather than treat it as a transient
  failure.

## Open Questions

- Does the MCP server expose the primitives unconditionally, or behind an opt-in?
  An MCP host that gains browser-writing tools by default is a bigger grant than
  its current read-and-apply tools. Resolve before the MCP tasks.
