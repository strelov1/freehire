# web-frontend Specification

## Purpose
TBD - created by archiving change add-web-frontend. Update Purpose after archive.
## Requirements
### Requirement: API permits cross-origin browser access

The HTTP API SHALL respond with CORS headers that allow a browser running on a
different origin to call the read endpoints, so the frontend can fetch data
directly without a proxy.

#### Scenario: Browser preflight is allowed

- **WHEN** a browser sends an `OPTIONS` preflight to `/api/v1/jobs` with an
  `Origin` header
- **THEN** the response includes `Access-Control-Allow-Origin` matching the
  configured frontend origin and the request succeeds

#### Scenario: Cross-origin GET returns data

- **WHEN** the frontend issues a cross-origin `GET /api/v1/jobs`
- **THEN** the response carries `Access-Control-Allow-Origin` and the JSON body
  is readable by the browser

### Requirement: Region (remote reach) filter facet

The frontend job-search filter UI SHALL offer a curated "Region" facet, rendered
as pills under the "Work format" facet, that filters on the search API's
`regions` parameter. Its options SHALL be the macro-region reach vocabulary
(Global, North America, LATAM, Europe, UK, MENA, Africa, APAC, CIS), each mapping
to a `regions` code (`global`, `north_america`, `latam`, `eu`, `uk`, `mena`,
`africa`, `apac`, `cis`) — one consistent macro level, with country-level
filtering handled by the separate Countries facet. The facet SHALL support
exclusion like the other facets. The facet's option values SHALL be codes from
the backend's `regions` vocabulary.

#### Scenario: Filtering by a region

- **WHEN** a user selects the "Europe" pill in the Region facet
- **THEN** the search request carries `regions=eu` and the results are jobs whose
  reach includes Europe

#### Scenario: Excluding a region

- **WHEN** a user excludes the "North America" pill
- **THEN** the search request excludes `regions=north_america` and such jobs are
  omitted

### Requirement: Jobs list with pagination

The frontend SHALL present a list of jobs from `GET /api/v1/jobs`, showing for
each job its title, company, location, work arrangement (and, for remote roles,
its reach), source, and posted date, and SHALL paginate using the API's
`limit`/`offset` driven by `meta.total`. The work arrangement SHALL be derived
from `enrichment.work_mode`; for a remote role the reach SHALL be shown from
`enrichment.regions` (e.g. `Global`, `Europe`). The frontend SHALL NOT rely on a
raw `remote` field (it is no longer in the API). The first page of the list
SHALL be **server-rendered** — its rows present in the initial HTML — and then
hydrate on the client for subsequent interaction.

When more jobs remain, the frontend SHALL load the next page automatically once
the user scrolls to the bottom of the current list (infinite scroll), without
pre-fetching ahead of the viewport. An accessible control SHALL remain available
as a fallback so that keyboard and screen-reader users — who do not
scroll-trigger — can load the next page, and so that a failed page load can be
retried.

#### Scenario: Jobs are listed

- **WHEN** a user opens the jobs route `/jobs`
- **THEN** the server returns HTML already containing the first page of job rows,
  each linking to its job detail

#### Scenario: User reaches the bottom of the list

- **WHEN** more jobs exist than the current page (`offset + limit < meta.total`)
  and the user scrolls to the bottom of the loaded rows
- **THEN** the next page is fetched and appended automatically, without the user
  clicking a control

#### Scenario: Fallback control loads and retries

- **WHEN** more jobs remain but the user navigates by keyboard or a page load
  failed
- **THEN** an accessible control is present that fetches the next page on
  activation, and a failed load surfaces a retry affordance rather than silently
  stopping

#### Scenario: A global-remote job shows its reach explicitly

- **WHEN** a listed job has `work_mode=remote` and `regions=[global]`
- **THEN** its row shows a "Global" reach indicator rather than a bare "Remote"
  with no reach

### Requirement: Job detail

