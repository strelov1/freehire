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
`regions` parameter. Its options SHALL be a curated, extensible subset of the
reach vocabulary (Global, Russia, Europe, USA), each mapping to a `regions` code
(`global`, `ru`, `eu`, `us`). The facet SHALL support exclusion like the other
facets. The facet's option values SHALL be codes from the backend's `regions`
vocabulary.

#### Scenario: Filtering by a region

- **WHEN** a user selects the "Europe" pill in the Region facet
- **THEN** the search request carries `regions=eu` and the results are jobs whose
  reach includes Europe

#### Scenario: Excluding a region

- **WHEN** a user excludes the "USA" pill
- **THEN** the search request excludes `regions=us` and such jobs are omitted

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

#### Scenario: Jobs are listed

- **WHEN** a user opens the jobs route `/jobs`
- **THEN** the server returns HTML already containing the first page of job rows,
  each linking to its job detail

#### Scenario: User loads more jobs

- **WHEN** more jobs exist than the current page (`offset + limit < meta.total`)
- **THEN** a control lets the user fetch and view the next page

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

### Requirement: Company detail

The frontend SHALL show a single company from `GET /api/v1/companies/:slug`
together with its jobs, reusing the same job row presentation as the jobs list.
The page SHALL be **server-rendered** — the company info and its jobs present in
the initial HTML — and then hydrate on the client.

#### Scenario: Company detail is shown

- **WHEN** a user navigates to `/companies/:slug`
- **THEN** the server returns HTML already containing the company info and its
  jobs

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

