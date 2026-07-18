## ADDED Requirements

### Requirement: Jobylon maps its global jobs sitemap to jobs

The system SHALL provide a `jobylon` source adapter that enumerates Jobylon's global jobs sitemap
— resolving the `sitemap-jobs` sub-sitemap of `https://emp.jobylon.com/sitemap.xml` — into
`https://emp.jobylon.com/jobs/<id>-<slug>/` job URLs, and maps each job's server-rendered
schema.org `JobPosting` ld+json to a normalized `Job`. The `Job` SHALL carry the numeric `<id>`
from the job URL as its `ExternalID`, the job URL as its canonical `URL`, the HTML-unescaped ld+json
`title` as its title, `hiringOrganization.name` as its company, the `jobLocation` places joined
into a free-text location as its location, the sanitized `description` HTML as its body, remote
inferred from the location text, and `datePosted` as `PostedAt`.

The adapter is **boardless** (Jobylon's sitemap is one global feed with no per-tenant board) and an
**aggregator** (each posting's company comes from its own `hiringOrganization`), so it stays in the
source facet. The configured board file entry carries no `board` value.

#### Scenario: A job page maps to a job

- **WHEN** the adapter reads the job URL `https://emp.jobylon.com/jobs/369523-acme-engineer/` whose
  page carries a `JobPosting` ld+json with `title` `Engineer &amp; Lead`, `hiringOrganization.name`
  `Acme`, a `jobLocation`, a `description` body, and a `datePosted`
- **THEN** it yields one `Job` with `ExternalID` `369523`, that URL as the canonical URL, title
  `Engineer & Lead`, company `Acme`, the joined location, the sanitized description, and `PostedAt`
  parsed from `datePosted`

#### Scenario: An inconsistently-typed employmentType does not drop the posting

- **WHEN** a job page's `JobPosting` ld+json emits `employmentType` as an array (e.g.
  `["CONTRACTOR"]`) rather than a string
- **THEN** the posting still maps to a `Job` (the adapter does not model `employmentType`, so its
  type variance never fails the posting's ld+json decode)

### Requirement: Jobylon drops unusable postings

The adapter SHALL drop any job URL that yields no numeric id, any page that carries no `JobPosting`
ld+json, and any posting whose `title` or `hiringOrganization.name` resolves empty — an empty dedup
key or empty company would break the posting's public slug. A single dropped posting SHALL NOT abort
the crawl.

#### Scenario: A posting with an empty company is dropped

- **WHEN** a job page's `JobPosting` ld+json resolves to an empty `hiringOrganization.name`
- **THEN** the adapter yields no `Job` for that posting and continues mapping the rest

### Requirement: Jobylon hydrates only new postings

The adapter SHALL implement incremental hydration: given the set of already-ingested posting ids,
it SHALL issue the per-job ld+json detail fetch only for postings not already ingested, and SHALL
emit each already-ingested posting as a liveness refresh — a `Job` carrying only its identity
(`ExternalID`, `URL`) with `SeenRefresh` set — without a detail request or content rewrite. When no
seen set is available, the adapter SHALL fall back to detail-fetching every enumerated posting.

#### Scenario: A seen posting is refreshed, not re-fetched

- **WHEN** the adapter enumerates a job URL whose id is already ingested
- **THEN** it yields a `Job` with that id and URL, `SeenRefresh` set, and no description, and issues
  no detail request for it

#### Scenario: An unseen posting is hydrated

- **WHEN** the adapter enumerates a job URL whose id is not yet ingested
- **THEN** it fetches that job page and yields the fully-mapped `Job` from its `JobPosting` ld+json
