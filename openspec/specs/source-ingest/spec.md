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

The pipeline SHALL persist each fetched posting that passes the catalogue-fit check via the existing job write path, and SHALL reject — without persisting — any posting whose title the non-tech dictionary flags. It
SHALL set `source` to the board's provider, derive `company_slug` from the company
name using the existing slug normalization, and set `external_id` to the namespaced
form `"<board>:<native-posting-id>"`. Namespacing SHALL guarantee that two companies
on the same platform sharing a native posting id do not collide on the dedup key
`UNIQUE (source, external_id)`. Rejections MUST be counted separately from postings skipped by a construct or save error, so a catalogue-fit rejection never reads as a board malfunction, and a board with any rejections SHALL be logged once per run with its rejected share — a board rejecting everything is the signature of a dictionary term that is too broad, and must be visible within one crawl cycle. The rejection applies only to the crawled board pipeline; write paths that are never re-crawled are unaffected, because a rejection there could not be undone by a later crawl.

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

#### Scenario: A non-technical posting is rejected before the write

- **WHEN** a fetched posting's title is flagged by the non-tech dictionary
- **THEN** the posting is not persisted and is counted as rejected, not as skipped

#### Scenario: Rejections do not mask save failures

- **WHEN** a board yields both a rejected posting and a posting that fails to save
- **THEN** the run reports one rejection and one skip separately

#### Scenario: A board rejecting everything is visible

- **WHEN** a board's postings are all rejected in a run
- **THEN** the run logs that board once with its rejected count and share

#### Scenario: Non-crawled write paths are unaffected

- **WHEN** a job is created through Telegram extraction, a user submission, or a link-source import and its title would be flagged
- **THEN** the job is still persisted

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

The system SHALL provide a standalone `cmd/ingest` binary that loads configuration, runs every configured board once with bounded concurrency, reports how many jobs were ingested and how many sources failed, and exits — suitable for scheduled execution. It SHALL accept an optional shard selector `--shard=i/n` (or the `SHARD` environment variable, both 1-based) that restricts the run to shard i of n, where distinct companies (keyed by their normalized company slug) are assigned round-robin to shards and all of a company's boards go to the same shard, so a provider whose board list is too large to finish within one timeout can be partitioned across several staggered runs that together cover the whole file. All boards of one company SHALL land in the same shard, so the per-company stale-job sweep of one shard never closes the still-live boards another shard owns. A malformed or out-of-range shard selector SHALL fail fast before any crawl, and the full board file SHALL be validated on every shard's run (not only the slice it crawls).

#### Scenario: Ingest command runs a bounded batch and exits

- **WHEN** the ingest command is run
- **THEN** it processes every configured board once and exits after reporting the ingested and failed counts

#### Scenario: A shard selector crawls only its slice

- **WHEN** the ingest command is run with `--shard=i/n` on a board file
- **THEN** it crawls only shard i's boards, and N such runs together crawl every board exactly once

#### Scenario: A company's boards are never split across shards

- **WHEN** a board file lists several boards for the same company and it is crawled with `--shard=i/n`
- **THEN** all of that company's boards fall in a single shard, so no shard's per-company stale sweep closes boards another shard owns

#### Scenario: A malformed shard selector fails fast

- **WHEN** the ingest command is run with a shard selector that is not `i/n` with `1 <= i <= n` and `n >= 1`
- **THEN** it reports the error and exits without crawling

### Requirement: Adapter descriptions are sanitized HTML

Each adapter SHALL yield the job `description` as sanitized HTML assembled from the platform's authoritative HTML field(s), so the stored value is safe to render directly in a browser without further escaping. An adapter SHALL NOT yield raw or entity-encoded source markup, and SHALL NOT rely on a platform plain-text field that the platform may leave empty or partial. Sanitization SHALL run server-side before the description is persisted, stripping scripts, event handlers, and other active content while preserving structural formatting (headings, paragraphs, lists, emphasis).

