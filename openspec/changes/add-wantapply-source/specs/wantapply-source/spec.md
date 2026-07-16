## ADDED Requirements

### Requirement: Wantapply sitemap crawl

The system SHALL provide a `wantapply` source adapter that crawls the Wantapply site over its
WAF-free mirror host `https://wantapply.cy`. The adapter is boardless and an aggregator (it stays
in the source facet and takes each posting's company from the posting). It SHALL discover vacancies
from `https://wantapply.cy/sitemap.xml`, treating a `<loc>` as a vacancy candidate only when its
path has exactly one segment and that segment is not a reserved page. Reserved (non-vacancy) paths
are the static set (`/`, `/create`, `/sign-in`, `/sign-up`, `/privacy-policy`, `/terms-of-service`)
and any multi-segment path (notably `/company/*` and `/jobs/*`). The vacancy slug SHALL be the
posting's `ExternalID` and the stored URL SHALL be `https://wantapply.cy/<slug>`.

#### Scenario: Vacancy slug becomes a candidate

- **WHEN** the sitemap contains `<loc>https://wantapply.cy/android-team-lead-at-tradingview-3</loc>`
- **THEN** the adapter treats it as a vacancy candidate with `ExternalID` `android-team-lead-at-tradingview-3` and URL `https://wantapply.cy/android-team-lead-at-tradingview-3`

#### Scenario: Non-vacancy paths are filtered out

- **WHEN** the sitemap contains `/company/tradingview`, `/jobs/data-analyst`, `/sign-in`, and the root `/`
- **THEN** the adapter yields none of them as vacancy candidates

### Requirement: Wantapply hydrates only unseen vacancies

The adapter SHALL implement the hydrating crawl: it receives a `seen(externalID)` predicate and
fetches a vacancy's detail page only for a slug the catalogue does not already have, so a
steady-state crawl issues detail requests only for new vacancies. For an unseen slug it SHALL GET
`https://wantapply.cy/<slug>`, extract the page's `JobPosting` JSON-LD, and map it into a `Job`:
title from `title`; company from `hiringOrganization.name`; description from the sanitized
`description` HTML; posted-at from `datePosted`; location from the `jobLocation[].address`
components; work mode `remote` (and the `Remote` flag) when `jobLocationType` is `TELECOMMUTE`;
and employment type mapped from the first `employmentType` entry into freehire's employment-type
vocabulary (`FULL_TIME` → `full-time`), left empty when it does not map. For a seen slug the adapter
SHALL yield a liveness-refresh `Job` (marked, no detail request) so the pipeline refreshes the row's
last-seen and open state WITHOUT rewriting its already-hydrated content. A vacancy whose detail page
has no `JobPosting` JSON-LD or no company SHALL be dropped (it cannot be normalized). Detail requests
SHALL be bounded (a single crawl SHALL NOT issue unbounded concurrent detail requests), and one
vacancy's detail failure SHALL be isolated: the adapter logs and skips that vacancy rather than
aborting the crawl.

#### Scenario: New vacancy is hydrated from JobPosting

- **WHEN** the crawl encounters a slug whose `ExternalID` is not in the seen set and its detail page carries a `JobPosting` with `title`, `hiringOrganization.name`, `description`, and `datePosted`
- **THEN** the adapter fetches that page and yields a `Job` with title, company, sanitized description, posted-at, and location taken from the JSON-LD

#### Scenario: Remote vacancy sets the work mode

- **WHEN** an unseen vacancy's `JobPosting` has `jobLocationType` equal to `TELECOMMUTE`
- **THEN** the yielded `Job` has `Remote` true and `WorkMode` `remote`

#### Scenario: Already-ingested vacancy skips the detail request and preserves content

- **WHEN** the crawl encounters a slug whose `ExternalID` is already in the seen set
- **THEN** the adapter yields a liveness-refresh `Job` (marked, no detail request), and the pipeline only refreshes the row's last-seen/open state — its stored description and facets are not overwritten

#### Scenario: A vacancy with no JobPosting is dropped

- **WHEN** an unseen vacancy's detail page has no `JobPosting` JSON-LD (e.g. a closed posting returning an empty page)
- **THEN** the adapter drops that vacancy and continues crawling the rest

#### Scenario: A single detail failure does not abort the crawl

- **WHEN** the detail request for one unseen vacancy fails
- **THEN** the adapter logs and skips that vacancy and continues crawling the remaining vacancies

### Requirement: Wantapply closes via the unseen-sweep

The `wantapply` adapter SHALL NOT be self-closing: it re-lists every current vacancy each crawl
(hydrating new ones and liveness-refreshing seen ones), so the pipeline's post-run unseen-sweep is
the authoritative close signal. A vacancy that drops out of the sitemap is no longer listed or
refreshed and SHALL be closed by the sweep after its unseen window.

#### Scenario: Vanished vacancy is closed by the sweep

- **WHEN** a vacancy previously ingested from `wantapply` no longer appears in the sitemap across the sweep's unseen window
- **THEN** the post-run unseen-sweep closes that vacancy (it is not excluded as a self-closing source)
