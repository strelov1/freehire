## Context

The panel's chat runs on Roy over a control protocol: `POST /sessions`, connect a
WebSocket, then `attach` → `acquire_input` → `send`, folding streamed `TurnEvent`s
into a message list. `lib/roy/` is 553 lines carrying that.

hire's own assistant needs none of it. A turn is one POST whose response body
streams SSE; there is no connection between turns, no attach, no input lease.
Cancelling is aborting the fetch — the backend notices its next write fail and
stops before spending another model call.

Everything else in this extension already talks to hire directly, including the
browser-tool relay that the assistant's new `read_current_page` reaches through.

The backend half shipped as freehire#1217 (`assistant-browse-preset`). The full
cross-repo write-up is `docs/superpowers/specs/2026-07-28-panel-on-freehire-assistant-design.md`.

## Goals / Non-Goals

**Goals:**

- One agent runtime. `agent.freehire.me` leaves this repository.
- The panel's chat and the web's chat stay one implementation, ported once.
- The agent can see the page, at a moment of its own choosing.

**Non-Goals:**

- A session rail. The panel is a narrow column; one active conversation and a "new
  chat" action is the whole affordance. The web keeps the rail, and a conversation
  begun here appears in it.
- Writing to the page from the chat. Autofill stays the deterministic, reviewable
  flow it is.
- Consent UI for which pages may be read (see Risks).

## Decisions

**Port the web's client rather than write a second one.** `sse.ts`, `wire.ts`,
`chat.ts`, `deck.ts` and `tool-formatters.ts` come across as-is. The repo's
existing convention — "`lib/roy/` mirrors the web assistant, keep them aligned" —
survives the rename and gets easier to honour, because what is being mirrored
shrinks from three protocol layers to one fetch.

**Two divergences, both forced, both in one place.**

1. *Auth and origin.* The web uses `credentials: 'include'` against a relative
   path; the panel uses an absolute `HIRE_ORIGIN` and `Authorization: Bearer`. This
   lives in `api.ts` and `client.ts` and nowhere else.
2. *Job fetching.* The web's `jobCache.ts` imports `$lib/api` and `$lib/types` —
   SvelteKit aliases that do not exist here. The cache's logic (dedupe by slug,
   evict a rejection so a later render can retry) is ported over
   `lib/freehire.ts`'s `getJob`, which needs the token the web's cookie made
   implicit.

**Persist the session id, not the transcript.** `chrome.storage.local` holds the
id; reopening replays the server's transcript through `eventsFromTranscript` into
the same reducer a live turn folds through. Caching messages locally would mean
two sources of truth for a conversation the web can also continue.

**Drop the optimistic user bubble.** It existed because Roy echoed `user_prompt`
back and the panel had to paint before the round trip. hire emits `user_prompt` as
the turn's first frame, before the first model call, so the reducer paints it just
as fast — and the echo suppression (`pendingEcho`) goes away with it.

**`read_page` is a third primitive, not an overload of `read_form`.** `read_form`
returns an application form's fields and uploads; what the agent needs is the
posting's prose. `scraper.ts` already produces exactly that for the match card, and
the `GET_PAGE_SNAPSHOT` runtime message already carries it — so this is wiring, not
new logic.

## Risks / Trade-offs

**The panel is one release behind its backend.** → The assistant routes only accept
a Bearer credential once freehire#1217 is deployed. Until then the panel gets 401s.
Nothing to do here but sequence the deploys.

**Reading arbitrary pages.** → `read_page` will serve whatever tab is active,
including a bank or a private inbox, and hire persists the result verbatim into the
transcript. This extension is the only side that can gate it: it knows the url
before it scrapes. Not built in this change, and recorded as owed — hire's design
doc names it as a cross-repo dependency, and this is the repo that owes it.

**Deleting the transport wholesale.** → `lib/roy/` has tests; deleting them removes
coverage of code that no longer exists. The ported files arrive with the web's own
tests, so the count moves rather than drops.

## Migration Plan

No user-facing migration: a conversation held with Roy has no counterpart in hire's
store, so the panel starts fresh. That is acceptable — the feature is behind hire's
restricted rollout and has no users.

Order: freehire#1217 deploys, then this ships. Rolling back means shipping the
previous extension build, which still speaks Roy — so Roy's deployment stays up
until this is confirmed, and is retired separately.