A persisted description SHALL carry no links. Sanitization SHALL unwrap every anchor to its visible text, so a posting can neither route a reader off the catalogue nor pass link authority, while the sentence the link sat in stays whole. It SHALL discard an anchor whose visible text merely restates its own destination, together with any block element that leaves empty — stripped of its href such text is a dead address rather than prose.

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

#### Scenario: A worded link keeps its words and loses its destination

- **WHEN** a source posting's HTML links a word inside a sentence (e.g. `We use <a href="https://k8s.io">Kubernetes</a> in prod.`)
- **THEN** the persisted description reads `We use Kubernetes in prod.` with no anchor, href, or `rel` attribute

#### Scenario: A link labelled with its own address is discarded

- **WHEN** a source posting's HTML contains an anchor whose visible text restates its destination (e.g. `<p><a href="https://x.co/1">https://x.co/1</a></p>`)
- **THEN** the persisted description contains neither the address nor the paragraph it stood alone in

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

When an ATS exposes BOTH a work-mode field and a boolean flag for the same posting, the
adapter SHALL resolve the mode from the field, and MAY fall back to the flag only for
postings that omit the field — for a posting that carries the field, a flag that merely
separates onsite from every other arrangement SHALL NOT be mapped to `remote`, because
such a flag is set on hybrid postings too. Where an ATS splits the signal across separate
`remote` and `hybrid` booleans, both false SHALL yield no work mode rather than a guessed
`onsite`, since the API cannot distinguish "marked as office" from "not marked at all";
both true SHALL yield `remote`, the broader arrangement.

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

#### Scenario: An Ashby hybrid posting is not remote

- **WHEN** an Ashby posting carries `workplaceType=Hybrid` together with `isRemote=true`
- **THEN** the yielded job's `work_mode` is `hybrid` and the job is not marked remote

#### Scenario: An Ashby posting without a workplace type falls back to the flag

- **WHEN** an Ashby posting omits `workplaceType` and carries `isRemote=true`
- **THEN** the yielded job's `work_mode` is `remote`

#### Scenario: A Recruitee hybrid offer keeps its work mode

- **WHEN** a Recruitee offer carries `remote=false` and `hybrid=true`
- **THEN** the yielded job's `work_mode` is `hybrid` and the job is not marked remote

#### Scenario: A SmartRecruiters hybrid posting keeps its work mode

- **WHEN** a SmartRecruiters posting's location carries `remote=false` and `hybrid=true`
- **THEN** the yielded job's `work_mode` is `hybrid` and the job is not marked remote

#### Scenario: Neither remote nor hybrid leaves the work mode unknown

- **WHEN** an offer carries `remote=false` and `hybrid=false`
- **THEN** the yielded job carries no structured `work_mode`, leaving the decision to the
  pipeline's location parser

#### Scenario: An offer flagged both remote and hybrid resolves to remote

- **WHEN** an offer carries `remote=true` and `hybrid=true`
- **THEN** the yielded job's `work_mode` is `remote`

#### Scenario: A BambooHR posting takes its work mode from locationType

- **WHEN** a BambooHR careers-list posting carries `locationType` `0`, `1`, or `2` with a
  null `isRemote`
- **THEN** the yielded job's `work_mode` is `onsite`, `remote`, or `hybrid` respectively,
  and only the `remote` one is marked remote

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

### Requirement: An aggregator provider derives each job's company from the posting

An aggregator provider MUST set each job's company from the posting payload itself, not from the configured board entry. An aggregator is a provider that crawls a marketplace whose postings each name a different employer. The configured entry's company field is a placeholder used only to satisfy board-file validation. An aggregator provider MUST be boardless (one global feed, no per-tenant board id), so its entry omits the board id.

#### Scenario: Each posting keeps its own employer

- **WHEN** an aggregator board returns postings whose payloads name employers "Sliiip" and "Psyflo"
- **THEN** the corresponding jobs carry `Company` "Sliiip" and "Psyflo" respectively
- **AND** neither inherits the configured entry's placeholder company

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

