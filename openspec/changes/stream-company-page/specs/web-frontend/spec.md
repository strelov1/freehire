## MODIFIED Requirements

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

## ADDED Requirements

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