The frontend SHALL show a single job from the public job API at the route
`/jobs/:slug` with its title, company link, work-arrangement/source badges,
posted date, description, and a link to the external posting URL. For a remote
role the displayed facets SHALL convey reach from `enrichment.regions` rather
than a raw `remote` flag. The page SHALL be **server-rendered** — the job's
fields present in the initial HTML — and then hydrate on the client.

#### Scenario: Job detail is shown

- **WHEN** a user navigates to `/jobs/:slug`
- **THEN** the server returns HTML already containing the job's fields, with an
  "Apply" link pointing to `job.url`

#### Scenario: Missing job

- **WHEN** the API returns 404 for the requested slug
- **THEN** the view shows an error state instead of broken content

#### Scenario: Remote reach is shown on detail

- **WHEN** a job has `work_mode=remote` and `regions=[eu]`
- **THEN** the detail view conveys a Europe reach rather than only "Remote"

### Requirement: Companies list

The frontend SHALL present companies from `GET /api/v1/companies`, showing each
company's name and its job count, with each row linking to the company detail.

The page SHALL provide a name-search input. Typing SHALL filter the list against
the API's `q` parameter (debounced), and the current query SHALL be mirrored into
the URL query string (`?q=`) so a search survives reload, sharing, and
back/forward navigation. The page SHALL show the count of matching companies and
a distinct empty state when a search matches nothing.

The page SHALL present filters through the same two-pane "All filters" modal and
chip-summary sidebar the jobs list uses (the reusable filter shells), over the
company facet set: **collection**, **region**, **country**, **industry** (domains),
**company type**, and **company size**, reusing the jobs filter controls and
closed-vocabulary option registries (country is a searchable select over the country
list; the others are pill/select controls over their fixed vocabularies). On desktop
the sidebar SHALL show the applied-facet chips plus an **All filters** button opening
the modal; on narrow viewports the modal SHALL open from a pinned left-edge tab. As
in the jobs modal, facet edits SHALL be staged and applied on **Show results**;
applying SHALL refetch the list against the corresponding repeatable API facet
parameters and mirror the active facets into the URL query string, so a filtered view
survives reload, sharing, and back/forward navigation, composably with the `q` search.
The summary sidebar SHALL offer a **Reset all** control.

#### Scenario: Companies are listed

- **WHEN** a user opens `/companies`
- **THEN** a page of companies is fetched and rendered with job counts

#### Scenario: User searches companies by name

- **WHEN** a user types a query into the companies search input
- **THEN** the list is refetched filtered by that query and the URL query string
  is updated to `?q=<query>`

#### Scenario: Search restored from the URL

- **WHEN** a user opens `/companies?q=acme` directly or via back/forward
- **THEN** the search input is prefilled with `acme` and the filtered list is
  shown

#### Scenario: Search matches nothing

- **WHEN** a search returns no companies
- **THEN** an empty state ("No matching companies.") is shown instead of an empty
  list

#### Scenario: User filters companies by a facet

- **WHEN** a user opens the **All filters** modal, selects a region, and activates
  **Show results**
- **THEN** the list is refetched against `?regions=<value>` and the URL query
  string reflects the active facet

#### Scenario: Filters restored from the URL

- **WHEN** a user opens `/companies?collections=yc&regions=europe` directly or via
  back/forward
- **THEN** the summary sidebar shows those facets active and the list is filtered to
  match

#### Scenario: Clearing filters

- **WHEN** a user activates **Reset all**
- **THEN** the facet parameters are removed from the URL and the full list (for the
  current `q`, if any) is shown

### Requirement: Company detail

The frontend SHALL show a single company from `GET /api/v1/companies/:slug`
together with its jobs, reusing the same job row presentation as the jobs list.
The company entity (name, logo, facets) and its SEO metadata (title, canonical,
JSON-LD) SHALL be **server-rendered** — present in the initial HTML — and then
hydrate on the client. The job list SHALL be **streamed** independently of the
company entity: the page load SHALL NOT block on the (slower) job-search query,
and while the job list is pending the frontend SHALL render a job-list
**skeleton** in its place. On a client-side navigation into a company page, the
company header SHALL become visible as soon as the company entity resolves,
without waiting for the job list.

