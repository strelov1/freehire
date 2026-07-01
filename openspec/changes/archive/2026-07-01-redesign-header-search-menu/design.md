## Context

The header lives in `web/src/lib/components/TopBar.svelte`, rendered once by
`web/src/routes/+layout.svelte`. Today it holds a logo, seven inline nav links
(collapsing to a hamburger below `sm`), a `UserMenu.svelte` avatar dropdown (for
signed-in users) or a Sign in button, and a `ThemeToggle`. Free-text search
exists only on `/jobs` (`JobsView.svelte`), bound to the URL `?q=` and the
Meilisearch-backed `GET /api/v1/jobs/search`.

The reference is telagon's `header-search.tsx`: a centered input with a
debounced, abortable instant-results dropdown, `Cmd+K`/`/` hotkeys, keyboard
navigation, and a mobile backdrop with scroll lock. We port that pattern to
Svelte 5 (runes) and freehire's Tailwind/`$lib/ui` conventions.

Constraints: web-only, no backend changes, reuse `api.searchJobs` and
`api.listCompanies`. The project uses Svelte 5 runes (`$state`/`$derived`/
`$effect`), `resolve()` for internal links, and has no unit-test runner for web
(verify via `svelte-check` + lint per repo memory).

## Goals / Non-Goals

**Goals:**

- One header layout — logo | search | menu — identical on desktop and mobile.
- Instant search with JOBS + COMPANIES sections, keyboard-driven, hotkey-focusable.
- A single menu that absorbs the nav links, the signed-in account items, theme
  toggle, and auth action.
- Polished mobile presentation (full-width overlays, backdrop, scroll lock).

**Non-Goals:**

- No command-palette overlay style (rejected in brainstorming).
- No new search endpoint, no server/DB changes.
- No change to `/jobs`'s own `q` filter input beyond it staying URL-synced.
- No fuzzy "actions" search (e.g. "toggle theme" as a result) — only jobs/companies.

## Decisions

### Component split

Rewrite `TopBar.svelte` as the layout shell owning the three slots and the menu
state. Extract two focused child components:

- `HeaderSearch.svelte` — the input, the debounced/abortable query, the dropdown
  (both sections, empty state), and all search keyboard/hotkey handling.
- `HeaderMenu.svelte` — the trigger button plus the menu panel (nav links,
  account items, theme toggle, auth action), driven by auth state.

Rationale: each unit has one purpose and a small surface. `UserMenu.svelte`'s
item list moves into `HeaderMenu.svelte`; the standalone `UserMenu.svelte` is
removed since it no longer has a separate home. Alternative — keeping everything
in `TopBar.svelte` — was rejected: the file would grow past a comfortable size
and mix three concerns (per the repo's "file doing too much" guidance).

### Data fetching for the dropdown

On each debounced non-empty query, fire `api.searchJobs(params, JOBS_LIMIT)` and
`api.listCompanies(q, COMPANIES_LIMIT)` concurrently. `params` carries `q=<query>`
(built via the existing `filtersToParams`/`URLSearchParams` shape the search API
reads). Guard against out-of-order responses with a monotonic request token (or
`AbortController`): only the latest query's results are applied. Limits kept
small (~6 jobs, ~4 companies) for a snappy dropdown.

Rationale: both endpoints already exist and return exactly the fields the
sections need (`Job.title/company/location/public_slug`; `CompanyListItem.name/
slug/job_count`). No aggregation layer needed.

### Navigation and URL sync

Result selection and full-search use SvelteKit `goto()` with `resolve()` for
internal paths (jobs `/jobs/<slug>`, companies `/companies/<slug>`, full search
`/jobs?q=<query>`). The header search does NOT own global query state — it is a
launcher. `/jobs`'s existing input remains the source of truth for list
filtering and stays URL-synced, so arriving via `/jobs?q=…` prefills it through
the page's existing `FilterStore` seeding. This satisfies "keep both" without
coupling the two inputs.

### Debounce, hotkeys, dismissal

Reuse telagon's proven values: ~250ms debounce; `Cmd/Ctrl+K` always focuses,
`/` focuses only when focus is not already in an input/textarea. Dropdown
dismissal on `Escape`, outside click (a `svelte:window` click handler checking
containment, matching the existing `UserMenu` pattern), and route change. The
menu uses the same dismissal set.

### Mobile presentation

Below `sm`, the search dropdown and the menu render as fixed full-width panels
with a semi-transparent backdrop; body scroll is locked via a class/style toggle
on `document.body` while either is open (mirroring telagon). A shared small
helper avoids duplicating the lock logic between the two overlays.

## Risks / Trade-offs

- **Two requests per keystroke-batch (jobs + companies)** → debounce + abort/token
  keeps it to one live pair per settled query; limits are tiny so payloads are small.
- **Body-scroll-lock leak** (lock not released if a component unmounts while open)
  → release in the same effect's cleanup (`$effect` return) and on close, so
  unmount always restores scroll.
- **Losing an affordance users relied on** (inline desktop links now one click
  deeper in the menu) → accepted per the chosen minimalist direction; the search
  field covers the most common intent (finding a job) without any menu at all.
- **No web unit-test runner** → verify behavior with `svelte-check`, lint, and a
  manual/headless-Chrome pass (per repo memory on web verification).

## Migration Plan

Pure front-end swap: replace the header component tree and delete the orphaned
`UserMenu.svelte`. No data migration, no feature flag. Rollback is reverting the
web commit. Ships through the normal `web`-only deploy path.

## Open Questions

None outstanding — layout, result sections, `/jobs` handling, hotkeys, and
result targets were all settled during brainstorming.
