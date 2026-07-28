## Context

The assistant runs in-process (`internal/assistant`): a bounded tool-calling loop
whose tools are built in `internal/handler` from the same services the HTTP
handlers use. A preset selects the system prompt and the tool set, and nothing
else — which is why one chat component serves `/my/assistant` and the CV-tailoring
workspace.

Separately, `internal/browsertools` already relays tool calls between an
in-process harness and a user's browser extension. `RunAgentAutofill` is that
harness today: it takes a `Caller` on the user's channel, drives `read_form` and
`fill_simple`, and closes it. The extension end of that wire is served by the
freehire side panel whenever it is open.

Those two systems have never met. This change introduces them.

The wider motivation — retiring the separate agent service the panel's chat runs
on — is written up in the sibling repository at
`freehire-extension/docs/superpowers/specs/2026-07-28-panel-on-freehire-assistant-design.md`.

## Goals / Non-Goals

**Goals:**

- An assistant conversation can be held from the browser extension.
- Within such a conversation, the agent can read the page the user is on, at a
  moment of its own choosing.
- Nothing about the web assistant changes.

**Non-Goals:**

- Writing to the page from the assistant. `fill_simple` and the combobox
  primitives stay behind `/me/autofill/run`, which is a deterministic, reviewable
  flow. An agent that can fill a form mid-conversation is a different feature with
  a different consent story.
- Deciding *which* pages may be read. See the cross-repo obligation below — that
  gate belongs in the extension, which is the only side that can enforce it.
- Fixing the one-harness-per-channel limit (see Risks).
- Correcting the `extension-auth` spec's drift from the code.

## Decisions

**Widen the gate to `mw.key` rather than build a JWT-only Bearer gate.**
`auth.RequireAuthOrKey` already resolves a cookie, a session JWT, or a full-scope
API key to one user id, and the extension already authenticates to
`/me/autofill/run` through it. A new middleware admitting the JWT but not the key
would be a fourth auth path to maintain for no security gain: a full-scope key
already reaches every other per-user route, so the assistant was the outlier, not
the exception.

**A third preset, not a tool added to `chat`.** The prompt genuinely differs — a
page to look at, a 400px column to answer into — and preset is the mechanism the
package already has for exactly that. Registering `read_current_page` for `chat`
was the alternative: fewer moving parts, but every web session would carry a tool
that fails unless a panel happens to be open, and a reliably-failing tool teaches
the model to stop calling it.

**A new `read_page` primitive rather than reusing `read_form`.** `read_form`
returns form fields and uploads — an application form's shape, not a posting's
prose. What the agent needs here is the page snapshot the panel already builds
for its match card (`scraper.ts`). Overloading `read_form` would mean one tool
whose result shape depends on the caller.

**A `Caller` per tool invocation.** `RunAgentAutofill` takes one for the length of
a run and closes it; a tool call is shorter still. Holding a long-lived harness
for the length of a conversation would mean a session pinning the user's channel
even while the user is doing something else in it.

**No browser attached is a tool error, not a turn error.** The package's rule is
already that a tool failure is not a turn failure: `Registry.Call` never returns a
Go error, so the model reads `{"error": ...}` and corrects within the same turn.
Here the correction is a sentence to the user — "open the side panel" — which is
strictly better than a failed turn with no explanation.

## The extension owes a consent gate

`read_current_page` returns whatever tab the panel is attached to. That may be a
vacancy — or a bank, a private inbox, an internal wiki. The result is sent to the
model provider and **persisted verbatim into `assistant_messages`**, where it stays
for the life of the conversation. The model chooses when to call it, and the
browsing prompt actively tells it to call again after any navigation.

Nothing here can gate that, because this side cannot see what the page is until it
has already read it. The extension can: it knows the url before it scrapes, and it
owns the surface where a user could be shown what is about to be sent. So the
obligation is recorded as a cross-repo dependency rather than left implicit —
`freehire-extension` decides which pages `read_page` will serve, and this side reads
whatever it is given.

Worth weighing against the fact that this product already runs a PII-masking service
for CVs; page text arriving unfiltered is an inconsistency, not a settled position.

## Risks / Trade-offs

**One harness end per channel; last connection wins.** → An autofill run and a
turn calling `read_current_page` would evict each other. Accepted: a person clicks
"Autofill" or sends a message, not both at once. The seam for fixing it is `Hub` —
several harnesses, each addressed by the id it is waiting on. Noted, not built.

**The tool ships before the primitive that serves it.** → Until the extension
implements `read_page`, every call answers "no browser attached" (the relay
answers an unreachable end rather than hanging). That is a correct, non-breaking
degradation, and it is what a web caller sees permanently.

**A full-scope API key can now reach the assistant.** → Accepted, and arguably a
fix: such a key already reaches every other full-scope route, and assistant turns
are billed to us only behind the rollout gate, which is evaluated after auth and
reads group membership fresh per request.

**Hub is in-memory and per-instance.** → Both ends of a channel must be on the
same process. Already true for autofill; the API is single-node. A multi-node
deployment needs a shared backplane, and the seam is still `Hub`.

## Migration Plan

Deploy order matters across repositories: this change first, the panel's second.
The panel cannot call the assistant until the routes accept a Bearer credential,
and it has nothing to answer `read_page` with until it ships that primitive.

Migration 0047 widens the `preset` CHECK and must run before the code that writes
`browse` — on prod, manually (`SET ROLE hire`) ahead of the deploy, as 0044 itself
records. No backfill: it only permits a value nothing has written yet.

Rollback has two halves. Redeploying the previous binary is safe on its own —
sessions already recorded as `browse` fall back to the chat prompt by the existing
unknown-preset rule, so they keep answering rather than erroring, and the widened
CHECK accepts everything the old code writes. Restoring the narrow CHECK is the
part that is not free: it fails while any `browse` row exists, so it comes after
deleting or re-labelling them, and only if the constraint is genuinely wanted back.
