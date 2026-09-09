## MODIFIED Requirements

### Requirement: robots.txt and sitemap

The site SHALL serve a real `GET /robots.txt` (a valid robots file that allows
crawling of public pages and references the sitemap) and a `GET /sitemap.xml`
that is a valid **sitemap index** (`<sitemapindex>`) referencing sub-sitemaps for
the static pages, the jobs, and the companies. Neither the index nor any
sub-sitemap SHALL return the HTML application shell, and each SHALL be valid XML.
Every sub-sitemap SHALL hold at most 50,000 URLs (the sitemap-protocol limit).
Every company with at least one FINDABLE role (`/companies/:slug`) SHALL be
enumerated across keyset-cursor-paged company sub-sitemaps, and a company with
none SHALL NOT appear. "Findable" is the job search index's own scope — open,
non-duplicate, non-private, categorized, with a body — because the company page's
job list is served by that index, so a company outside it has a page that renders
its name and "0 open jobs". The scope is carried by `companies.job_count`, which
`RefreshCompanyFacets` computes under exactly those predicates.

The job sub-sitemaps SHALL enumerate the **TECH** postings of that same index
(`/jobs/:slug`), and no others. A posting SHALL be named when `is_tech` is `tech`
and its reality class is not `likely-evergreen`; a `non_tech` posting, a posting
whose `is_tech` no dictionary and no enrichment resolved, and one carrying the
perpetual-listing verdict SHALL NOT be named.

This narrows the sitemap ONLY. A posting outside this scope keeps its page, its
canonical URL, its place in `/jobs` and its search result — the sitemap is a claim
about what is worth crawling, and this site's claim is tech jobs. The unknown
bucket is excluded rather than admitted because the sitemap asserts; a posting
nothing could classify is not asserted to be a tech job. `likely-evergreen` is
excluded because the site publishes a ghost verdict on those same pages, and
inviting a crawler to one contradicts it.

The population the sitemap index tiles into chunks SHALL be the same filtered
population the chunks serve, so that the index can never name a chunk the job
reader would not fill.

#### Scenario: robots.txt is a valid robots file

- **WHEN** `GET /robots.txt` is requested
- **THEN** the response is a `text/plain` robots file (not HTML) that references
  the sitemap URL

#### Scenario: sitemap.xml is a sitemap index

- **WHEN** `GET /sitemap.xml` is requested
- **THEN** the response is valid XML with a `<sitemapindex>` root listing
  sub-sitemap `<loc>` URLs for the static pages, the jobs, and every company
  chunk, and it does not contain job or company page `<url>` entries directly

#### Scenario: the job sub-sitemap lists open tech jobs

- **WHEN** a job sub-sitemap URL from the index is requested
- **THEN** the response is a valid `<urlset>` of at most 50,000 open-job
  (`/jobs/:slug`) URLs, each with a `<lastmod>`, and no closed job

#### Scenario: a non-tech posting is not named in the sitemap

- **WHEN** an open, findable posting is classified `is_tech = non_tech`
- **THEN** no job sub-sitemap lists its `/jobs/:slug` URL, and the page itself is
  still served, still canonical and still returned by `/jobs`

#### Scenario: an unclassified posting is not named in the sitemap

- **WHEN** an open, findable posting has no resolved `is_tech` value
- **THEN** no job sub-sitemap lists its `/jobs/:slug` URL

#### Scenario: a posting the site calls a perpetual listing is not named

- **WHEN** an open tech posting's reality class is `likely-evergreen`
- **THEN** no job sub-sitemap lists its `/jobs/:slug` URL

#### Scenario: the index and its chunks count the same population

- **WHEN** `/sitemap.xml` names N job sub-sitemaps of chunk size C
- **THEN** the number of tech postings the job reader would serve is greater than
  `C * (N - 1)` and at most `C * N`, so no named chunk is empty

#### Scenario: a retired chunk offset is empty, not an error

- **WHEN** a crawler holding an older sitemap index requests a job sub-sitemap at
  an offset past the end of the filtered population
- **THEN** the response is a valid, empty `<urlset>` — never a 404 and never an
  error

#### Scenario: company sub-sitemaps cover every company worth crawling

- **WHEN** the index's company sub-sitemaps are followed in sequence by their
  slug cursors
- **THEN** every company holding at least one findable role appears in exactly one
  sub-sitemap, with no artificial cap

#### Scenario: a company with nothing findable is not in the sitemap

- **WHEN** a company's only open postings are private, body-less, or of a category
  no dictionary resolved — so its page's job list renders empty
- **THEN** no company sub-sitemap lists its `/companies/:slug` URL
