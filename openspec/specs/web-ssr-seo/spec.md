# web-ssr-seo Specification

## Purpose
TBD - created by archiving change web-sveltekit-ssr. Update Purpose after archive.
## Requirements
### Requirement: Public pages are server-rendered

The frontend SHALL render the public read pages — the jobs list (`/jobs`), job
detail (`/jobs/:slug`), companies list (`/companies`), and company detail
(`/companies/:slug`) — on the server, so the initial HTML response contains the
page's primary content (job titles/descriptions, company info) before any
client JavaScript runs. The page SHALL then hydrate on the client to become
interactive. The data needed for SSR SHALL be fetched server-side against the
backend API, forwarding the incoming request's session cookie so an
authenticated request renders the correct content.

#### Scenario: Job detail content is in the initial HTML

- **WHEN** a crawler or client requests `GET /jobs/:slug` for an existing job
- **THEN** the returned HTML body already contains the job's title, company, and
  description text (not an empty `<div id="app"></div>` shell)

#### Scenario: Listing content is in the initial HTML

- **WHEN** a crawler or client requests `GET /jobs` or `GET /companies`
- **THEN** the returned HTML body already contains the first page of rendered
  rows

#### Scenario: A missing job server-renders an error state

- **WHEN** the backend returns 404 for the requested slug
- **THEN** the server responds with an error page (not a 200 empty shell), and
  the response status reflects that the resource is not found

### Requirement: Per-route document metadata

Each server-rendered page SHALL emit route-specific document `<head>` metadata in
its initial HTML: a descriptive `<title>`, a `<meta name="description">`, a
canonical URL, and Open Graph / Twitter Card tags. The job-detail page's title
and description SHALL derive from the job (e.g. title and company), replacing the
single static `freehire` title used for every URL.

The job-detail page SHALL additionally emit an `og:image` pointing at that job's
preview image (`/jobs/:slug/og.png`) as an absolute URL, with its declared
dimensions (1200×630) and an `og:image:alt`, and SHALL use the
`summary_large_image` Twitter Card with a matching `twitter:image`. The metadata
component SHALL treat the preview image as optional: routes that pass no image
keep the prior `summary` Twitter Card behaviour and emit no `og:image`.

#### Scenario: Job page has a job-specific title and canonical

- **WHEN** `GET /jobs/:slug` is requested for an existing job
- **THEN** the HTML `<head>` contains a `<title>` built from the job's title and
  company, a `<meta name="description">`, a `<link rel="canonical">` to the
  job's canonical URL, and Open Graph tags

#### Scenario: Job page emits a large-image preview

- **WHEN** `GET /jobs/:slug` is requested for an existing job
- **THEN** the HTML `<head>` contains an `og:image` whose absolute URL ends in
  `/jobs/:slug/og.png`, with `og:image:width` 1200 and `og:image:height` 630,
  and a `twitter:card` of `summary_large_image`

#### Scenario: List pages carry their own metadata

- **WHEN** a public list page (`/jobs`, `/companies`) is requested
- **THEN** its `<head>` carries a page-appropriate title, description, and
  canonical URL distinct from the job-detail metadata, and — having no preview
  image — keeps the `summary` Twitter Card with no `og:image`

### Requirement: JobPosting structured data

The job-detail page SHALL include a `JobPosting` JSON-LD `<script type="application/ld+json">`
block in its server-rendered HTML, populated from the job's public fields
(title, description, hiring organization, location/remote, posting date, and the
application URL), so the posting is eligible for Google Jobs. Company pages
SHALL include `Organization` JSON-LD.

#### Scenario: Job page emits valid JobPosting JSON-LD

- **WHEN** `GET /jobs/:slug` is requested for an existing job
- **THEN** the HTML contains one `application/ld+json` script with `@type`
  `JobPosting` whose `title`, `description`, `hiringOrganization`, and
  `datePosted` reflect the job

#### Scenario: A closed job reflects its status in structured data

- **WHEN** the job carries a `closed_at`
- **THEN** the `JobPosting` data conveys that the posting is no longer accepting
  applications rather than presenting it as open

### Requirement: robots.txt and sitemap

The site SHALL serve a real `GET /robots.txt` (a valid robots file that allows
crawling of public pages and references the sitemap) and a `GET /sitemap.xml`
that is a valid **sitemap index** (`<sitemapindex>`) referencing sub-sitemaps for
the static pages, the jobs, and the companies. Neither the index nor any
sub-sitemap SHALL return the HTML application shell, and each SHALL be valid XML.
Every sub-sitemap SHALL hold at most 50,000 URLs (the sitemap-protocol limit).
All companies (`/companies/:slug`) SHALL be enumerated across keyset-cursor-paged
company sub-sitemaps. The job sub-sitemap SHALL list the **freshest** open jobs
(`/jobs/:slug`, newest first) up to the per-file limit, rather than the full
catalogue: the jobs table is too large to enumerate per request without a
heap-bound scan that also evicts the buffer cache, so full job coverage is a
deliberate follow-up (a precomputed narrow table), not a live scan.

#### Scenario: robots.txt is a valid robots file

- **WHEN** `GET /robots.txt` is requested
- **THEN** the response is a `text/plain` robots file (not HTML) that references
  the sitemap URL

#### Scenario: sitemap.xml is a sitemap index

- **WHEN** `GET /sitemap.xml` is requested
- **THEN** the response is valid XML with a `<sitemapindex>` root listing
  sub-sitemap `<loc>` URLs for the static pages, the jobs, and every company
  chunk, and it does not contain job or company page `<url>` entries directly

#### Scenario: the job sub-sitemap lists the freshest open jobs

- **WHEN** the job sub-sitemap URL from the index is requested
- **THEN** the response is a valid `<urlset>` of at most 50,000 open-job
  (`/jobs/:slug`) URLs, newest first, each with a `<lastmod>`, and no closed job

#### Scenario: company sub-sitemaps cover all companies

- **WHEN** the index's company sub-sitemaps are followed in sequence by their
  slug cursors
- **THEN** every company appears in exactly one sub-sitemap, with no artificial cap

