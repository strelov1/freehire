# AGENTS.md

Guidance for AI agents working in this directory.

## What this is

A browser extension that puts a job-application **agent in a side panel** — a
Claude/Gemini-style side bar with access to whatever page the user is on. The
agent is **freehire's own**, running inside hire's API (`internal/assistant`):
the panel holds a conversation under the `browse` preset, whose agent can read
the open page through the browser-tool relay this extension serves.

**Current state: the chat runs on hire's assistant.** Sign in with freehire and
talk to it; there is no "read page" affordance, because the agent calls
`read_current_page` itself when a question needs it. A profile-aware match card,
page intake and agent-driven Autofill also ship.

Everything reaches one origin. Auth crosses it with the session JWT the connect
flow minted: `Authorization: Bearer` on the HTTP API, the `freehire-jwt`
subprotocol on the relay's WebSocket (a browser cannot set headers on
`new WebSocket`).

Stack: **WXT + Svelte** (Chrome MV3), styled on `freehire-design-system`
(Tailwind v4 tokens + primitives — see the root `AGENTS.md`'s design-system row).
No local server, and no separate agent service.

## Layout

```
extension/            WXT + Svelte MV3 extension (npm-managed; the repo's other
                       JS packages, web/ and design-system/, are pnpm)
  entrypoints/
    background.ts     service worker: opens the panel, relays panel <-> content
    content.ts        injected everywhere; reads the page into a PageSnapshot
    sidepanel/        Svelte chat app (owns the relay WebSocket)
      App.svelte      chat UI + match card + apply plan + page intake + Autofill
      MatchCard.svelte  profile-match card
      ApplyPlan.svelte  the open form's questions, answered or not, + the counter
      ToolGroupList.svelte  what the agent is doing, mid-turn
      JobDeck.svelte / JobDeckCard.svelte  the `present_jobs` cards
      app.css         Tailwind + freehire-design-system/theme.css import
      main.ts         Svelte 5 mount
  lib/
    assistant/        the chat: SSE reader, wire types, chat + deck reducers,
                      tool formatters, the session store, and the API and turn
                      clients (+ tests). Ported from the freehire web assistant.
    tools/            browser-tool executor: the wire contract, the executeTool
                      dispatch (+ tests), the ToolChannel socket to hire's relay,
                      and the PageBridge that reaches the tab's frames.
    auth.ts           "Sign in with freehire" (launchWebAuthFlow) + token storage
    freehire.ts       hire API reads (job, match, autofill profile)
    protocol.ts       in-extension RuntimeMessage contract (+ test)
    scraper.ts        DOM -> PageSnapshot, pure over its Document arg (+ test)
    form.ts           form observe/map/act/reveal for Autofill (+ test)
    applyPlan.ts      form fields -> the panel's checklist + required counter (+ test)
    walk.ts           the order an autofill works through the form, as a value (+ test)
    debounce.ts       one call after a burst goes quiet (+ test)
    combobox.ts       drives custom-widget comboboxes (open/options/select/verify);
                      pure over its Document like form.ts, but async (+ test)
  wxt.config.ts       manifest (permissions, side_panel, host_permissions) +
                      the Tailwind Vite plugin
```

## Architecture

- **Side panel** (Svelte) owns the relay WebSocket. It's the only context that
  stays alive while open — unlike the MV3 service worker, which Chrome kills when
  idle, so it must not hold the durable socket. The chat needs no socket at all;
  each turn is its own request.
- **Content script** reads the live DOM on request. It holds no state.
- **Background** is a thin relay: the panel can't message a content script
  directly, so background forwards a snapshot request to the active tab.

A turn is **one POST whose response body streams SSE** (`lib/assistant/`): create
a conversation once (`POST /assistant/sessions?preset=browse`), then post each
message and fold the streamed `TurnEvent`s into the message list. Nothing is held
open between turns, and cancelling is aborting the fetch — the backend notices its
next write fail and stops before spending another model call.

The conversation id lives in `storage.local`, so closing the panel and reopening
it resumes; the transcript is replayed from the server through the same reducer a
live turn folds through. A conversation deleted from the web starts a fresh one
silently.

Read-a-page flow — note that the panel is not the one deciding:

```
hire's agent --read_page--> relay --> panel
panel --GET_PAGE_SNAPSHOT--> background --(active tab)--> content
content --PAGE_SNAPSHOT--> background --> panel --> relay --> the agent, mid-turn
```

## Conventions

- **`protocol.ts` is the in-extension contract** — `RuntimeMessage`
  (panel <-> background <-> content, discriminated by `kind`). Neither the chat's
  wire nor the browser-tool wire is here: they are `lib/assistant/wire.ts` and
  `lib/tools/wire.ts`, each mirroring a contract hire owns.
- **Autofill is watched, not batched.** The panel walks the form one question at a
  time (`lib/walk.ts` holds the order as a value; App.svelte supplies the ~300ms
  pauses), each fill carrying `reveal` so the page scrolls to the question and
  outlines it as the value lands. The pause is for the eye — the walk IS the audit
  the user would otherwise have to do afterwards. The agent path fills server-side
  as before and the panel plays its report back over the page, revealing each label
  it reported without re-writing anything.
- **The plan is the panel's standing account of the form**, computed by
  `buildPlan` from what the page reported and rebuilt on every page change, after
  every walk step, and on `FORM_CHANGED` — the page's own debounced notice that
  someone typed into it. It counts REQUIRED questions only (they are what gates
  submission) and is null for a page showing no application form.
- **`revealField` borrows the page's style and gives it back.** The outline is
  inline style, restored after ~600ms: the extension runs on pages freehire does
  not own, where an injected class can collide with the ATS's own and a stylesheet
  outlives our interest in the element.
- **The agent's reading is bounded, and visible.** `read_page` refuses any tab that
  is not `http(s)` (`lib/tools/readable.ts`), decided from the url before the page
  is read — this extension is the only side that sees a url before scraping it. The
  panel then names the page each read touched, minus query and fragment, which is
  where session tokens live. That display lives in `lib/assistant/pageRead.ts`
  rather than in `tool-formatters.ts`, precisely because the latter is a verbatim
  port and `read_current_page` cannot occur in the web app.
- **`lib/assistant/` mirrors the freehire web assistant** (`web/src/lib/assistant`
  in this repo). `sse`, `wire`, `chat`, `deck` and `tool-formatters` are
  verbatim ports — keep them aligned. Three files diverge, and they say so
  at the top: `api.ts`/`client.ts` (absolute origin + Bearer, since extension code
  has no cookie) and `jobCache.ts` (reads the token the web's cookie made
  implicit).
- **Test the logic, not the transport.** `scraper`, `form`, `protocol`, the
  assistant's reducers and its API clients are tested (vitest); the chrome message
  plumbing, the live socket and `storage.local` helpers are thin glue.
- **TypeScript stays on 5.x** — svelte-check does not yet support the native
  `typescript@7` (tsgo).
- **Styling comes from `freehire-design-system`** via a `file:../design-system`
  dependency (npm's local-symlink protocol — pnpm's `link:` has no npm
  equivalent by that name). Components not covered by an existing primitive keep
  local scoped CSS, but reference the package's CSS custom properties
  (`var(--brand)`, `var(--muted-foreground)`, ...) rather than literal colors.
  **`design-system/` must have its own dependencies installed
  (`cd design-system && pnpm install`) before `extension/` will build** — same
  gotcha the root `AGENTS.md` documents for `web/`: a `file:`/`link:`-linked
  package's own dependencies are not pulled in by the consumer's install.

## Commands

```bash
# hire must be reachable at WXT_HIRE_ORIGIN (default: the local SPA at :5173,
# which proxies /api). Any signed-in account will do — the assistant is open
# to every signed-in user.

cd design-system && pnpm install     # once, and after pulling design-system changes
cd extension && npm install          # runs `wxt prepare` (generates .wxt/)
cd extension && npm run dev          # dev build with HMR
cd extension && npm run build        # production build -> .output/chrome-mv3
                                     # targets freehire.me
                                     # (extension/.env.production); dev keeps
                                     # the localhost default
cd extension && npm test             # vitest: lib/**/*.test.ts
cd extension && npm run check        # svelte-check
```

Load unpacked in Chrome: `chrome://extensions` → Developer mode → **Load
unpacked** → `extension/.output/chrome-mv3`. Click the toolbar icon to open the
panel.
