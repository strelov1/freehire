## Context

`read_page` (`lib/tools/page.ts`) asks the background for a snapshot of the active
tab and hands it back over the browser-tool wire. hire's `read_current_page` calls
it mid-turn and persists the result into the conversation transcript.

Nothing between those two points decides *whether* the page should be read, and
nothing afterwards says *which* page was.

## Goals / Non-Goals

**Goals:**

- A tab outside `http(s)` is refused deliberately, with a reason the model can
  relay.
- The transcript attributes each read to a page.

**Non-Goals:**

- Asking permission. A per-host consent prompt is a separate feature with its own
  problems — the agent is mid-turn behind a 15-second deadline, and a prompt needs
  a management surface for the answers. Not built here.
- Filtering page *content*. Whether a read page contains something sensitive is a
  different question from whether the tab should have been read at all.

## Decisions

**The boundary is checked from the tab's url, in the extension.** This is the only
place that knows the url before the page is read — hire, by the time it could
check, is holding the page. `page.ts` already queries the active tab, so this is a
guard on a value it can have for free.

**Refuse by naming the rule, not the symptom.** "no page to read: the active tab
could not be reached" describes what happened; "I can only read ordinary web pages"
tells the model something it can say to the user. The refusal text is read by a
model whose only way to help is to explain.

**Attribution reads the url back out of the tool's result, not its arguments.**
`read_current_page` takes no arguments — the url exists only in what came back. So
the display is derived from `ToolCall.result`, which the formatters already carry.

**It lives beside the component, not in `tool-formatters.ts`.** That file is a
verbatim port of the web app's and its worth is that it stays one; `read_current_page`
cannot occur in the web, so putting its display logic there would start a
divergence in the one file whose value is having none. A small
`lib/assistant/pageRead.ts` keeps the logic pure and testable, and
`ToolGroupList.svelte` — already rewritten for this panel — calls it.

**Query and fragment are dropped.** Session tokens live there. The origin and path
answer "which page", which is the question being asked.

## Risks / Trade-offs

**The boundary is mostly belt-and-braces.** `<all_urls>` already excludes
`chrome://` and friends, so these tabs fail today — noisily and by accident. The
value is a deliberate, explainable failure and independence from that Chrome
behaviour. Overstating it would be dishonest; omitting it would leave the failure
mode unexplained.

**Attribution is after the fact.** The user sees the url once it has been read and
sent. Showing it before would mean a prompt, which is the non-goal above. This
makes reads auditable, not preventable.

## Migration Plan

None. No stored state, no schema, no wire change — `read_page`'s success shape is
unchanged and hire needs no counterpart edit.
