## MODIFIED Requirements

### Requirement: robots.txt and sitemap

The site SHALL serve a real `GET /robots.txt` (a valid robots file that allows
crawling of public pages and references the sitemap) and a `GET /sitemap.xml`
that is a valid **sitemap index** (`<sitemapindex>`) referencing sub-sitemaps.
The sub-sitemaps together SHALL enumerate every indexable public URL — the
static pages, all open job (`/jobs/:slug`) pages, and all company
(`/companies/:slug`) pages — with no artificial cap, each sub-sitemap holding at
most 50,000 URLs (the sitemap-protocol limit). Neither the index nor any
sub-sitemap SHALL return the HTML application shell, and each SHALL be valid XML.
The job and company sub-sitemaps SHALL be paginated by a keyset cursor (the last
id / slug of the previous chunk) rather than an offset, so serving a chunk is a
bounded scan independent of catalogue size.

#### Scenario: robots.txt is a valid robots file

- **WHEN** `GET /robots.txt` is requested
- **THEN** the response is a `text/plain` robots file (not HTML) that references
  the sitemap URL

#### Scenario: sitemap.xml is a sitemap index

- **WHEN** `GET /sitemap.xml` is requested
- **THEN** the response is valid XML with a `<sitemapindex>` root listing
  sub-sitemap `<loc>` URLs for the static pages, every job chunk, and every
  company chunk, and it does not contain job or company page `<url>` entries
  directly

#### Scenario: a job sub-sitemap lists a bounded chunk of open jobs

- **WHEN** a job sub-sitemap URL from the index is requested
- **THEN** the response is a valid `<urlset>` of at most 50,000 open-job
  (`/jobs/:slug`) URLs derived from current data, each with a `<lastmod>`

#### Scenario: sub-sitemaps together cover the full open catalogue

- **WHEN** the index's job and company sub-sitemaps are followed in sequence by
  their keyset cursors
- **THEN** every open job and every company in the catalogue appears in exactly
  one sub-sitemap, with no 5,000-row (or other artificial) cap
