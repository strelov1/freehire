## Why

`read_page` serves whatever tab is active, and hire persists what comes back
verbatim into the conversation transcript — permanently, where the web app can
replay it. The agent decides when to call it, and its prompt encourages calling
again after any navigation.

Two things follow, and neither is currently true:

- The extension is the only side that can see a url *before* scraping it, so
  drawing a boundary is its job. hire cannot: by the time it has the page, it has
  the page.
- A read the user cannot see is a read they cannot object to. The transcript says
  "Reading the page you are on" without saying which page that was.

Recorded as owed in both repositories' design docs when the browsing preset
shipped. This is that debt.

## What Changes

- `read_page` refuses any tab that is not `http`/`https`, with a message that
  states the rule rather than the symptom.
- The tool's line in the transcript names the page that was read — origin and
  path, e.g. `boards.greenhouse.io/acme/jobs/12`.
- **Query strings and fragments are dropped from that display.** They routinely
  carry one-time tokens and session identifiers; putting them on screen would be
  the very leak this change is about.

## Capabilities

### Modified Capabilities

- `browser-tool-page-read`: the primitive gains an explicit boundary — a tab
  outside `http(s)` is refused rather than attempted.
- `panel-assistant-chat`: a page read is shown with the page it read.

## Impact

**Code.** `extension/lib/tools/page.ts` (the boundary), a new
`extension/lib/assistant/pageRead.ts` (reading the url back out of the tool's
result), `extension/entrypoints/sidepanel/ToolGroupList.svelte` (showing it).

**Not touched: `lib/assistant/tool-formatters.ts`.** It is a verbatim port of the
web app's, and its value is that it stays one. `read_current_page` exists only in
this surface, so its display logic lives beside the component that renders it
rather than diverging a shared file.

**Honest about what the boundary is worth.** A content script declared on
`<all_urls>` is already not injected into `chrome://`, the Web Store, or (absent an
explicit opt-in) `file://`, so those tabs already fail. What changes is that they
fail *deliberately and legibly*, and that the boundary stops depending on Chrome
continuing to read `<all_urls>` that way. This is not a fix for a leak that was
happening.

**Does not close the whole obligation.** A consent prompt — asking before reading a
host for the first time — remains unbuilt. This makes reads visible and bounded;
it does not make them opt-in.