### Requirement: A board entry MAY be marked as a community hub whose employer comes from each posting

The system SHALL support an optional `hub` flag on a board-file entry that marks a board-based
provider's board as a community/agency hub: a board that lists vacancies on behalf of many
partner companies rather than a single employer. Unlike a boardless aggregator (one global feed,
no board id), a hub entry still names a board id and still requires a `company` — that configured
company is the hub's own name and the per-vacancy fallback employer. When the flag is absent or
false, the provider's existing behaviour SHALL be unchanged and each job's company SHALL be the
configured entry company.

For a hub entry, the provider SHALL resolve each vacancy's employer from the posting itself
rather than from the configured company, falling back to the configured company when the posting
carries no resolvable employer (for example the hub's own internal roles).

A hub entry MAY additionally carry an optional `tenants` map, from the tenant key the platform
stamps on a posting to that tenant's employer display name. It exists for hubs whose postings
identify the tenant only by an opaque key — a URL path segment or an id — which is not itself a
usable company name. The map SHALL be optional and SHALL be ignored by providers that do not
implement hub resolution, exactly as `hub` and `region` are. A tenant key absent from the map
SHALL fall back to the configured company rather than be turned into a name by transforming the
key, because a plausible-but-wrong employer is worse in the catalogue than the parent brand.

#### Scenario: A non-hub entry attributes every job to the configured company

- **WHEN** a board file lists an entry without the `hub` flag (or with `hub: false`)
- **THEN** each crawled job's company is the entry's configured `company`, exactly as before

#### Scenario: A hub entry attributes each job to the employer named in the posting

- **WHEN** a board file lists an entry with `hub: true`
- **THEN** each crawled job's company is resolved from that vacancy's own payload, and only a
  vacancy with no resolvable employer falls back to the configured `company`

#### Scenario: A hub entry maps an opaque tenant key to an employer name

- **WHEN** a hub entry carries a `tenants` map and a vacancy's tenant key is present in it
- **THEN** the job's company is that map's value

#### Scenario: An unmapped tenant key falls back rather than being invented

- **WHEN** a hub entry carries a `tenants` map and a vacancy's tenant key is absent from it
- **THEN** the job's company is the configured `company`, and the key is NOT transformed into a
  name

### Requirement: Source adapters may emit structured facet signals

The normalized `Job` a source adapter returns SHALL carry optional structured
facet fields — `Seniority`, `Category`, `Skills`, and `ExperienceYearsMin` —
alongside the existing `WorkMode`. Each field carries a STRUCTURED signal the
source states explicitly (an enum, tag list, or numeric field), never a value an
adapter inferred from free text. An adapter that has no structured signal for a
field SHALL leave it empty/nil, so the downstream derivation falls back to the
dictionaries. An adapter SHALL map a source value into freehire's controlled
vocabulary and SHALL emit nothing for a value it cannot map (it never guesses or
forwards an out-of-vocabulary value).

#### Scenario: An adapter with no structured signal leaves the fields empty

- **WHEN** an adapter normalizes a posting whose platform exposes no grade, skills,
  category, or experience field
- **THEN** the returned `Job` has empty `Seniority`/`Category`/`Skills` and a nil
  `ExperienceYearsMin`

#### Scenario: An unmappable source value is dropped, not forwarded

- **WHEN** an adapter reads a source facet value with no equivalent in freehire's
  controlled vocabulary
- **THEN** the corresponding `Job` field is left empty rather than carrying the raw value

### Requirement: The write path reports new and changed writes for indexing

The job write path SHALL maintain a deterministic content hash over a job's
indexed fields and SHALL report, for each write, whether it **inserted** a new
row or **changed** the indexed content of an existing one. An upsert that updates
only bookkeeping (such as the last-seen timestamp) without changing any indexed
field SHALL report neither inserted nor changed. The hash SHALL be derived from
the same persisted values that form the search document, so the signal tracks
exactly what the index would need re-pushed.

#### Scenario: A first-time write reports inserted

- **WHEN** a posting not previously in the catalogue is persisted
- **THEN** the write reports it as inserted

#### Scenario: An edited write reports changed

- **WHEN** an existing posting is persisted with an edited indexed field (e.g. its
  title)
- **THEN** the write reports it as changed

#### Scenario: A no-op refresh reports neither

- **WHEN** an existing posting is persisted with identical indexed fields and only
  its last-seen timestamp advances
- **THEN** the write reports neither inserted nor changed

### Requirement: Ingest feeds new and changed jobs to the search index

When the search engine is configured for the ingest worker, the worker SHALL push
the open jobs reported as inserted or changed to the live facet search index,
batched rather than one document per posting, after they are persisted. The push
SHALL be best-effort: a search-index error SHALL be logged and SHALL NOT fail the
ingest run or change its exit code. When the search engine is not configured for
the worker, ingest SHALL run unchanged and skip indexing.

#### Scenario: Configured worker indexes new and changed jobs

- **WHEN** a crawl persists new and edited jobs and the search engine is configured
- **THEN** those jobs' documents are pushed to the live facet index in batches

#### Scenario: Unconfigured worker skips indexing

- **WHEN** ingest runs without a configured search engine
- **THEN** ingest persists and sweeps as before and pushes nothing to the index

#### Scenario: Index error does not fail the run

- **WHEN** pushing documents to the search engine fails
- **THEN** the error is logged and the ingest run's success is unaffected

### Requirement: The write path persists a repost-clustering role fingerprint
The `UpsertJob` write path SHALL compute and persist a narrow `role_fingerprint` for every job — derived from company, normalized title, and description, and deliberately excluding volatile fields (`posted_at`, url, public slug) — so reposts of the same role under new `external_id`s cluster to one fingerprint.

#### Scenario: The fingerprint ignores a bumped posted date
- **WHEN** a role is reposted under a new `external_id` with an identical title and description but a refreshed `posted_at`
- **THEN** both postings resolve to the same `role_fingerprint`

#### Scenario: content_hash is not reused as the fingerprint
- **WHEN** the write path computes the fingerprint
- **THEN** it uses a dedicated narrow fingerprint (not `content_hash`, which includes `posted_at` and so changes on repost)

#### Scenario: A distinct role gets a distinct fingerprint
- **WHEN** two postings differ in normalized title or description
- **THEN** they resolve to different `role_fingerprint` values

### Requirement: Adapters may hydrate only postings the catalogue lacks

The pipeline SHALL support an optional adapter capability that fetches per-posting detail
only for postings the catalogue does not already have, so an adapter whose detail requests
are expensive (a large aggregator, or a board with tens of thousands of postings) issues them
only for new postings rather than on every crawl. The capability SHALL be opt-in through a
distinct adapter interface (not a change to `Source.Fetch`), so every other adapter and the
base fetch signature are unchanged — the same optional-port shape the pipeline already uses
for streaming and self-closing sources.

When a board's adapter implements this capability, the pipeline SHALL supply a `seen(externalID)`
predicate backed by the set of `external_id`s already stored for **that board**, obtained with a
single query per crawled board (not one query per posting). The seen-set SHALL be scoped by the
board's `external_id` namespace prefix rather than by provider alone, because the lookup runs once
per board: a provider-wide set makes the capability unusable for a multi-board provider, whose
per-board cost would be the whole provider's catalogue. A boardless adapter, whose postings carry
no board prefix, SHALL keep the provider-wide set. An adapter that does not implement the
capability SHALL be crawled through the normal fetch path, unaffected.

A hydrating adapter re-lists postings it already ingested but fetches no fresh content for them,
so the pipeline MUST refresh such a posting's liveness (last-seen state, and reopen if closed)
WITHOUT rewriting its content columns. A content-less re-upsert would re-derive the deterministic
facets from an empty description and overwrite the row's hydrated description/skills/etc. The
adapter SHALL mark an already-ingested posting for liveness refresh; the pipeline routes a marked
posting to the refresh path instead of the content upsert. (This mirrors the removed-posting
close routing.) A refresh keeps the posting out of the post-run unseen sweep just as a full
upsert would.

A liveness refresh SHALL be subject to the same catalogue-fit check as a content upsert: a
posting the non-tech dictionary flags is neither written nor refreshed, so it stops being seen
and the unseen sweep closes it. Without this, hydration would keep a rejected posting alive
indefinitely and defeat the pruning path that depends on a rejected posting ageing out. A
refusal to refresh SHALL be counted as a catalogue rejection, not as a skip.

The check on a refresh SHALL read the tech evidence STORED with the row rather than what the
re-listed posting alone implies, so it reaches the same verdict the write path would. The
dictionary is consulted only after the tech check, and a hydrating crawl carries no description
to derive that check from: judged on the listing alone, 1.7% of the titles the catalogue holds
as technical are flagged — engineering roles whose evidence lives in the description they were
hydrated from. The seen-set SHALL therefore carry each stored posting's tech evidence alongside
its id, since it is read from the same rows.

#### Scenario: Already-ingested posting refreshes liveness without a content rewrite

- **WHEN** a hydrating adapter yields a posting marked for liveness refresh (already ingested)
- **THEN** the pipeline refreshes its last-seen/open state and does NOT overwrite its stored
  description or facets

#### Scenario: Hydrating adapter is driven by the seen-set

- **WHEN** a board's adapter implements the hydrating capability
- **THEN** the pipeline loads the set of already-ingested external ids for that board once
  and passes a membership predicate to the adapter, which hydrates only postings not in the set

#### Scenario: The seen-set of one board excludes a sibling board on the same host

- **WHEN** two boards of one provider share a host prefix (e.g. `…/dollartreeus` and
  `…/dollartreeca`) and each has stored postings
- **THEN** a board's seen-set contains only its own postings, so a posting of the sibling board
  is treated as new rather than as already ingested

#### Scenario: A boardless adapter keeps the provider-wide seen-set

- **WHEN** a hydrating adapter is boardless, so its stored `external_id`s carry an empty board
  prefix
- **THEN** the pipeline supplies the set of every `external_id` stored for the provider

#### Scenario: Non-hydrating adapter is unaffected

- **WHEN** a board's adapter does not implement the hydrating capability
- **THEN** the pipeline crawls it through the normal fetch path and issues no seen-set query

#### Scenario: Seen-set lookup failure does not abort the crawl

- **WHEN** the seen-set query fails for a hydrating board
- **THEN** the pipeline logs the failure and crawls the board with an empty seen-set (every
  offer treated as new) rather than skipping the board

#### Scenario: A rejected posting is not kept alive by a liveness refresh

- **WHEN** a hydrating adapter marks an already-ingested posting for liveness refresh, the
  non-tech dictionary flags its title, and its stored row carries no tech evidence
- **THEN** the pipeline does NOT refresh it, counts it as a catalogue rejection, and the posting
  ages out to the unseen sweep

#### Scenario: A flagged title whose stored row proves it technical is still refreshed

- **WHEN** a posting marked for liveness refresh has a title the dictionary flags but its stored
  row carries tech evidence (the description it was hydrated from established it)
- **THEN** the pipeline refreshes it and counts no rejection — the same verdict the write path
  reaches with the description in hand

### Requirement: The provider taxonomy is independent of crawl credentials

The registry constructor SHALL answer two separable questions and MUST NOT conflate them: which
providers this process can crawl (a runtime fact about configured credentials and transport) and
what kind of source each provider is (a static fact about the adapter type, expressed by its
`Provider()` key and its marker interfaces).

The registry SHALL therefore be assembled in two modes by the same constructor:

- Assembled **with an HTTP client** — the crawl registry — it SHALL omit any provider whose
  credential is not configured, so a board file naming that provider fails config validation
  before any request is made.
- Assembled **without an HTTP client** — the taxonomy registry — it SHALL be total: every
  adapter the binary knows about is present with its markers, whatever the environment holds. A
  credential-bearing adapter SHALL be registered here with an empty credential, which is safe
  because the taxonomy path never fetches.

Consumers that classify rather than crawl — the source facet's provider list, the status
page's provider kind, and the aggregator set used by the cross-source dedup pass — SHALL read
the taxonomy registry, and their answers SHALL NOT vary with the environment's credentials.

#### Scenario: The taxonomy registry lists a keyed provider without its credential

- **WHEN** `USAJOBS_API_KEY`, `REED_API_KEY` and `WHATJOBS_PUBLISHER_IDS` are all unset and the
  registry is assembled without an HTTP client
- **THEN** the registry contains `usajobs`, `reed` and every WhatJobs market provider (`whatjobs`,
  `whatjobs-br`, and the rest of `whatjobsMarkets`), each carrying its markers
- **AND** the source facet's provider list, the aggregator set and each provider's kind are the
  same as they are in a fully configured environment

#### Scenario: The crawl registry still omits an unconfigured provider

- **WHEN** `WHATJOBS_PUBLISHER_IDS` is unset and the registry is assembled with an HTTP client
- **THEN** the registry has no `whatjobs` entry (nor any other WhatJobs market provider)
- **AND** validating a board file that names `whatjobs` fails before any request is made

#### Scenario: An aggregator is suppressed against its ATS twin on any host

- **WHEN** the reindex pass computes the aggregator set on a host that has no ingest credentials
- **THEN** `whatjobs` is in that set, so a `whatjobs` posting duplicating a first-party ATS
  posting of the same company, title and country is marked a duplicate of the ATS row

### Requirement: An adapter may yield an application form with a job

An adapter SHALL be able to yield, together with a normalized job, the
application form the platform published for that posting, when the platform's
list endpoint already carries it. The ingest write path SHALL persist such a form
alongside the job.

Yielding a form SHALL remain optional: an adapter that yields none SHALL keep
working unchanged, and the absence of a form SHALL NOT fail the posting, the
board, or the run. An adapter SHALL NOT issue an extra request in order to yield
a form — a form that costs a request is captured after ingest, not during it.

#### Scenario: A yielded form is persisted with the job

- **WHEN** an adapter yields a job carrying an application form
- **THEN** ingest writes the job and stores the form against it

#### Scenario: An adapter that yields no form is unaffected

- **WHEN** an adapter yields a job carrying no application form
- **THEN** ingest writes the job exactly as it does today and stores no form

### Requirement: An adapter may supply a company-level description separate from posting bodies

A `Source` adapter MAY, when its platform exposes a company/board-level
description through a distinct endpoint or field from the one it uses for
postings, fetch and return that text once per board crawl. An adapter that
does not implement this SHALL simply return nothing for it, and the pipeline
SHALL treat that as "no company description available" rather than an error.

Fetching a company-level description SHALL NOT require a per-posting
request: it is fetched at most once per board per crawl, independent of how
many postings that board yields.

#### Scenario: An adapter returns a company description alongside its postings

- **WHEN** an adapter's platform exposes a company-level description and the
  adapter implements this capability
- **THEN** a single crawl of that board yields the postings as before, plus
  one company-level description for the board

#### Scenario: An adapter without this capability yields nothing extra

- **WHEN** an adapter does not implement a company-level description lookup
- **THEN** the pipeline receives no company description for that board and
  proceeds exactly as it did before this capability existed

#### Scenario: A blank company-level field is not treated as a description

- **WHEN** an adapter's platform exposes the field but the employer left it
  empty
- **THEN** the adapter returns nothing for it, not an empty string
