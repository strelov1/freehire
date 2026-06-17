# source-ingest Specification

## Purpose
TBD - created by archiving change add-source-ingest-pipeline. Update Purpose after archive.
## Requirements
### Requirement: Jobs enter the catalogue through modular source adapters

The system SHALL ingest jobs through `Source` adapters, each implementing exactly
one job-source platform. An adapter SHALL expose its provider key and SHALL fetch
all current postings for one configured board. Adapters SHALL be assembled into a
provider-keyed registry by a single explicit constructor, so that adding a platform
is a new adapter file plus one registration line and requires no change to the
pipeline.

An adapter SHALL prefer the platform's list endpoint, but MAY perform per-posting
detail requests and paginate the list when the list endpoint does not carry the
full posting (for example, when it omits the description). Such detail requests
SHALL be bounded so a single board cannot issue unbounded concurrent requests.

#### Scenario: Adapter is dispatched by provider key

- **WHEN** a configured board names provider `greenhouse`
- **THEN** the pipeline dispatches that board to the registered `greenhouse` adapter
  and uses the postings it returns

#### Scenario: Adapter maps a posting to the normalized job shape

- **WHEN** an adapter fetches a board and the platform returns a posting
- **THEN** the adapter yields a job carrying at least title, url, location, remote
  flag, description, and the platform's native posting id

#### Scenario: Adapter fetches detail when the list lacks the description

- **WHEN** a platform's list endpoint returns postings without a description (e.g. SmartRecruiters)
- **THEN** the adapter paginates the list and fetches each posting's detail to obtain
  the description, still yielding the normalized job shape

### Requirement: Boards to crawl are configured in a file

The system SHALL read the set of boards to crawl from configuration at ingest startup,
each entry naming a `company`, a `provider`, and — for a provider that has a board/tenant
concept — a `board`. An entry's `provider` MAY be set on the entry itself; when it is
absent the provider SHALL default to the configuration file's base name. Because the
provider is resolved per entry, a single file MAY list entries belonging to several
providers (e.g. a shared `custom.yml` of single-source configs). A configured entry whose
resolved `provider` has no registered adapter SHALL cause the ingest command to fail fast
at startup rather than silently skip the board. An entry whose `board` is empty SHALL fail
fast at startup **unless** its provider is a single-company provider that declares it needs
no board (a `boardless` provider), in which case the empty `board` SHALL be accepted.

#### Scenario: Configured boards are loaded

- **WHEN** the configuration lists a board with `company`, `provider`, and `board`
- **THEN** the ingest run includes that board, dispatched to the named provider

#### Scenario: Provider defaults to the file name when the entry omits it

- **WHEN** an entry does not set `provider` and the configuration file's base name is a
  registered provider (e.g. `greenhouse.yml`)
- **THEN** the entry is dispatched to the file-name provider, as before

#### Scenario: A single file lists entries for multiple providers

- **WHEN** one configuration file lists entries that each name their own `provider`
  (e.g. `custom.yml` with `vk` and `ozon` entries)
- **THEN** each entry is dispatched to its own named provider within the same run

#### Scenario: Unknown provider fails fast

- **WHEN** the configuration has an entry whose resolved `provider` has no registered
  adapter (either named explicitly or defaulted from a file name that is not a provider)
- **THEN** the ingest command exits with an error naming the unknown provider and
  ingests nothing

#### Scenario: Empty board fails fast for a board-based provider

- **WHEN** the configuration has an entry with an empty `board` whose provider has a
  board concept (e.g. `greenhouse`)
- **THEN** the ingest command exits with an error naming the company with the empty board

#### Scenario: Empty board is accepted for a boardless provider

- **WHEN** the configuration has an entry with an empty `board` whose provider is a
  single-company `boardless` provider (e.g. `ozon`)
- **THEN** validation accepts the entry and the ingest run includes that board

### Requirement: Ingest writes jobs through a normalized, namespaced write path

The pipeline SHALL persist each fetched posting via the existing job write path. It
SHALL set `source` to the board's provider, derive `company_slug` from the company
name using the existing slug normalization, and set `external_id` to the namespaced
form `"<board>:<native-posting-id>"`. Namespacing SHALL guarantee that two companies
on the same platform sharing a native posting id do not collide on the dedup key
`UNIQUE (source, external_id)`.

#### Scenario: External id is namespaced by board

- **WHEN** a posting with native id `42` is ingested for board `cohere` on provider
  `greenhouse`
