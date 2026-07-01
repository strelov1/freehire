## Why

The current header spreads seven inline nav links, an avatar dropdown, and a
theme toggle across the bar, yet offers no way to search from anywhere — text
search only exists on `/jobs`. Users must first navigate to the jobs page to
look anything up. A single prominent search field plus a consolidated menu makes
the whole catalogue reachable from any page and declutters the header on both
desktop and mobile.

## What Changes

- Redesign the header into a unified three-slot layout: **logo | large search
  input | single hamburger menu**, identical on desktop and mobile.
- Add a large header search with an instant-results dropdown as the user types,
  split into two sections — **JOBS** (title, company, location) and
  **COMPANIES** (name, job count) — backed by the existing `searchJobs` and
  `listCompanies` API (no backend changes).
- Global hotkeys `Cmd/Ctrl+K` and `/` focus the search; arrow keys move the
  active result, `Enter` opens it (or, with no active result, navigates to
  `/jobs?q=…`), `Escape` closes the dropdown.
- Selecting a job result navigates to `/jobs/:slug`; a company result to
  `/companies/:slug`.
- **BREAKING (UI):** Consolidate the previously inline nav links AND the
  signed-in avatar dropdown into the single hamburger menu — nav links (Jobs,
  Companies, Collections, Analytics, CLI, For recruiters, For companies), the
  my-menu items (My jobs, Search profiles, Notifications, API keys, Submit a
  job, My submissions, Moderation), theme toggle, and sign in / out — with a
  backdrop and body-scroll lock on mobile.
- The `/jobs` page keeps its own `q` filter input; the header search `Enter`
  navigates to `/jobs?q=…` and the two stay in sync via the URL.

## Capabilities

### New Capabilities

- `header-navigation`: The site-wide header — its unified layout, the instant
  search field (dropdown sections, keyboard navigation, hotkeys, result
  targets), and the consolidated hamburger menu (nav links, signed-in
  account items, theme toggle, auth actions, mobile behavior).

### Modified Capabilities

<!-- No existing spec's requirements change; the header is a new capability. -->

## Impact

- `web/` only (SvelteKit SPA). Primary files: `web/src/lib/components/TopBar.svelte`
  (rewritten), a new header search component, and consolidation of
  `UserMenu.svelte`'s items into the menu. No API, DB, or server changes.
- Reuses existing endpoints `GET /api/v1/jobs/search` and `GET /api/v1/companies`
  via the current `api.searchJobs` / `api.listCompanies` client functions.
