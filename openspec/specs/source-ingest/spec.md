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