- **THEN** the stored job has `source = "greenhouse"` and
  `external_id = "cohere:42"`

#### Scenario: Same native id on different boards does not collide

- **WHEN** two boards on the same provider each return a posting with native id `42`
- **THEN** both jobs are stored as distinct rows, differing in `external_id`

#### Scenario: Re-ingest of the same posting updates in place

- **WHEN** a posting already in the catalogue is ingested again with an edited title
- **THEN** the existing row is updated rather than duplicated, keyed on
  `(source, external_id)`

### Requirement: Re-ingest preserves existing enrichment

The ingest write path SHALL NOT write or overwrite a job's enrichment payload or
provenance. Re-ingesting a job that has already been enriched SHALL leave its
`enrichment`, `enriched_at`, and `enrichment_version` unchanged, so that source
re-ingestion never wipes enrichment produced by the enrichment worker.

#### Scenario: Enrichment survives a re-ingest

- **WHEN** an already-enriched job is re-ingested with updated source fields
- **THEN** the job's source fields update but its `enrichment`, `enriched_at`, and
  `enrichment_version` are unchanged

### Requirement: A source failure is isolated from the rest of the run

The ingest run SHALL process boards with bounded concurrency and SHALL isolate
failures: a board whose fetch or parse errors SHALL be recorded and skipped without
aborting the run or preventing the remaining boards from being ingested.

#### Scenario: One failing board does not abort the run

- **WHEN** one configured board's fetch errors and the others succeed
- **THEN** the failing board is recorded as failed and every other board is still
  ingested

### Requirement: A standalone command runs ingest on a schedule

The system SHALL provide a standalone `cmd/ingest` binary that loads configuration,
runs every configured board once with bounded concurrency, reports how many jobs
were ingested and how many sources failed, and exits — suitable for scheduled
execution.

#### Scenario: Ingest command runs a bounded batch and exits

- **WHEN** the ingest command is run
- **THEN** it processes every configured board once and exits after reporting the
  ingested and failed counts

### Requirement: Adapter descriptions are sanitized HTML

Each adapter SHALL yield the job `description` as sanitized HTML assembled from the platform's authoritative HTML field(s), so the stored value is safe to render directly in a browser without further escaping. An adapter SHALL NOT yield raw or entity-encoded source markup, and SHALL NOT rely on a platform plain-text field that the platform may leave empty or partial. Sanitization SHALL run server-side before the description is persisted, stripping scripts, event handlers, and other active content while preserving structural formatting (headings, paragraphs, lists, emphasis, links).

#### Scenario: Greenhouse entity-encoded HTML is decoded and sanitized

- **WHEN** a greenhouse posting returns `content` as entity-encoded HTML (e.g. `&lt;h2&gt;Role&lt;/h2&gt;`)
- **THEN** the adapter yields a description whose entities are decoded to real markup and then sanitized, so the stored value contains `<h2>Role</h2>` rather than the encoded entities

#### Scenario: Lever multi-field body is assembled, not truncated

- **WHEN** a lever posting splits its body across `description`, one or more `lists` (each with a heading `text` and HTML `content`), and `additional`
- **THEN** the adapter yields a description that concatenates the opening `description`, each list as a heading followed by its content, and the closing `additional` — even when `descriptionPlain` is empty

#### Scenario: Ashby uses the HTML field

- **WHEN** an ashby posting exposes both `descriptionHtml` and `descriptionPlain`
- **THEN** the adapter yields the sanitized `descriptionHtml`, preserving its formatting

#### Scenario: Active content is stripped

- **WHEN** a source posting's HTML contains a `<script>` tag or an inline event handler (e.g. `onclick`)
- **THEN** the persisted description contains neither the script nor the event handler, while its safe structural markup is retained

### Requirement: Workable, Recruitee, and SmartRecruiters are registered providers

The system SHALL register `workable`, `recruitee`, and `smartrecruiters` adapters so
boards on these platforms can be listed in a `sources/` board file. Each adapter SHALL yield the
job description as sanitized HTML assembled from the platform's authoritative HTML
field(s), consistent with the existing adapters.

#### Scenario: Workable board is crawled

- **WHEN** a `sources/` board file lists a board with provider `workable`
- **THEN** the adapter fetches that account's jobs in one request and yields each with a
  sanitized HTML description from the inline `description` field

#### Scenario: Recruitee description and requirements are combined

