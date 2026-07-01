<!-- Note: web/ has no unit-test runner (per repo convention). Each task is
     verified with `svelte-check` + lint and, for behavior, a manual/headless
     pass — standing in for the RED→GREEN test step. -->

## 1. Scaffolding & shared helpers

- [x] 1.1 Add a body-scroll-lock helper (lock/unlock `document.body`) reusable by both overlays, with cleanup on release
- [x] 1.2 Confirm `api.searchJobs` and `api.listCompanies` signatures and the `Job` / `CompanyListItem` fields the dropdown renders; note the `q` param shape for `searchJobs`

## 2. HeaderSearch component

- [x] 2.1 Create `HeaderSearch.svelte` with the input, `Search` icon, clear button, and `/` `kbd` hint (empty-query state)
- [x] 2.2 Implement debounced (~250ms) querying that fires `searchJobs` + `listCompanies` concurrently with small limits, discarding stale responses (request token / abort)
- [x] 2.3 Render the dropdown: JOBS section (title, company, location), COMPANIES section (name, job count), and a no-match empty state
- [x] 2.4 Wire result selection — job → `/jobs/:slug`, company → `/companies/:slug` (via `goto`+`resolve`); clear query and close on select
- [x] 2.5 Implement keyboard control: Arrow up/down active index, Enter opens active result or navigates to `/jobs?q=…`, Escape closes; outside-click and route-change dismissal
- [x] 2.6 Add global hotkeys `Cmd/Ctrl+K` (always) and `/` (only when not in an input/textarea) to focus the field
- [x] 2.7 Mobile: full-width dropdown + backdrop, engage the scroll-lock helper while open

## 3. HeaderMenu component

- [x] 3.1 Create `HeaderMenu.svelte` with the trigger button (Menu/X icon) and the panel; port `UserMenu.svelte`'s items
- [x] 3.2 Populate the menu: nav links (Jobs, Companies, Collections, Analytics, CLI, For recruiters, For companies) with active-state marking
- [x] 3.3 Add auth-aware section: signed-in account items (My jobs, Search profiles, Notifications, API keys, Submit a job, My submissions) + Log out; moderator-only Moderation; signed-out Sign in action
- [x] 3.4 Add the theme toggle inside the menu; close the menu on item select, Escape, and outside click
- [x] 3.5 Mobile: full-width panel + backdrop, engage the scroll-lock helper while open

## 4. TopBar assembly & cleanup

- [x] 4.1 Rewrite `TopBar.svelte` as the three-slot shell (logo | `HeaderSearch` | `HeaderMenu`), preserving the existing auth-dialog wiring (`?auth_error` / `?auth=required`)
- [x] 4.2 Remove the old inline nav, mobile nav panel, and standalone avatar/Sign in from `TopBar.svelte`; delete the now-orphaned `UserMenu.svelte` (also deleted orphaned `ThemeToggle.svelte`)
- [x] 4.3 Verify `/jobs?q=…` from a header Enter prefills the `/jobs` filter input (URL-synced) and no console/nav regressions

## 5. Verification

- [x] 5.1 Run `svelte-check` and lint clean for the touched files
- [x] 5.2 Manual/headless pass across the spec scenarios (headless Chrome, mocked API): three-slot layout desktop+mobile ✓, JOBS+COMPANIES dropdown ✓, empty state ✓, arrow-key highlight ✓, `/` hotkey focus + Escape ✓, signed-out menu ✓, mobile full-width menu + search dropdown with backdrop ✓, no console errors ✓ (signed-in menu / moderator item / result-click nav are code-verified — need a live backend + auth)
