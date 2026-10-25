## ADDED Requirements

### Requirement: A board is one sitemap URL

The system SHALL register provider `sitemapldjson` in `sources.All` as a hydrating source that does not claim a full board listing. The board value MUST be an absolute http or https URL. Any other board value MUST fail the crawl before any page is fetched.

#### Scenario: A non-URL board is refused

- **WHEN** the board is not an absolute http(s) URL
- **THEN** the crawl returns an error and fetches nothing

#### Scenario: The provider is hydrating

- **WHEN** the provider is looked up in `sources.All`
- **THEN** it implements the hydrating fetch and does not implement `fullBoardListing`

### Requirement: Sitemap documents include gzip files and indexes

The system SHALL read a sitemap body that is XML or gzip (bytes starting with gzip magic `1f 8b`), including after a UTF-8 BOM. A sitemap index MUST be walked for its child sitemaps. A child that cannot be fetched or is not a `urlset` or `sitemapindex` MUST fail the whole board. An empty valid `urlset` MUST succeed with no jobs. The walk MUST refuse more than 1000 sitemap documents or an index nested past depth 3.

#### Scenario: A gzip child of an index yields its jobs

- **WHEN** the board sitemap is an index whose child is a gzip urlset containing a job page
- **THEN** the crawl reads that child and returns the job

#### Scenario: A failed child fails the board

- **WHEN** a child sitemap cannot be fetched
- **THEN** the crawl returns an error

#### Scenario: HTML is not a sitemap

- **WHEN** the board body is HTML or another XML root
- **THEN** the crawl returns an error

### Requirement: Page URLs are filtered and canonicalised

The system SHALL keep a sitemap loc only when it matches the board region. An empty region matches every loc. A region with no `*` is a substring. A region containing `*` is a glob in which `*` matches any characters, including slashes. Relative locs MUST be resolved against the sitemap URL. The fetch URL MUST keep the loc's query. The external id MUST be that URL with the query and fragment removed and a trailing slash trimmed, except when the path is `/`.

#### Scenario: A pattern drops non-job URLs before fetch

- **WHEN** the region is `/jobs/` and the sitemap lists a matching job URL and a product URL
- **THEN** the product URL is not fetched

#### Scenario: The external id drops the query

- **WHEN** a loc is `/jobs/engineer?utm=1` on `https://jobs.example.com`
- **THEN** the page is fetched at the loc with the query, and the external id is `https://jobs.example.com/jobs/engineer`

### Requirement: New pages are hydrated up to a cap; seen pages are not fetched

The system SHALL return an already-stored external id as a seen refresh and MUST NOT fetch its HTML. HTML fetches MUST run only for new URLs, at most 200 per board per run, after shuffling the new set. A fetch error on a new URL MUST omit that page so a later run can retry it. A page with no `JobPosting`, or with no title, or with no company, MUST be omitted and MUST NOT be recorded as unreadable.

#### Scenario: A seen URL is not fetched

- **WHEN** the seen predicate reports the external id
- **THEN** the result is a seen refresh and the page is not fetched

#### Scenario: New fetches stop at the cap

- **WHEN** the sitemap lists more new URLs than the cap
- **THEN** the crawl fetches at most that many new pages

### Requirement: A JobPosting maps onto a catalogue job

The system SHALL take the title, description, date posted, and employment type from the `JobPosting`. The description MUST be HTML-unescaped and then sanitized. The company MUST be the hiring organization's name when present, otherwise the board company. `TELECOMMUTE` MUST set the remote flag and the remote work mode. Location and country codes MUST come from `jobLocation`, and when that yields no location, from applicant location requirements. A malformed `jobLocation` or hiring organization MUST NOT drop the rest of the posting.

#### Scenario: A matching page becomes a job

- **WHEN** a matching page carries a `JobPosting` with a title, a hiring organization, an HTML description, `TELECOMMUTE`, a full-time employment type, a posted date, and a US address
- **THEN** the job uses the hiring organization's name, a sanitized description, remote work mode, full-time employment, that posted year, the address, and country `us`

#### Scenario: The board company fills a missing hiring organization

- **WHEN** the posting has a title and no hiring organization name
- **THEN** the job company is the board company

### Requirement: The provider has a display label

The system SHALL label provider `sitemapldjson` as `Sitemap JSON-LD` in the source facet, and the generated source values MUST include `sitemapldjson`.

#### Scenario: The label is not the title-cased slug

- **WHEN** the source facet is rendered for `sitemapldjson`
- **THEN** the label is `Sitemap JSON-LD`
