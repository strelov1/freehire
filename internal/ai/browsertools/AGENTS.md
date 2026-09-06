# internal/ai/browsertools — Browser-Tool Relay

The wire between an agent harness and a user's browser extension. The harness
issues tool calls (`read_form`, `fill_simple`, …); the extension executes them
against whatever page the user is on and sends results back.

## Architecture

- `Hub` holds one channel per user, each with up to two ends (`RoleHarness`,
  `RoleExtension`). `Join` attaches an end and returns its `leave`; `Forward`
  hands a frame to the *other* end of the sender's own channel.
- **Owner-scoped, always.** A channel is keyed by the user id the authenticating
  middleware resolved — never by anything the client claims — so a harness can
  only ever reach its own user's browser.
- **Opaque frames.** The relay parses nothing but the `id` (and only to build an
  error answer): `{id,tool,args}` / `{id,result}` pass through verbatim. Adding a
  primitive is a change in the extension and the harness, not here.
- **Never hang a caller.** A call with no extension attached is answered with
  `{id, error}` rather than dropped — the harness is blocked on that id. A result
  with no harness left is dropped (nobody is waiting). That one answer is the
  exported `ErrNotConnected`, and `Caller.Call` re-raises it as the sentinel rather
  than as a fresh error built from the text: a Go error cannot cross a JSON frame, so
  the wire text IS the encoding, and an in-process caller has to tell "nobody is there
  to run this" (a state its user fixes) from a tool that ran and failed (which may be
  a fault worth reporting). Everything else in an `{id, error}` frame is the
  extension's own words, which this package cannot classify — but it does say WHO
  spoke, by wrapping them in the exported `ErrExecutorReported`. That is the
  distinction a caller actually needs: "no active tab" and a content script Chrome has
  already discarded are the MOST COMMON way a browser call fails, and they are the
  user's to fix however they are worded. Without the sentinel the only discriminator is
  the string, so an HTTP caller mapping errors by kind answers the ordinary case with
  "internal server error" and files a report about a tab somebody closed.
- **Last connection wins.** Re-joining in a role replaces the previous socket; the
  displaced connection's `leave` is a no-op, so it cannot evict its successor.
- **A connected end is not yet a joined end.** `Join` runs in the websocket
  handler's own goroutine, which starts *after* the 101 handshake the client sees.
  So a call issued the instant a socket opens can find the channel half-open and
  come back `{id, error: "the browser extension is not connected"}` — correct
  behaviour, not a fault, and a caller that treats it as fatal is wrong to. This
  cost the integration test two rounds of "widen the timeout" before anyone
  reproduced it (sleep 50ms before `Join` and it is deterministic); the test now
  waits for the relay to actually route before asserting on it.

## Who the harness is

Two in-process harnesses take a `Caller` on a user's channel, both briefly:

- `RunAgentAutofill` (`/me/autofill/run`), for the length of one autofill run.
- the assistant's `read_current_page` tool, for the length of one tool call, in a
  `browse` session.

A channel has one harness end and the last connection wins, so **any** harness
evicts any other — including the long-running one holding an API key (the shape
`freehire-cli` uses). An evicted harness's in-flight call is answered into its
successor's `Caller`, which drops it as a late answer to an abandoned call, so the
evicted caller waits out its own deadline.

In practice a person clicks "Autofill" or sends a message, not both at once, so this
is left as it is. Fixing it means several harnesses per channel, each addressed by
the id it is waiting on — the seam is `Hub`, and it is the same seam a multi-node
deployment needs.

This is why `read_current_page` carries a deadline: the relay answers by itself only
when NO extension is attached, so every other way a call can go unanswered — an
evicted harness, a panel closed mid-call, a wedged tab — is bounded by the caller or
not at all.

## Transport

`internal/api/handler/browsertools.go` upgrades `GET /api/v1/tools/ws?role=…` behind
`auth.RequireAuthWS`. Only the subprotocol marker is echoed back, never the token.

Carrier and credential vary independently:

- **Carrier.** A browser can set no headers on `new WebSocket` and has no
  cross-origin cookie, so the extension puts its token in the subprotocol
  (`freehire-jwt, <token>`). Everything else sends `Authorization: Bearer`.
- **Credential.** Either a session JWT or an API key. The extension holds the JWT
  the connect flow minted; a long-running harness holds an API key — the same
  credential `freehire-cli` uses — which it can store and revoke, rather than a
  short-lived token it has no way to re-mint.

Both resolve to the same thing: the id of the freehire user who owns the channel.

The hub is in-memory and per-instance: both ends of a channel must be connected
to the same process. Fine while the API is single-node; a multi-node deployment
needs a shared backplane (the seam is `Hub`).