#### Scenario: Company entity and SEO are server-rendered

- **WHEN** a user navigates directly to `/companies/:slug`
- **THEN** the server returns HTML already containing the company info and its
  SEO metadata (title, canonical, organization JSON-LD)

#### Scenario: Job list streams behind a skeleton

- **WHEN** the company page is rendered and its job-search result is still pending
- **THEN** a job-list skeleton is shown in place of the rows until the streamed
  results arrive, and the company header is already visible

#### Scenario: Client navigation shows the header before the jobs

- **WHEN** a user clicks a company from the companies list
- **THEN** the company header renders as soon as the company entity resolves,
  before the job list has loaded

### Requirement: Global navigation progress indicator

The frontend SHALL display a global progress indicator during any client-side
navigation, driven by SvelteKit's reactive `navigating` state (`$app/state`), so
the user gets immediate visual feedback the moment a navigation begins and until
it completes.

#### Scenario: Indicator appears on navigation

- **WHEN** a client-side navigation is in flight
- **THEN** a progress indicator is visible in the root layout

#### Scenario: Indicator clears when navigation settles

- **WHEN** the navigation completes (or is aborted)
- **THEN** the progress indicator is no longer shown

### Requirement: Light and dark theme

The frontend SHALL support light, dark, and system themes, applying dark mode via
a `.dark` class on the document root, persisting the choice in localStorage, and
tracking `prefers-color-scheme` when in system mode.

#### Scenario: User toggles theme

- **WHEN** a user activates the theme toggle
- **THEN** the interface switches between light and dark and the choice persists
  across reloads

#### Scenario: System mode follows OS preference

- **WHEN** the theme is set to system
- **THEN** the effective theme matches the OS `prefers-color-scheme` and updates
  if the OS preference changes

### Requirement: Async load states

Every data-driven view SHALL render distinct loading, empty, and error states so
the user is never shown broken or blank content during or after a fetch.

#### Scenario: Loading state

- **WHEN** a view's request is in flight
- **THEN** a loading indicator is shown until data or an error arrives

#### Scenario: Empty state

- **WHEN** a successful response contains no items
- **THEN** an empty-state message is shown instead of an empty list

#### Scenario: Error state

- **WHEN** a request fails (network or non-2xx)
- **THEN** an error message is shown

### Requirement: The job page renders a closed state

When a job view carries `closed_at`, the job page SHALL show that the position is
no longer accepting applications and SHALL NOT render the Apply action. Open jobs
are unaffected.

#### Scenario: Closed job shows the closed state

- **WHEN** a signed-in or anonymous user opens a closed job's page
- **THEN** the page shows a "no longer accepting applications" notice instead of
  the Apply button

### Requirement: API key management page

The SPA SHALL provide an API-keys management page at `/my/api-keys`, reachable
from the authenticated user menu, where a signed-in user can list, create, and
revoke their API keys. The list SHALL show each key's name, display prefix,
created time, last-used time (or "never"), and expiry. Creating a key SHALL
reveal the full plaintext token **once**, with a copy control and a ready-to-run
`curl` example that sends `Authorization: Bearer <key>`, alongside a notice that
the token will not be shown again. Revoking a key SHALL require an explicit
confirmation. The page and its menu entry SHALL be available only to signed-in
users.

#### Scenario: Reaching the page from the user menu

- **WHEN** a signed-in user opens the user menu and selects "API keys"
- **THEN** the SPA navigates to `/my/api-keys` and lists the user's keys with name,
  prefix, created, last-used, and expiry

#### Scenario: Creating a key reveals the secret once

- **WHEN** the user creates a key (name, optional expiry)
- **THEN** the SPA shows the full plaintext token with a copy control, a `curl`
  example using `Authorization: Bearer <key>`, and a "won't be shown again" notice
- **AND** the new key appears in the list

#### Scenario: The secret is not shown again

- **WHEN** the user dismisses the reveal or navigates away and returns
- **THEN** the page shows only the key's metadata (including its prefix), never the
  full token again

