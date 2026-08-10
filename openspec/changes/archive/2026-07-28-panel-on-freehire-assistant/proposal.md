## Why

The side panel's chat is the last thing in this extension that still talks to Roy.
Everything else already reaches hire directly: the browser-tool relay
(`/api/v1/tools/ws`), the agentic autofill (`/me/autofill/run`), the job and match
reads.

hire has since grown its own in-process assistant — the one the web app runs at
`/my/assistant` — and, as of the `assistant-browse-preset` change, it accepts a
Bearer session JWT and offers a `browse` preset whose agent can read the page
through the relay this extension already serves.

Two agent runtimes means every new tool is written twice and the panel's chat
drifts from the web's. The panel moves onto the same assistant, and
`agent.freehire.me` leaves this repository.

## What Changes

- **BREAKING** `extension/lib/roy/` is deleted — client, session bootstrap, wire,
  chat reducer and their tests. `ROY_ORIGIN` goes with it.
- A new `lib/assistant/` is ported from `web/src/lib/assistant/` in the hire repo:
  the SSE reader, the wire types, the chat reducer, the deck reducer, and the API
  and turn clients. One divergence: absolute `HIRE_ORIGIN` and
  `Authorization: Bearer` instead of a same-origin cookie.
- A turn becomes one POST whose response body streams SSE. No socket between
  turns, no `attach`, no input lease; cancelling is aborting the fetch.
- The panel creates its conversation with `?preset=browse` and remembers its id, so
  closing and reopening the panel resumes rather than restarts.
- The relay gains a third primitive, `read_page`, answering with the `PageSnapshot`
  the scraper already builds — this is what the assistant's `read_current_page`
  calls.
- **The "Read page" button is removed.** It was the manual stand-in for what the
  agent now does on its own, at a moment of its own choosing.
- Tool activity and `present_jobs` cards render in the panel, ported from the web.

## Capabilities

### New Capabilities

- `panel-assistant-chat`: the panel's conversation with freehire's assistant — its
  transport, how a turn runs and is cancelled, how a session survives the panel
  closing, and what the user sees while the agent works.
- `browser-tool-page-read`: the `read_page` primitive this extension answers, which
  is how the assistant sees the page.

### Modified Capabilities

None. `panel-contribute-page` is untouched: handing an unknown page to freehire is
a deterministic feature that never went through Roy.

## Impact

**Deleted:** `extension/lib/roy/` (6 files), `ROY_ORIGIN` in `env.d.ts` and
`.env.production`, every mention of Roy in `AGENTS.md` and `README.md`.

**Rewritten:** `extension/entrypoints/sidepanel/App.svelte` loses its transport
state — the client, the attach flag, the input lease, the echo suppression, the
frame subscription — and gains a session id it persists.

**Added:** `extension/lib/assistant/` and the tool-activity and job-card
components.

**Untouched:** `lib/tools/` beyond the new primitive, `lib/form.ts`,
`lib/combobox.ts`, `lib/auth.ts`, `lib/freehire.ts`, the background relay, the
content script, and all of autofill. None of it went through Roy.

**Depends on** hire's `assistant-browse-preset` (merged, freehire#1217). Until it is
deployed the panel gets 401s on the assistant routes.

**Stale artefacts removed with it:** the unarchived
`openspec/changes/connect-panel-to-roy/`, and
`docs/superpowers/specs/2026-07-24-panel-roy-chat-design.md`, which describes the
world this replaces.
