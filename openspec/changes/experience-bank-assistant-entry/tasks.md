## 1. The kickoff module

- [x] 1.1 RED: `web/src/lib/assistant/presets.test.ts` — a `profile` entry resolves to a
  session preset plus a kickoff line; a `chat` entry (and an unknown/absent value) resolves
  to the chat preset and no kickoff, so every other way into the assistant opens silent.
- [x] 1.2 GREEN: `web/src/lib/assistant/presets.ts` — the kickoff text and a pure
  `entryFromQuery(params)` that maps a URL's `preset` value to `{preset, kickoff}`. Keeping
  it pure is what makes the routing behaviour testable without rendering a page.
- [x] 1.3 REFACTOR + `simplify` on the diff; `pnpm --dir web test` green.

## 2. Merge the assistant routes

- [x] 2.1 Create `web/src/routes/my/assistant/[[id]]/+page.svelte` from the two pages it
  replaces: no id → open newest/create and rewrite the address with `replaceState`; id →
  open it, and selecting another chat pushes a history entry. Delete
  `web/src/routes/my/assistant/+page.svelte` and `web/src/routes/my/assistant/[id]/+page.svelte`.
- [x] 2.2 Feed the page from `entryFromQuery` — pass `preset` and `kickoff` into
  `AssistantChat`, so `?preset=profile` creates a session AND starts its first turn.
- [x] 2.3 Update the typed `resolve()` call sites for the renamed route
  (`AssistantChat.svelte:453` and the merged page); `pnpm --dir web run build` must pass,
  which is what catches a missed one.
- [x] 2.4 `pnpm --dir web test` green — `accountNav.test.ts` still finds `/my/assistant`.

## 3. The bank's call to action

- [x] 3.1 `ExperienceBankView.svelte`: drop the `Sparkles` import and element, relabel the
  button "Add an achievement", and add the example line beneath it.
- [x] 3.2 Render the same button + example in the empty state, beneath the existing
  explanation, replacing the action-less `States` message.
- [x] 3.3 REFACTOR + `simplify`; the header row and the empty state must share one snippet
  rather than repeating the markup.

## 4. Verify the whole change

- [x] 4.1 `pnpm --dir web test`, `pnpm --dir web run lint`, `pnpm --dir web run build`.
- [x] 4.2 Visual check of both bank states (populated and empty) against the running app.
- [x] 4.3 Manual flow: bank action → a new `profile` session opens, the kickoff appears as
  the candidate's message, the agent answers with one question about a thin spot, and the
  address has been rewritten to that session's URL without the turn being cut off.
- [x] 4.4 Manual regression: Back/Forward between two chats, a reload of a session with
  history sends no kickoff, and the account-nav entry still opens silent.

## Verification notes

- 4.3 was run against a local stack with no LLM credentials, so the agent's reply itself
  could not be observed: the turn reached the backend and was refused there with 503 "the
  assistant is not available". That is the part this change is responsible for — the
  request survives the rewrite of the address instead of being aborted by a remount — and
  the API log confirms `POST /sessions` (201, preset `profile`) followed by
  `POST /sessions/<id>/messages` after the URL had already become the session's own.
  The agent's first question needs a stack with `LLM_BASE_URL` set.
