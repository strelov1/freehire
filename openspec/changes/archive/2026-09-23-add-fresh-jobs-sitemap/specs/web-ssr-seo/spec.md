# web-ssr-seo — delta

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

The jobs are carried by **two kinds of sub-sitemap, and the difference between them
is the ordering guarantee**. The paged job sub-sitemaps SHALL enumerate the whole
findable job catalogue across offset-addressed chunks, in **unspecified order**: they
are drawn from the search index's document store, which can address any offset
directly but cannot sort, and that is what let the sitemap stop asking Postgres to
number millions of rows on every render. Separately, a **freshest-first job
sub-sitemap** SHALL list open jobs ordered by creation time, newest first, bounded to
a fixed recent window, and SHALL be listed in the index **ahead of** the paged
chunks. Both draw on the same findable scope, so a job appears in both, which the
sitemap protocol permits.

The freshest-first file exists because a crawler's budget is finite and arbitrary
order spends it on postings the crawler already holds. Measured on prod over
2026-09-09..19: of 153,030 newly created findable jobs, OAI-SearchBot first fetched
27,447 — 17.9% — while spending ~54k requests a day against a flow of ~11k eligible
new postings a day. The ordering is the whole point of the file; a test that asserts
only its shape would pass against the defect this requirement exists to prevent.

An offset past the end of either kind SHALL return an empty `<urlset>`, never an
error: a crawler holding a stale sitemap index must not be answered with a failure.

`<lastmod>` is the job's last-modified time and SHALL be emitted whenever the index
holds one. It is NOT the value either job sub-sitemap is ordered by — the freshest-first
file is ordered by CREATION time, which no `<lastmod>` reports — so the ordering
guarantee above cannot be checked by reading dates out of the XML, and a check that
tried would pass against a wrongly-ordered file. A document indexed before the
attribute joined the shape carries no `<lastmod>` and SHALL still be listed: the tag is
optional in the protocol, and dropping the URL would cost a crawlable page to save an
optional field.

#### Scenario: robots.txt is a valid robots file

- **WHEN** `GET /robots.txt` is requested
- **THEN** the response is a `text/plain` robots file (not HTML) that references
  the sitemap URL

#### Scenario: sitemap.xml is a sitemap index

- **WHEN** `GET /sitemap.xml` is requested
- **THEN** the response is valid XML with a `<sitemapindex>` root listing
  sub-sitemap `<loc>` URLs for the static pages, the freshest-first jobs, the paged
  jobs, and every company chunk, and it does not contain job or company page `<url>`
  entries directly

#### Scenario: the job sub-sitemap lists the freshest open jobs

- **WHEN** the freshest-first job sub-sitemap is requested at its first offset
- **THEN** the response is a valid `<urlset>` of open-job (`/jobs/:slug`) URLs whose
  entries are in non-increasing order of the job's CREATION time — the newest job in
  the catalogue first — each carrying a `<lastmod>` when the index holds one

#### Scenario: the freshest-first sub-sitemap is listed before the paged chunks

- **WHEN** `GET /sitemap.xml` is requested
- **THEN** every freshest-first job sub-sitemap `<loc>` appears in the index before
  the first paged job sub-sitemap `<loc>`

#### Scenario: the freshest-first window is bounded

- **WHEN** the freshest-first job sub-sitemap is requested at an offset past the end
  of its window
- **THEN** the response is a valid, empty `<urlset>` and not an error

#### Scenario: the paged job sub-sitemaps cover the whole findable catalogue

- **WHEN** the index's paged job sub-sitemaps are followed in sequence by their
  offsets
- **THEN** every findable open job appears, each entry is a `/jobs/:slug` URL carrying
  a `<lastmod>` when the index holds one, no closed job appears, and no sub-sitemap
  exceeds the per-file limit

#### Scenario: company sub-sitemaps cover every company worth crawling

- **WHEN** the index's company sub-sitemaps are followed in sequence by their
  slug cursors
- **THEN** every company holding at least one findable role appears in exactly one
  sub-sitemap, with no artificial cap

#### Scenario: a company with nothing findable is not in the sitemap

- **WHEN** a company's only open postings are private, body-less, or of a category
  no dictionary resolved — so its page's job list renders empty
- **THEN** no company sub-sitemap lists its `/companies/:slug` URL