#### Scenario: Revoking a key

- **WHEN** the user revokes a key and confirms the action
- **THEN** the key is removed from the list

#### Scenario: Signed-out users have no access

- **WHEN** a signed-out user has no session
- **THEN** the user menu offers no "API keys" entry and the page is not presented
  as an authenticated surface

### Requirement: Jobs browse sort control

The jobs browse UI SHALL provide a sort control offering two options: **Date
posted** (the source's `posted_at`) and **Recently added** (`created_at`), each
ordered newest first. Selecting an option SHALL refetch the list ordered by that
field. The selection SHALL be mirrored into the URL query string (`?sort=`,
alongside the existing filter params) so it survives reload, sharing, and
back/forward navigation. The default selection SHALL be **Date posted**, and the
URL SHALL omit `?sort=` while the default is active (kept clean, like an empty
search query).

#### Scenario: Default sort is by posting date

- **WHEN** a user opens the jobs page with no `sort` in the URL
- **THEN** the control shows "Date posted" and the list is ordered by
  `posted_at` descending

#### Scenario: User switches to recently added

- **WHEN** a user selects "Recently added"
- **THEN** the list is refetched ordered by `created_at` descending and the URL
  query string is updated to include `sort=created_at`

#### Scenario: Sort restored from the URL

- **WHEN** a user opens the jobs page with `?sort=created_at` directly or via
  back/forward
- **THEN** the control is preset to "Recently added" and the list is ordered by
  `created_at` descending

### Requirement: Analytics page

The web frontend SHALL provide a public `/analytics` page that visualizes the
facet-distribution counts from `GET /api/v1/jobs/facets`. The page SHALL render
each facet as a breakdown of values with their vacancy counts, sorted by count
descending, and SHALL display the total number of vacancies under the current
filters.

The page SHALL be server-side rendered for its initial state (counts under the
empty filter) so it is indexable and usable without client-side JavaScript for
the first paint.

#### Scenario: Initial render

- **WHEN** a visitor opens `/analytics`
- **THEN** the page shows the total vacancy count and per-facet breakdowns for
  the unfiltered catalogue, rendered server-side

#### Scenario: Breakdown ordering

- **WHEN** a facet breakdown is shown
- **THEN** its values are listed from highest count to lowest, each with its
  count and a proportional bar

### Requirement: Analytics drill-down

The analytics page SHALL let a visitor narrow the result set interactively by
selecting facet values, reusing the same URL-synced filter model as the jobs
browse page. Selecting a value SHALL update the URL and recompute every breakdown
to reflect the new filter set; the selection SHALL survive a page reload via the
URL.

#### Scenario: Selecting a facet value narrows the counts

- **WHEN** a visitor selects a value in a breakdown (e.g. `category = backend`)
- **THEN** the URL gains the corresponding filter param and all breakdowns and
  the total recompute to reflect the narrowed set

#### Scenario: Filters persist across reload

- **WHEN** a visitor reloads `/analytics` with filter params in the URL
- **THEN** the page renders the breakdowns and total for those filters

### Requirement: Open-vocabulary facet filters are distribution-driven selects

Facets with an open or high-cardinality vocabulary (Skills and Countries) SHALL
be filtered through a searchable select whose options come from the live facet
distribution (`GET /api/v1/jobs/facets`) rather than free-text entry, so the user
sees which values exist and how many open jobs each has under the current
filters. Each option SHALL display its job count, and options SHALL be ordered
by count (busiest first). Country options SHALL be labelled with a human-readable
country name derived from the ISO code. A value already selected but absent from
the current distribution SHALL remain listed so it stays removable.

The job-search view SHALL fetch the distribution under the same filter params as
the result list, debounced, and SHALL discard a stale (superseded) response so
the counts never reflect an older filter state.

#### Scenario: Skills/Countries options come from the distribution with counts

- **WHEN** the user opens the Skills or Countries filter section
- **THEN** the selectable options are the values present in the current facet
  distribution, each labelled with its job count and ordered busiest-first

#### Scenario: Country codes are shown as names

- **WHEN** a country option for ISO code `de` is rendered
- **THEN** its label reads `Germany`, not `de`

#### Scenario: A stale distribution response is ignored

- **WHEN** filters change rapidly and an earlier distribution request resolves
  after a later one
- **THEN** the later (current) response wins and the earlier one is discarded

### Requirement: Pill facets keep removed-vocabulary selections removable

A pill-control facet SHALL render any currently-selected value that has no
matching option as an active, removable pill, so a value removed from the
controlled vocabulary after a bookmark or saved search was created does not
become an invisible filter the user cannot clear from the UI.

#### Scenario: A removed region value stays removable

- **WHEN** the active filters include a region value no longer in the region
  vocabulary (e.g. an old `?regions=us` link after the macro-region change)
- **THEN** that value renders as an active pill the user can click to remove,
  rather than silently constraining results with no visible control

### Requirement: Responsive URL-synced search and filter input

The frontend's search and filter inputs SHALL stay responsive while the user
types — across the jobs search box and facet filters, the companies name search,
and the analytics filters — so that characters are never dropped or reverted
during fast typing, even while a data reload triggered by an earlier keystroke is
still in flight. The current search/filter state SHALL be mirrored into the URL
query string so it survives reload, sharing, and browser back/forward, and the
data reload that reflects a change SHALL be debounced so that typing does not
issue one request per character.

#### Scenario: Fast typing keeps every character

- **WHEN** a user types into a search/filter input faster than the reload
  debounce window, and an earlier reload is still in flight
- **THEN** the input retains exactly what was typed — no characters are dropped or
  reverted by the in-flight reload completing

#### Scenario: Filter state survives back/forward

- **WHEN** a user changes filters, navigates away, and returns via browser
  back/forward
- **THEN** the inputs and results are restored to match the URL of that history
  entry

#### Scenario: Typing debounces the reload

- **WHEN** a user types a multi-character query in quick succession
- **THEN** the URL updates to reflect the query but the data reload is coalesced
  rather than issued once per keystroke

### Requirement: Company filter facet is a lazy, server-backed typeahead

The frontend job-search filter UI SHALL offer a "Company" facet that sources its
options from the companies endpoint (`GET /api/v1/companies?q=`), not from the
Meilisearch facet distribution. As the user types, the facet SHALL query the
endpoint (debounced) and present matching companies by their display `name`
alongside their global open-job count, ordered most-active first. With an empty
query the facet SHALL present the most popular companies (the endpoint's
count-ordered first page). Selecting a company SHALL apply the existing
`company_slug` search parameter, so URL state, exclusion, and the selected-value
chips behave identically to the other facets. The count shown for a company is
its global open-job count, not a count contextual to the other active filters.

#### Scenario: Typing finds a company outside the top results

- **WHEN** a user types "google" into the Company facet
- **THEN** the facet queries `GET /api/v1/companies?q=google` and lists matching
  companies by name with their job counts, even though "google" is not among the
  most popular companies shown for an empty query

#### Scenario: Empty query shows the most popular companies

- **WHEN** a user opens the Company facet without typing
- **THEN** the facet shows the most active companies first (highest job count),
  sourced from the endpoint's first page

#### Scenario: Selecting a company filters the job list

- **WHEN** a user selects a company in the facet
- **THEN** the search request carries that company's `company_slug` and the job
  results are limited to that company, and a removable chip shows the company's
  display name

#### Scenario: A pre-selected company from the URL is shown by name

- **WHEN** the page loads with `?company_slug=stripe` already set
- **THEN** the Company facet shows the selection as a removable chip with a
  human-readable label, without requiring the user to first search for it

### Requirement: Already-viewed jobs are visually marked in the browse list

The SPA SHALL visually de-emphasise job cards that the signed-in user has already
viewed, in both the jobs list and the search results, so they can tell at a
glance what they have already opened. The marking SHALL be driven by the set of
viewed slugs read from `GET /api/v1/me/jobs/viewed`, loaded once when a signed-in
user opens the browse view. A viewed card SHALL be dimmed (reduced opacity) and
SHALL return to full strength on hover to signal it remains clickable. For
anonymous (signed-out) visitors no card SHALL be dimmed. Surfaces where every
listed job is by definition already viewed (the My Jobs history and board) SHALL
NOT dim their cards.

#### Scenario: Signed-in user sees viewed jobs dimmed

- **WHEN** a signed-in user who has viewed some jobs opens the jobs list or runs
  a search
- **THEN** the cards for jobs in their viewed-slug set are rendered dimmed
- **AND** cards for jobs they have not viewed are rendered at full strength

#### Scenario: Hovering a viewed card restores it

- **WHEN** the user hovers a dimmed (viewed) job card
- **THEN** the card returns to full strength while hovered

#### Scenario: Anonymous visitor sees no dimming

- **WHEN** a signed-out visitor opens the jobs list or runs a search
- **THEN** no job card is dimmed

#### Scenario: Opening a job marks it viewed without a reload

- **WHEN** a signed-in user opens a job from the list and navigates back
- **THEN** that job's card is shown dimmed without requiring a full reload

#### Scenario: My Jobs surfaces are not dimmed

- **WHEN** a signed-in user opens the My Jobs history or board, where every card
  is already viewed
- **THEN** no card is dimmed

### Requirement: My jobs page

The web SPA SHALL provide a `/my/jobs` page for signed-in users listing their
job interactions with All / Saved / Applied tabs (with per-tab counts), reusing
the standard job-row presentation and linking each row to the job page. The
page SHALL be reachable from the user menu. A signed-out user navigating to it
SHALL be prompted to sign in instead of seeing an error.

#### Scenario: Viewing applications

- **WHEN** a signed-in user opens `/my/jobs` and selects the Applied tab
- **THEN** the SPA lists the jobs they marked applied, most recently touched
  first, each row linking to the job page

#### Scenario: Tab counts

- **WHEN** the page loads
- **THEN** each tab shows the count of interactions it would list

#### Scenario: Signed-out visitor

- **WHEN** a signed-out user navigates to `/my/jobs`
- **THEN** the SPA shows a sign-in prompt, not the listing and not an error page

### Requirement: Save toggle on the job page

The web SPA SHALL show a Save/Saved toggle on the job detail page for signed-in
users, reflecting the saved state returned by the silent view recording and
flipping via the save/unsave endpoints. Signed-out users SHALL NOT see the
toggle.

#### Scenario: Saving from the job page

- **WHEN** a signed-in user clicks Save on a job page
- **THEN** the SPA calls the save endpoint and the button reflects the saved
  state from the response

#### Scenario: Unsaving

- **WHEN** a signed-in user clicks the toggle on an already-saved job
- **THEN** the SPA calls the unsave endpoint and the button returns to the
  unsaved state

#### Scenario: Signed-out user

- **WHEN** a signed-out user opens a job page
- **THEN** no Save toggle is rendered

### Requirement: Profile filters appear only on the Market coverage tab

The `/my/profile` page SHALL expose its role/filter controls (the summary sidebar,
the mobile left-edge tab, and the two-pane modal) only while the **Market coverage**
tab is active, since coverage is the only view computed against ad-hoc filters. On the
**Your CV** and **CV readiness** tabs the filter controls SHALL NOT be shown, and CV
readiness SHALL be scored against the profile's default role. The profile page SHALL
NOT present the "My filters" (saved-search) tab.

#### Scenario: Filters shown on Market coverage

- **WHEN** a signed-in user with a profile opens the **Market coverage** tab
- **THEN** the filter summary, the mobile filters tab, and the **All filters** modal
  are available to refine the comparison role

#### Scenario: Filters hidden on other tabs

- **WHEN** the user switches to the **Your CV** or **CV readiness** tab
- **THEN** no filter summary, mobile tab, or modal is shown

#### Scenario: No My filters on the profile

- **WHEN** the profile's **All filters** modal is opened
- **THEN** it presents the facet rail without a "My filters" (saved-search) tab

