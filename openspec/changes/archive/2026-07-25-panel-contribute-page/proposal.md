## Why

When the panel cannot recognise the page as a catalog job, it falls back to matching the
scraped text — a card with the host for a company, whatever headline the scraper found for
a title, and none of the catalog's enrichment. On a himalayas posting that card read "This
page — 0%", for a vacancy freehire actually carries.

The recognition half of that is fixed server-side (a page URL now resolves against the
posting's stored URL, not just the handful of ATS URL shapes a parser understands). What
remains is the honest case: a vacancy we genuinely do not carry. The user is standing on
it, and there is nothing they can do about it.

freehire can now take it: `POST /api/v1/jobs/resolve` reads the page, imports the vacancy
when an adapter can parse it, and queues the link for a maintainer when none can.

## What Changes

- When no catalog job backs the current page, the panel offers **Add to freehire** — one
  button, on both the ad-hoc match card and the empty state.
- Pressing it posts the page URL and reports what happened:
  - imported → the curated card replaces the ad-hoc one, no reload needed
  - already ours → the curated card appears (the page was recognised after all)
  - queued → a note that freehire will look at the link
- Failures say so and leave the panel as it was; the button never blocks the chat.

## Capabilities

### New Capabilities
- `panel-contribute-page`: offering the current page to the catalog from the side panel
  when freehire does not carry it.

## Impact

- `lib/freehire.ts` — `resolveJob()` over the new endpoint, plus the pure status→notice
  mapping the panel renders.
- `entrypoints/sidepanel/App.svelte` — the button, its busy state, and re-loading the
  match on a resolved slug.
- No new permission: the panel already reads the active tab's URL for the match.
