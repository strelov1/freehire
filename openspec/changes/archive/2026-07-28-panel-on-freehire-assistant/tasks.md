Ported files come from `web/src/lib/assistant/` in the hire repo, with their tests.
"Port" means copy and adjust imports only — a behavioural change to a ported file
is a divergence and needs saying so in a comment.

## 1. The `read_page` primitive

- [x] 1.1 Add a failing test in `lib/tools/executor.test.ts`: a `read_page` call resolves to the page's url, title, headline and text, over a fake `PageBridge`.
- [x] 1.2 Add a failing test that a bridge which cannot reach the tab answers with an error result carrying the call's id, rather than throwing.
- [x] 1.3 Add `readPage()` to `PageBridge` and the `read_page` case to `executeTool`; implement the bridge method in `lib/tools/page.ts` over the existing `GET_PAGE_SNAPSHOT` runtime message.

## 2. The transport

- [x] 2.1 Port `sse.ts` and its test verbatim into `lib/assistant/`.
- [x] 2.2 Port `wire.ts` and its test verbatim (the `TurnEvent` shapes and `eventsFromTranscript`).
- [x] 2.3 Port `chat.ts` and its test verbatim (the reducer both live turns and replayed transcripts fold through).
- [x] 2.4 Write `lib/assistant/api.ts`: create (with `?preset=browse`), get, delete a conversation — absolute `HIRE_ORIGIN`, `Authorization: Bearer`, and `SessionNotFound` for a 404. Test the divergent part: the request carries the Bearer credential and no cookie.
- [x] 2.5 Write `lib/assistant/client.ts`: `sendTurn` posting a message and reading the SSE stream, returning `{done, cancel}`. Test that cancelling surfaces as a `cancelled` result rather than an error.

## 3. The panel runs on it

- [x] 3.1 ~~Add a failing test for the session store~~ — dropped. `AGENTS.md` says to test the logic and not the transport, and `auth.ts` sets the precedent: its `parseAuthRedirect` is tested, its `browser.storage` helpers are not. A remember/recall/forget trio over `storage.local` is that same thin glue; the logic worth testing (a dead id starts a fresh conversation) lives in the restore path and is covered where it is decided.
- [x] 3.2 Write the session store over `browser.storage.local`, shaped like `auth.ts`'s token helpers.
- [x] 3.3 Rewrite `App.svelte`'s chat: `sendTurn` in place of connect/attach/acquire/send; the stop button cancels the turn; `resetRoy`, `ensureConnected`, `endTurn`, `pendingEcho`, `inputAcquired`, `attached` and the frame subscription all go.
- [x] 3.4 On mount, restore the remembered conversation through `getSession` → `eventsFromTranscript` → the reducer; a `SessionNotFound` starts a fresh one silently. Add "New chat".

## 4. What the user sees while it works

- [x] 4.1 Port `tool-formatters.ts` and its test; add a label for `read_current_page`.
- [x] 4.2 Port `ToolGroupList.svelte`, tightened for a 400px column.
- [x] 4.3 Port `deck.ts` and its test verbatim.
- [x] 4.4 Port `jobCache.ts` over `lib/freehire.ts`'s `getJob` — it needs the token the web's cookie made implicit. Test the dedupe and the eviction-on-failure.
- [x] 4.5 Port `JobDeck.svelte` / `JobDeckCard.svelte`, styled beside the existing `MatchCard`.
- [x] 4.6 Remove the "Read page" button and the snapshot-into-the-prompt path it drove.

## 5. Roy leaves

- [x] 5.1 Delete `lib/roy/` entirely.
- [x] 5.2 Remove `ROY_ORIGIN` from `env.d.ts` and `.env.production`.
- [x] 5.3 Rewrite the Roy passages in `AGENTS.md` and `README.md`: the panel talks to hire's assistant; `lib/assistant/` mirrors the web's, with the two divergences named.
- [x] 5.4 Delete `openspec/changes/connect-panel-to-roy/` and `docs/superpowers/specs/2026-07-24-panel-roy-chat-design.md`, both of which describe the world this replaces.

## 6. Verify

- [x] 6.1 `npm test`, `npm run check`, `npm run build`.
- [x] 6.2 Load the built extension against a local hire and hold one conversation: a turn streams, a tool call renders, `read_current_page` returns the open page, cancelling stops a turn, and reopening the panel resumes it.
