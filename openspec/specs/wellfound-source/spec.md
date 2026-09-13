# wellfound-source Specification

## Purpose

Crawl Wellfound (formerly AngelList Talent) into the catalogue by reading its own
role-taxonomy search pages through the hosted Firecrawl tier and parsing the structured job
payload each page already embeds, rather than scanning rendered DOM or scraping the general
marketplace firehose.

## Requirements

### Requirement: Wellfound role-scoped search crawl

The system SHALL provide a `wellfound` source adapter that crawls one of Wellfound's own
role-taxonomy search pages (e.g. `/role/r/software-engineer`) per configured board, rather
than the whole general-marketplace listing. Each board is a role slice, not a per-company
tenant.

#### Scenario: A role slice yields its postings

- **WHEN** the adapter crawls a configured role-slice board
- **THEN** it returns one `Job` per job listing present in that role's search results,
  populated from the embedded payload (title, HTML description with the stated compensation
  folded into it as free text — Wellfound states a display range rather than a structured
  amount and currency, so it is carried the same way SEEK's `salaryLabel` and Workstream's
  pay line already are in this catalogue, never guessed into a structured salary field —
  remote flag, free-text locations, hiring company)

#### Scenario: The general marketplace firehose is never crawled directly

- **WHEN** a board is configured
- **THEN** its URL is always one of Wellfound's own role-taxonomy search pages, never the
  unscoped `/jobs` or `/remote` listing — so a crawl never pays for the marketplace's
  majority-non-technical traffic

### Requirement: Self-contained aggregator company identity

The adapter MUST read each posting's employer from the listing's own hiring-company data,
because Wellfound is a multi-company aggregator where each posting names its own startup.
The adapter is registered as an aggregator-marker source so its jobs are stored under their
own company identity.

#### Scenario: Company comes from the posting's own hiring startup

- **WHEN** a listing entry names a hiring startup
- **THEN** that startup's name is the job's company

#### Scenario: Posting without a resolvable employer is dropped

- **WHEN** a listing entry has no resolvable hiring-startup name
- **THEN** the adapter omits that posting rather than persisting a company-less job

### Requirement: Stable dedup identity

The adapter SHALL derive each job's `ExternalID` from Wellfound's own numeric job listing
id, so re-crawling the same or an overlapping role slice dedups to the same catalogue row.

#### Scenario: Re-crawl dedups

- **WHEN** the same posting is seen again, whether on a later crawl of the same board or on
  a different role-slice board that also lists it
- **THEN** it maps to the same `ExternalID` and updates the existing row rather than
  creating a duplicate; a listing entry with no extractable id is dropped rather than
  persisted with an empty key

### Requirement: Hosted-tier transport past the Cloudflare challenge

The adapter's pages MUST be fetched through the hosted Firecrawl tier, because Wellfound's
search and listing pages sit behind a Cloudflare JavaScript challenge that neither a direct
request nor this repository's own proxied headless-browser tier can pass. The adapter is
registered in the crawl registry unconditionally (so classification and cross-source dedup
always know about it, the same reasoning already applied to `bayt`/`gulftalent`/`jobleads`);
without a Firecrawl credential configured, it is still present but every crawl attempt fails
loudly on the Cloudflare challenge response, exactly as `bayt`/`gulftalent` already do without
one — never a silent, provider-absent no-op.

#### Scenario: Requests are served, not challenged

- **WHEN** the adapter fetches a role-slice search page and a Firecrawl credential is
  configured
- **THEN** it does so through the hosted tier so the response is the real page rather than
  the Cloudflare challenge page

#### Scenario: No credential means every crawl attempt fails loudly

- **WHEN** no Firecrawl credential is configured
- **THEN** `wellfound` remains present in the crawl registry, and a crawl attempt fails with
  the Cloudflare challenge response rather than the provider being silently absent

### Requirement: The listing carries the full posting body

The adapter SHALL read each posting's complete HTML description from the search page's own
embedded payload. It MUST NOT issue a second, per-posting request to hydrate the body,
because the listing already carries it in full.

#### Scenario: A posting's body needs no extra fetch

- **WHEN** the adapter parses a role-slice search page
- **THEN** every listed posting's job body comes from that same fetched page, and the crawl
  issues no additional request per posting

### Requirement: Pagination to an authoritative total

The adapter SHALL page through a role slice's results using the total the payload itself
states, rather than treating an empty page as the only proof pagination has ended.

#### Scenario: Pagination stops at the stated total

- **WHEN** a role slice's payload states how many results or pages exist
- **THEN** the adapter pages until that stated total is reached, rather than relying solely
  on encountering an empty page

### Requirement: Resilient parsing isolates per-posting failures but fails loudly on the page

The adapter SHALL drop an individual listing entry that carries no usable job payload (a
malformed or unexpectedly-shaped entry must not abort an otherwise healthy crawl), while a
page that cannot be fetched or whose embedded payload cannot be parsed at all SHALL error
that page's contribution to the crawl rather than silently returning zero postings for it.

#### Scenario: One malformed entry drops only that posting

- **WHEN** one listing entry on an otherwise-parseable page carries no usable job payload
- **THEN** the adapter omits that posting and keeps the page's other postings

#### Scenario: An unparseable page is a loud failure

- **WHEN** a role-slice page cannot be fetched, or its embedded payload cannot be parsed at
  all
- **THEN** the adapter reports that page as failed rather than as a page with zero postings
