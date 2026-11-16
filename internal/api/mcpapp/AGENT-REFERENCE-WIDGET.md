# internal/api/mcpapp — the widget and tool annotations

Part of [AGENT-REFERENCE.md](AGENT-REFERENCE.md): sections moved out of it for size.

## The widget

`search_jobs` and `get_job` render in an MCP Apps widget (`widget.go`, `widget/jobs.html`):
one self-contained page an MCP Apps host — ChatGPT, Claude — draws the result in as cards,
instead of the model retelling it as prose. The tools' contracts are untouched; a client
that ignores `_meta.ui` sees exactly what it saw before.

- **It is hand-written HTML, not a `web/` build, on purpose** — see
  `openspec/changes/archive/2026-10-20-chatgpt-app-widgets/design.md`. If it ever grows
  independent sign-in, complex forms or direct network requests, that is the point to
  move it. Saving and profile-match reads use the host bridge and existing tools, not
  another HTTP client or credential store.
- **The URI is the page's hash.** Hosts cache by URI; editing the file changes it. Never
  hard-code the URI anywhere else.
- **Wire data lives only in `result`, `job` and `req`** inside the page.
  `TestTheWidgetReadsOnlyFieldsTheWireShapeHas` greps those names and checks every field
  against `wire.go`, so a renamed field fails the build instead of rendering as nothing.
  Reading wire data through any other name blinds it.
- **Safe text, no direct network requests.** `TestTheWidgetCannotInjectMarkupOrMakeRequests` bans
  `innerHTML` and friends and every network API; the CSP admits only
  `https://logo.freehire.me` images. A URL is used only when it is `https:`.
- **Saving is an explicit host-mediated call.** `Save job` calls `save_job` with
  only the selected slug, never notes. Controls require the host's `serverTools`
  capability and never handle credentials. Stage editing stays in the tracking board
  and assistant tools rather than the card.
- **The match strip uses exact profile skill overlap**, like the website's
  `JobMatchBar`: percentage and matched/total job skills, case-insensitive. One
  successful host-mediated `get_profile` read serves the whole widget when a posting has
  skills; a failed read may retry on the next tool result. No profile, no skills or an
  unavailable host read means no strip, never a fabricated zero score. The profile
  response is not rendered; only the coverage is shown. Strips update in place so a late
  profile reply preserves focused controls and pending-save status announcements.
- **Success needs a server confirmation.** The returned tracking record must have
  a valid job id and saved timestamp. An unknown
  result or timeout locks writes until the person explicitly confirms their tracking
  board shows no change and chooses `I checked, allow retry`. Unlocking never sends
  a request: Save still needs its own click. Timed-out request ids are
  removed, so late replies cannot confirm a new operation. A confirmed refusal
  permits an explicit retry. Pending calls stay locked across
  result re-renders. `web/scripts/mcp-widget.test.mjs` drives the real embedded
  page's DOM and host protocol, including failure and source-spoofing cases.
- **Only a tool whose result the page can draw may name it** — a host draws the widget for
  every tool that does.
- **The handshake's field names are the specification's** (`appInfo`, not `clientInfo`),
  and a test holds them. The reference `AppBridge` accepts the wrong one without complaint,
  so a manual check cannot catch it.
- **Checking it by eye**: bundle the official host, `AppBridge` from
  `@modelcontextprotocol/ext-apps/app-bridge`, feed it real `tools/call` results from
  production, and drive it in a real browser (`agent-browser`). Connect the bridge BEFORE
  setting the iframe's `srcdoc` — the page's `ui/initialize` is its first act, and a bridge
  connected after load never hears it. Headless Chrome's `--virtual-time-budget` drops the
  iframe's `postMessage` traffic and renders an empty frame; do not use it.

## Annotations are load-bearing

Every tool declares `readOnlyHint: true`, `destructiveHint: false`, `openWorldHint: true`.
Incorrect annotation of these three is a named rejection reason in OpenAI's submission
guidelines. The block is written once (`ReadOnly()`) and a test walks the REGISTERED tools
rather than a list beside them — checked by removing one annotation and watching it fail.

A catalogue tool stays read-only even though every caller is signed in now: a write belongs
in the account tool set (`mixed.go`'s personal tools), whose annotations come from each
tool's `assistant.Effect`. Do not add a write tool to the catalogue.

The account server's annotations come from each tool's own `assistant.Effect` —
`EffectReads`, `EffectAdds` (nothing already held is lost: `save_job`, `apply_job`) or
`EffectOverwrites` (`unsave_job`, and `track_job`, which REPLACES the note). The zero value
is `EffectOverwrites` on purpose: a tool that declares nothing is announced as one that may
lose data. `TestMCPAccountTools_DeclareTheirEffect` walks the set the mount actually serves,
so a tool joining it must be classified there.

The account server also serves auto-apply and the screening answers
(`internal/api/handler/assistant_auto_apply_tools.go`, openspec/changes/auto-apply-over-mcp):
start, list, get, review, retry and cancel an attempt, plus the answer bank and the six typed
screening facts. The four that ACT on an attempt refuse unless the account turned on
`users.agents_may_apply`, which only a browser session can write — reading never needs it.
An answer saved here is the CANDIDATE's (`answerbank.AuthorCandidate`, so it is sent): this
server only ever acts for the person who approved the connection. `accountInstructions`
describes the account boundary and the tool groups actually served, opens with one line on what
freehire can do (search, save/track, alerts, auto-apply prepared for review) and says to call
`get_profile` before the first search and, with auto-apply served, `get_plan` before auto-apply. Its budget is 720 characters;
Claude Code cuts server instructions at 2048. Search and application
workflows belong to the packaged skill; initialization describes capabilities and result
states without prescribing a call sequence or presentation.

`get_plan` (`internal/api/handler/mcp_plan_tool.go`, openspec/changes/mcp-plan-standing) lets
an agent read the tier and today's per-feature allowances before it queues metered work,
instead of learning them from a 402. It reads `loadPlan`, the use case `GET /me/plan` serves,
so the two cannot disagree; it is an allowlist carrying no price or payment source (those are
the support agent's `my_account`, not a third-party model's). Grants and full-scope keys alike.

`present_jobs` is NOT served there (`mcpAccountTools`). It draws cards in our own chat UI and
its description tells the model never to write a job link; in a client with no card renderer
a model obeying it showed the person no vacancies at all.

Every server — both here and the OJCP one — introduces itself through `Identity`, which adds
the site's logo and website link when the origin is known.
