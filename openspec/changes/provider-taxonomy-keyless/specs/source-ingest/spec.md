## ADDED Requirements

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

- **WHEN** `USAJOBS_API_KEY`, `REED_API_KEY` and `WHATJOBS_PUBLISHER_ID` are all unset and the
  registry is assembled without an HTTP client
- **THEN** the registry contains `usajobs`, `reed` and `whatjobs`, each carrying its markers
- **AND** the source facet's provider list, the aggregator set and each provider's kind are the
  same as they are in a fully configured environment

#### Scenario: The crawl registry still omits an unconfigured provider

- **WHEN** `WHATJOBS_PUBLISHER_ID` is unset and the registry is assembled with an HTTP client
- **THEN** the registry has no `whatjobs` entry
- **AND** validating a board file that names `whatjobs` fails before any request is made

#### Scenario: An aggregator is suppressed against its ATS twin on any host

- **WHEN** the reindex pass computes the aggregator set on a host that has no ingest credentials
- **THEN** `whatjobs` is in that set, so a `whatjobs` posting duplicating a first-party ATS
  posting of the same company, title and country is marked a duplicate of the ATS row

## MODIFIED Requirements

### Requirement: Reed is a registered keyed, keyword-scoped aggregator provider

The ingest registry SHALL include a `reed` adapter over the Reed Jobseeker API
(reed.co.uk), and the crawl registry SHALL include it only when the `REED_API_KEY`
environment variable is set — like `usajobs`, the key is a secret read from the
environment and never stored in a board file. The taxonomy registry SHALL list `reed`
regardless, per "The provider taxonomy is independent of crawl credentials". The adapter
SHALL be boardless (one API, no per-tenant board id) and SHALL declare itself an
`aggregator`, taking each posting's employer from the API payload and remaining a value
in the source facet.

Because the Reed API filters only by free-text keywords (it exposes no sector
filter) and freehire is an IT job board, the adapter SHALL enumerate a topical IT
slice by searching a curated set of IT/technology keywords, unioning the results and
**deduping by the Reed job id** so a posting matched by several keywords is crawled
once. Because the search list omits the employer's real apply URL and truncates the
description, the adapter SHALL fetch each unique job's detail and take the full
description and the employer's `externalUrl` from it, falling back to the Reed
listing URL (`jobUrl`) when no `externalUrl` is present. Authentication SHALL use the
API key as HTTP Basic credentials.

Because the Reed API enforces a per-hour request quota, the adapter SHALL be a
hydrating adapter (per the "Adapters may hydrate only postings the catalogue lacks"
requirement): it SHALL fetch a posting's detail only when that posting is not already
ingested, and SHALL mark an already-ingested posting for a liveness refresh instead of
re-fetching its detail. When the pipeline cannot supply a seen-set, the adapter SHALL
fall back to fetching every unique job's detail.

#### Scenario: Crawled only when the key is configured

- **WHEN** `REED_API_KEY` is unset and the crawl registry is assembled
- **THEN** it does NOT contain `reed`
- **AND WHEN** `REED_API_KEY` is set
- **THEN** the crawl registry contains `reed` carrying that key

#### Scenario: Listed in the source facet whatever the environment holds

- **WHEN** the source-facet provider list is built with `REED_API_KEY` unset
- **THEN** `reed` is listed

#### Scenario: Keyword matches are deduped by job id

- **WHEN** the same Reed job id is returned by more than one of the curated
  keyword searches
- **THEN** the adapter emits that posting once, not once per matching keyword

#### Scenario: The employer apply URL comes from the job detail

- **WHEN** a job's detail carries an `externalUrl` (the employer's own posting)
- **THEN** the emitted job's URL is that `externalUrl`, not the Reed listing URL
- **AND WHEN** the detail has no `externalUrl`
- **THEN** the emitted job's URL falls back to the Reed listing URL

#### Scenario: Detail is fetched only for postings not already ingested

- **WHEN** the pipeline supplies a seen-set and a unioned Reed job id is already ingested
- **THEN** the adapter marks that posting for a liveness refresh and issues NO detail request
- **AND WHEN** a unioned Reed job id is not in the seen-set
- **THEN** the adapter fetches that job's detail and emits the hydrated posting

#### Scenario: Falls back to hydrating every posting without a seen-set

- **WHEN** the pipeline cannot supply a seen-set (e.g. a non-DB caller)
- **THEN** the adapter fetches every unique job's detail, as before