- **WHEN** a recruitee offer carries separate `description` and `requirements` HTML
- **THEN** the adapter yields one sanitized HTML description combining both

#### Scenario: SmartRecruiters posting gains its description from detail

- **WHEN** a smartrecruiters board is crawled
- **THEN** the adapter paginates the postings list and, per posting, fetches its detail
  and yields a sanitized HTML description assembled from `jobAd.sections`

### Requirement: Personio, Pinpoint, Rippling, and BambooHR are registered providers

The system SHALL register `personio`, `pinpoint`, `rippling`, and `bamboohr` adapters so
boards on these platforms can be listed in a `sources/` board file. Each adapter SHALL yield the
normalized job shape (at least title, url, location, remote flag, description, and the
platform's native posting id) with the `description` as sanitized HTML assembled from the
platform's authoritative HTML field(s), consistent with the existing adapters. An adapter
whose list endpoint omits the description SHALL fetch each posting's detail with bounded
concurrency rather than yield an empty body, and a single failed detail SHALL drop only
that posting rather than abort the board.

#### Scenario: Personio XML feed is crawled in one request

- **WHEN** a `sources/` board file lists a board with provider `personio`
- **THEN** the adapter fetches the board's `…jobs.personio.com/xml` feed in one request and
  yields each `<position>` with a sanitized HTML description assembled from its inline
  `jobDescriptions`, and a job URL built from the board and position id

#### Scenario: Pinpoint board carries the body inline

- **WHEN** a `pinpoint` board is crawled
- **THEN** the adapter fetches the board's `…/postings.json` in one request and yields each
  posting with a sanitized HTML description assembled from its inline body sections

#### Scenario: Rippling posting gains its description from detail

- **WHEN** a `rippling` board is crawled
- **THEN** the adapter fetches the board's job list and, per posting, fetches its detail with
  bounded concurrency to obtain the role description (excluding the company boilerplate),
  still yielding the normalized job shape

#### Scenario: BambooHR posting gains its description from detail

- **WHEN** a `bamboohr` board is crawled
- **THEN** the adapter fetches `…/careers/list` and, per posting, fetches `…/careers/{id}/detail`
  with bounded concurrency to obtain the description, still yielding the normalized job shape

#### Scenario: A failed detail request drops only that posting

- **WHEN** a detail-fetching provider's board lists several postings and one posting's detail
  request fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: A board with no open postings yields no jobs without error

- **WHEN** any of these providers' feeds returns an empty posting list for a configured board
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: Gem is a registered provider

The system SHALL register a `gem` adapter so boards on the Gem platform (jobs.gem.com)
can be listed in a `sources/` board file. The adapter SHALL treat the configured `board` value as
the Gem **vanity path** and pass it verbatim as the GraphQL `boardId`. It SHALL fetch
postings from the public GraphQL endpoint `POST https://jobs.gem.com/api/public/graphql`
using the `JobBoardList(boardId)` operation, and — because that list carries no
description — SHALL fetch each posting's body via the `ExternalJobPostingQuery(boardId,
extId)` operation with bounded concurrency. A single failed detail request SHALL drop
only that posting rather than abort the board. The adapter SHALL yield the normalized job
shape (at least title, url, location, remote flag, description, and the platform's native
posting id), with the `description` as sanitized HTML assembled from the posting's
`descriptionHtml` field, consistent with the existing adapters.

#### Scenario: Gem board is crawled list-then-detail

- **WHEN** a `sources/` board file lists a board with provider `gem` and a vanity-path `board`
- **THEN** the adapter calls `JobBoardList` with that vanity path as `boardId`, and per
  returned posting calls `ExternalJobPostingQuery` with the posting's `extId` to obtain a
  sanitized HTML description, yielding each as the normalized job shape with
  `external_id` set to the posting's `extId`

#### Scenario: Remote is taken from Gem's structured flags

- **WHEN** a Gem posting reports a location with `isRemote: true` or a `job.locationType`
  of `REMOTE`
- **THEN** the adapter yields the job with its remote flag set, without relying on a
  free-text location match

#### Scenario: Posting is dated from its first-published timestamp

- **WHEN** a Gem posting carries a `firstPublishedTsSec` Unix-seconds timestamp
- **THEN** the adapter yields the job with `posted_at` derived from that timestamp, and
  yields a nil `posted_at` when the timestamp is absent or zero

#### Scenario: A failed detail request drops only that posting

- **WHEN** a Gem board lists several postings and one posting's
  `ExternalJobPostingQuery` request fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: A board with no open postings yields no jobs without error

- **WHEN** a Gem board returns an empty `jobPostings` list
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply
  skipped

### Requirement: successfactors is a registered provider

The system SHALL register a `successfactors` adapter so SAP SuccessFactors career sites
can be listed in a `sources/` board file. The adapter SHALL treat the configured `board` value as the
career-site host and enumerate jobs from that site's `GET https://<board>/job_sitemap.xml`,
taking each `<url>`'s `<loc>` as the job page URL (with the job's native id as the numeric
segment of that path) and its `<lastmod>` as the posting date. Because the sitemap carries
no description, the adapter SHALL fetch each job page and extract the title and description
from the page's schema.org JobPosting microdata (`itemprop="title"` and
`itemprop="description"`), with bounded concurrency; a single failed page fetch SHALL drop
only that posting rather than abort the board. The adapter SHALL yield the normalized job
shape (at least title, url, remote flag, description, and the platform's native posting id),
with the `description` as sanitized HTML, consistent with the existing adapters. The job
`location` MAY be empty, since SuccessFactors does not expose it in the microdata and
enrichment derives it from the description.

#### Scenario: SuccessFactors board is enumerated from its sitemap

- **WHEN** a `sources/` board file lists a board with provider `successfactors` and a career-site host
- **THEN** the adapter fetches `https://<host>/job_sitemap.xml`, and per `<loc>` fetches the
  job page, yielding each as the normalized job shape with `external_id` set to the numeric
  id from the job URL and `posted_at` derived from the entry's `<lastmod>`

#### Scenario: Title and description come from JobPosting microdata

- **WHEN** a SuccessFactors job page is fetched
- **THEN** the adapter yields the job's title from `itemprop="title"` and a sanitized HTML
  description from the inner markup of `itemprop="description"`

#### Scenario: A failed job-page fetch drops only that posting

- **WHEN** a board's sitemap lists several jobs and one job page's fetch fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: An empty sitemap yields no jobs without error

- **WHEN** a board's `job_sitemap.xml` lists no job URLs
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: teamtailor is a registered provider

The system SHALL register a `teamtailor` adapter so Teamtailor career sites can be listed in
a `sources/` board file. The adapter SHALL treat the configured `board` value as the career-site host and
enumerate jobs from that site's `GET https://<board>/jobs` listing HTML, taking each job-card
anchor to a `/jobs/<id>-<slug>` path as a job (with the job's native id as the leading numeric
segment of that path). The adapter SHALL paginate the listing via `?page=N`, requesting
successive pages until a page yields no job links, so boards larger than one listing page are
fully enumerated. Because the listing carries no description, the adapter SHALL fetch each job
page and extract the posting from the page's schema.org JobPosting `application/ld+json` block,
with bounded concurrency; a single failed page fetch SHALL drop only that posting rather than
abort the board. The adapter SHALL yield the normalized job shape (at least title, url, remote
flag, description, and the platform's native posting id), with the `description` as sanitized
HTML (HTML-unescaped before sanitizing, since the `ld+json` description is double-encoded),
consistent with the existing adapters. The job `location` SHALL come from the JobPosting's
`jobLocation` address (locality and country) when present and MAY be empty otherwise.

#### Scenario: Teamtailor board is enumerated from its listing page

- **WHEN** a `sources/` board file lists a board with provider `teamtailor` and a career-site host
- **THEN** the adapter fetches `https://<host>/jobs`, and per job-card anchor fetches the job
  page, yielding each as the normalized job shape with `external_id` set to the numeric id from
  the job URL and `url` set to that job URL

#### Scenario: Title, description, and date come from the JobPosting ld+json

- **WHEN** a Teamtailor job page is fetched
- **THEN** the adapter yields the job's title from the JobPosting `title`, a sanitized HTML
  description from its HTML-unescaped `description`, `posted_at` parsed from `datePosted`, and
  `location` from the `jobLocation` address when present

#### Scenario: A multi-page board is fully enumerated

- **WHEN** a board's listing spans more than one page
- **THEN** the adapter requests successive `?page=N` pages until one yields no job links, and
  yields the jobs from every page

#### Scenario: A failed job-page fetch drops only that posting

- **WHEN** a board's listing yields several jobs and one job page's fetch fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: An empty listing yields no jobs without error

- **WHEN** a board's `/jobs` listing yields no job links
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: join is a registered provider

The system SHALL register a `join` adapter so Join.com career pages can be listed in
a `sources/` board file. The adapter SHALL treat the configured `board` value as the numeric Join
company id and enumerate jobs from the public JSON API
`GET https://join.com/api/public/companies/<board>/jobs?page=N&pageSize=<size>`, requesting
successive pages until all pages reported by the response's pagination have been read, so a
company with more jobs than one page is fully enumerated. Because the list response carries no
description, the adapter SHALL fetch each job's detail from
`GET https://join.com/api/public/jobs/<id>` with bounded concurrency; a single failed detail
request SHALL drop only that posting rather than abort the board. The adapter SHALL yield the
normalized job shape (at least title, url, remote flag, description, and the platform's native
posting id), with the `description` rendered from the job's Markdown body to sanitized HTML.
The job `location` SHALL come from the listing item's city (locality and country) when present
and MAY be empty otherwise.

#### Scenario: Join board is enumerated from the public list API

- **WHEN** a `sources/` board file lists a board with provider `join` and a numeric company id
- **THEN** the adapter requests `…/companies/<id>/jobs` and, per listed job, fetches
  `…/jobs/<job-id>`, yielding each as the normalized job shape with `external_id` set to the
  API's numeric job id and `url` set to `https://join.com/companies/<company-slug>/<idParam>`

#### Scenario: All pages of a multi-page board are enumerated

- **WHEN** a board's job count spans more than one API page
- **THEN** the adapter requests each page up to the pagination's reported page count and yields
  the jobs from every page

#### Scenario: Description is rendered from Markdown to sanitized HTML

- **WHEN** a Join job's detail is fetched and its body is Markdown
- **THEN** the adapter yields a `description` that is the Markdown rendered to HTML and then
  sanitized (active content stripped, structure such as lists and paragraphs kept)

#### Scenario: A failed detail request drops only that posting

- **WHEN** a board lists several jobs and one job's detail request fails
- **THEN** the failed posting is skipped and every other posting is still yielded, without
  aborting the board

#### Scenario: An empty board yields no jobs without error

- **WHEN** a board's list API reports zero jobs
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: Ingest persists job geography and work mode

The ingest write path SHALL parse each posting's `location` string into
`countries`/`regions`/`work_mode` (via the job-geography parser) and persist them
on the job row. These columns SHALL be written on insert and refreshed on
re-ingest, like the other raw source fields and unlike the enrichment payload
(which ingest never writes). A posting whose location yields no geography SHALL
store empty arrays.

For `work_mode`, when the adapter exposes a STRUCTURED work mode (a workplace-type
enum or an explicit remote flag from the ATS API) it SHALL take precedence over
the parser's free-text heuristic; the parser fills `work_mode` only when the
adapter has no structured signal.

#### Scenario: A new posting stores its parsed geography

- **WHEN** a posting with location `Remote - Germany` is ingested
- **THEN** the stored job has `countries=[de]` and `regions` including `eu`

#### Scenario: Re-ingest refreshes geography from the updated location

- **WHEN** an already-ingested posting is re-ingested with its location changed
  from `Remote - UK` to `Remote - USA`
- **THEN** the job's `countries` updates to `[us]` and its `regions` update
  accordingly

#### Scenario: A location with no geography stores empty arrays

- **WHEN** a posting with location `Remote` is ingested
- **THEN** the stored job has empty `countries` and empty `regions`

#### Scenario: A structured adapter work mode is persisted over the parsed one

- **WHEN** an adapter reports a structured `work_mode` (e.g. Lever's
  `workplaceType=hybrid`) for a posting whose location parses as `remote`
- **THEN** the stored `jobs.work_mode` is the structured value `hybrid`

### Requirement: A boardless provider may omit its board id

A source adapter SHALL be able to declare itself `boardless` when its API has no
per-tenant board id, so its configured entries MAY omit `board`. A `boardless`
provider MAY serve exactly one company or MAY aggregate postings from many
companies; the marker concerns only the absence of a board id, not the number of
companies. A provider that has a board or tenant concept SHALL NOT be boardless
and SHALL continue to require a non-empty `board`; this includes `yandex`, whose
`board` selects host and language.

#### Scenario: Boardless adapter is dispatched without a board

- **WHEN** a `boardless` provider's entry is crawled with an empty `board`
- **THEN** the adapter fetches its postings without requiring a board id and
  yields the normalized job shape

#### Scenario: Yandex requires its board

- **WHEN** a `yandex` entry is configured with an empty `board`
- **THEN** validation fails fast, because `yandex` selects host and language (`ru`/`com`)
  by its `board` and is not boardless

### Requirement: An adapter may send a per-request custom header

The shared HTTP client SHALL allow an adapter to attach custom request headers to an
individual request, in addition to the client's standard `User-Agent` and `Accept`,
without changing the behavior of requests that send no custom header. This SHALL support
sources that gate an otherwise-public JSON API behind a non-secret header (for example
MTS's public `x-api-key`), and the standard `User-Agent`/`Accept` and the retry/backoff
behavior SHALL be unchanged.

#### Scenario: Custom header is sent when provided

- **WHEN** an adapter issues a request with a custom header (e.g. `x-api-key`)
- **THEN** the outgoing request carries that header alongside the standard headers

#### Scenario: Existing adapters are unaffected

- **WHEN** an existing adapter issues a request through the unchanged `GetJSON`/`PostJSON`
  entry points
- **THEN** the request is sent with the standard headers and no custom header, exactly as
  before

### Requirement: An adapter may derive the description from source HTML

An adapter SHALL be permitted to obtain a posting body by extracting it from the source's
server-rendered HTML when the platform exposes no machine-readable description in its API,
and SHALL yield it as sanitized HTML, consistent with every other adapter's description
contract. Such extraction SHALL be isolated to that adapter, and an extraction failure for
one posting SHALL drop only that posting rather than abort the board.

#### Scenario: VK description is extracted from the vacancy page

- **WHEN** a `vk` board is crawled and VK's API carries no description field
- **THEN** the adapter fetches the vacancy page, extracts the description sections from its
  HTML, and yields a sanitized HTML description in the normalized job shape

#### Scenario: A failed extraction drops only that posting

- **WHEN** one VK posting's page cannot be fetched or its body cannot be extracted
- **THEN** that posting is skipped and every other posting is still yielded, without
  aborting the board

### Requirement: Russian bigtech single-company providers are registered

The system SHALL register adapters for the single-company Russian career APIs `yandex`,
`ozon`, `rwb`, `sber`, `alfabank`, `lamoda`, `kuper`, `aviasales`, `dodo`, `domclick`,
`mtslink`, `tbank`, `mts`, and `vk`, so each can be listed in the boards configuration.
Each adapter SHALL yield the normalized job shape (at least title, url, location, remote
flag, description, and the platform's native posting id) with the `description` as
sanitized HTML (or sanitized text for an API that publishes plain text) assembled from the
platform's authoritative field(s). An adapter whose list endpoint omits the description
SHALL fetch each posting's detail with bounded concurrency rather than yield an empty body,
and a single failed detail SHALL drop only that posting rather than abort the board. All
providers except `yandex` SHALL be `boardless`; `yandex` SHALL select host and language
(`ru`/`com`) from its `board`.

#### Scenario: Cursor-paginated board is fully crawled

- **WHEN** a `yandex` board is crawled and its list endpoint paginates by cursor
- **THEN** the adapter follows the cursor until exhausted, skips postings that redirect out
  or are hiring events, fetches each remaining posting's detail for the description, and
  yields each as the normalized job shape with `external_id` set to the posting's native id

#### Scenario: Page-paginated board is fully crawled

- **WHEN** an `ozon` board is crawled and its list endpoint paginates by page number
- **THEN** the adapter walks every page to the reported total, keeps only externally-listed
  vacancies, fetches each posting's detail for the description, and yields each as the
  normalized job shape

#### Scenario: Offset-paginated board with inline body needs no detail call

- **WHEN** a `sber` or `alfabank` board is crawled and the list endpoint carries the full
  body inline
- **THEN** the adapter walks the offset window to the reported total and yields each posting
  directly with a sanitized description, issuing no per-posting detail request

#### Scenario: Header-gated board is crawled

- **WHEN** an `mts` board is crawled and its API requires a non-secret `x-api-key` header
- **THEN** the adapter obtains the public key, sends it on each request, walks the offset
  window to the reported total, fetches each posting's detail, and yields each as the
  normalized job shape

#### Scenario: A board with no open postings yields no jobs without error

- **WHEN** any of these providers' endpoints returns an empty posting list for a configured
  board
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

### Requirement: An aggregator provider derives each job's company from the posting

An aggregator provider MUST set each job's company from the posting payload itself, not from the configured board entry. An aggregator is a provider that crawls a marketplace whose postings each name a different employer. The configured entry's company field is a placeholder used only to satisfy board-file validation. An aggregator provider MUST be boardless (one global feed, no per-tenant board id), so its entry omits the board id.

#### Scenario: Each posting keeps its own employer

- **WHEN** an aggregator board returns postings whose payloads name employers "Sliiip" and "Psyflo"
- **THEN** the corresponding jobs carry `Company` "Sliiip" and "Psyflo" respectively
- **AND** neither inherits the configured entry's placeholder company

### Requirement: Tecla is a registered boardless aggregator provider

The `tecla` provider SHALL crawl the `app.tecla.io` marketplace through its public JSON API,
paginating `getPublicJobs` over the API-reported page count up to a defensive cap, and map
each posting to the normalized job shape. Tecla is a remote-only marketplace, so every job
SHALL be marked remote. The public API truncates the description; the adapter SHALL persist
that public text as-is (the full, auth-gated text is intentionally not fetched).

#### Scenario: Tecla board is crawled across pages

- **WHEN** the `tecla` provider crawls its boardless entry and the API reports more than one page
- **THEN** the adapter requests each page through the API-reported page count
- **AND** every returned posting becomes a job with `ExternalID` set to the posting id and `URL` pointing at `app.tecla.io/job?id=<id>`

#### Scenario: Tecla posting maps to a remote job with its own employer

- **WHEN** a tecla posting carries a title, an employer name, a created timestamp, and a (truncated) description
- **THEN** the job's `Title`, `Company`, `PostedAt`, and `Description` come from that payload
- **AND** the job is marked remote (`Remote` true, `WorkMode` "remote")

### Requirement: An adapter MAY enumerate a board through a sitemap with JSON-LD detail

The system SHALL support adapters that enumerate a board's postings from the
board's XML sitemap rather than a JSON list endpoint, and that obtain each
posting's fields from a server-rendered detail page carrying a schema.org
`JobPosting` JSON-LD block. Such an adapter SHALL filter non-posting sitemap
entries, SHALL derive the platform's native posting id from the posting URL, and
SHALL bound its per-posting detail fetches (as required of any detail-fetching
adapter). A posting whose detail page cannot be fetched or carries no usable
posting id SHALL be skipped without aborting the rest of the board.

#### Scenario: Adapter enumerates postings from a sitemap

- **WHEN** an adapter fetches a board whose postings are listed only in an XML
  sitemap (e.g. an iCIMS career site)
- **THEN** the adapter reads the sitemap, keeps only the job-posting URLs, and
  fetches each posting's detail page to obtain the normalized job shape

#### Scenario: Adapter reads posting fields from JSON-LD detail

- **WHEN** a posting's detail page server-renders a schema.org `JobPosting`
  JSON-LD block
- **THEN** the adapter maps that block to the normalized job shape (at least
  title, url, location, description, and the platform's native posting id)

#### Scenario: A posting whose detail fetch fails is skipped

- **WHEN** one posting's detail page cannot be fetched or yields no posting id
- **THEN** that posting is dropped and the remaining postings on the board are
  still ingested

### Requirement: A multi-company aggregator stays in the source facet

The source facet (the provider list a user filters results by) SHALL exclude a
`boardless` provider that serves a single company, because filtering by it is
redundant with the company filter. A `boardless` provider that aggregates
postings from many companies SHALL declare itself an `aggregator` and SHALL
remain a value in the source facet, because filtering by "this aggregator" is not
redundant with any single company. A provider that has a board concept (not
boardless) SHALL remain in the source facet as before.

#### Scenario: Single-company boardless provider is excluded from the source facet

- **WHEN** the source-facet provider list is built (e.g. for the web contract)
- **THEN** a single-company boardless provider such as `ozon` is not listed

#### Scenario: Aggregator boardless provider stays in the source facet

- **WHEN** the source-facet provider list is built
- **THEN** an `aggregator` boardless provider such as `jobstash` or the existing
  `tecla` marketplace is listed, while the board-based providers (`greenhouse`,
  `lever`, …) remain listed as before

### Requirement: JobStash crypto-job aggregator is registered

The system SHALL register a `jobstash` adapter for the JobStash Web3 job
aggregator, configurable as a single `boardless` entry. The adapter SHALL
paginate JobStash's job-list endpoint by the reported total and yield every
posting as the normalized job shape (at least title, url, location, remote flag,
description, and the platform's native posting id). The company of each posting
SHALL be taken from the posting's `organization.name`, so one board yields jobs
across many companies. A posting carrying a `url` (a `public` posting's downstream
ATS apply link) SHALL use it unchanged; a posting with no `url` (a `protected`
posting, whose gated link the feed omits) SHALL link to the JobStash detail page
derived from its native id, so the stored job links to the real apply target when
one exists and to the best available link otherwise. A posting with no native id
or no company SHALL be dropped rather than persisted, since it would break the
dedup key or the company slug. The structured location type
(`ONSITE`/`REMOTE`/`HYBRID`) SHALL set the structured `work_mode`, and the publish
timestamp SHALL set `posted_at`. The adapter SHALL be both `boardless` and an
`aggregator`.

#### Scenario: JobStash board is fully crawled across companies

- **WHEN** a `jobstash` entry is crawled with an empty `board`
- **THEN** the adapter walks the list endpoint to the reported total and yields
  each posting as the normalized job shape, with `company` taken from the
  posting's `organization.name` and `external_id` set to the posting's native id

#### Scenario: A public posting links to its downstream ATS

- **WHEN** JobStash returns a posting whose `access` is `public` with a downstream
  ATS `url`
- **THEN** the yielded job's `url` is that downstream ATS link

#### Scenario: A protected posting with no url links to its JobStash page

- **WHEN** JobStash returns a `protected` posting whose `url` is null
- **THEN** the yielded job's `url` is the JobStash detail page derived from the
  posting's native id

#### Scenario: A posting with no company or id is dropped

- **WHEN** JobStash returns a posting with an empty `organization.name` or an
  empty native id
- **THEN** that posting is not yielded, while the other postings are

#### Scenario: Location type sets the structured work mode

- **WHEN** a posting reports `locationType` `REMOTE`
- **THEN** the yielded job's structured `work_mode` is `remote`

### Requirement: Google is a registered provider

The system SHALL register a `google` adapter so Google's own careers catalogue
(`www.google.com/about/careers/applications/jobs/results`) can be listed in a
`sources/*.yml` board file. The adapter SHALL be **boardless** (a single-company source
with no per-tenant board id), like the existing `uber`/`amazon` adapters. It SHALL fetch the
server-rendered list pages over the shared `HTTPClient`, paging via the `?page=N` query
parameter, and SHALL read each posting from the JSON payload embedded in the page's
`AF_initDataCallback({key:'ds:1', data:[…]})` script block — without any per-job detail
request, since that payload already carries the full job text. The adapter SHALL yield the
normalized job shape (at least title, url, location, description, and the platform's native
posting id), with `external_id` set to Google's numeric job id, `url` set to the public
`…/jobs/results/<id>` results page built from the job id (Google ignores any trailing slug,
so none is reproduced; NOT the sign-in-gated apply link), and `description` as sanitized HTML
assembled from the posting's description, responsibilities, and qualifications fields.

#### Scenario: Google catalogue is crawled page by page

- **WHEN** a `sources/*.yml` board lists provider `google`
- **THEN** the adapter requests `…/jobs/results?page=1`, extracts the `ds:1` payload, yields
  each embedded job record as the normalized job shape, then requests the next page, and
  continues until a page yields no records or the running yielded count reaches the payload's
  total-count field

#### Scenario: Job is mapped from the embedded record

- **WHEN** a page's `ds:1` payload carries a job record
- **THEN** the adapter yields a job whose `external_id` is the record's numeric id, whose
  `url` is the public `…/jobs/results/<id>` page built from the id alone, whose `title`,
  `location` (from the structured locations array), and `posted_at`
  (from the embedded Unix-epoch timestamp) come from the record, and whose `description` is
  sanitized HTML assembled from the record's description, responsibilities, and qualifications
  fields

#### Scenario: A posting is dated from its embedded timestamp

- **WHEN** a Google job record carries a Unix-epoch publish timestamp
- **THEN** the adapter yields the job with `posted_at` derived from that timestamp, and
  yields a nil `posted_at` when the timestamp is absent or zero

#### Scenario: An empty or out-of-range page ends the crawl without error

- **WHEN** a requested page past the last page returns no `ds:1` job records (the payload's
  job list is null or empty)
- **THEN** the adapter stops paging, yields the jobs collected so far, and returns no error

