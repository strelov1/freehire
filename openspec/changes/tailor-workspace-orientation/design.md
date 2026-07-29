## Context

Reported while using the workspace on production, with a screenshot: a CV opened by id showed
"Ask the agent anything to get started" and no actions, and the active tab was indistinguishable
from the inactive ones.

The first is a defect in the change that introduced the actions: they were gated on `resuming`,
a page-level flag meaning "opened with `?cv=`". The chat component already renders them only while
`chat.messages.length === 0`, so the flag was a second gate on the same question — and the two
disagree exactly when a bound conversation is empty, which is what a bootstrap leaves behind if
nobody presses anything.

## Goals / Non-Goals

**Goals:**
- An empty conversation always shows the way in, however it was reached.
- The current pane is obvious at a glance.
- The three sections people open mid-task are reachable without a detour.

**Non-Goals:**
- A general tab component. Two panels share a pattern; a third would be the time to extract one.
- Renaming routes. `/my/cvs` stays — the label changes, the address does not.
- Reordering the account navigation.

## Decisions

### One gate, in the component that renders

`openingFor(resuming)` becomes `openingActions()`. The host says WHAT it offers; the chat decides
WHEN, from the only state that matters (are there messages?). Two components answering the same
question is what produced the bug.

### The brand tint, not a darker grey

The palette's brand colour is already the olive (`--brand: #5b6f00`, with `--brand-muted` as its
soft tint and `--brand-strong` for readable text on it). Using it for the selected tab keeps the
selection consistent with every other "this one is chosen" affordance in the product, and needs no
new token.

### The header menu duplicates three sections, not all of them

Inbox was already there. Agent and Tailor join it because they are opened from a job page, a search,
or nothing in particular — the rest of the account sections are destinations you go to deliberately.

## Risks / Trade-offs

- **A longer header menu** → Three more rows on a menu that already scrolls on mobile, against a
  saved navigation each time; the sections chosen are the ones with the traffic.
- **"Tailor" as a noun is a little odd in English** → It matches the workspace it opens
  (`/tailor/<slug>`) and the verb the product uses everywhere else for this action.
