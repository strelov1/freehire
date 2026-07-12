## MODIFIED Requirements

### Requirement: Reed is a registered keyed, keyword-scoped aggregator provider

The ingest registry SHALL include a `reed` adapter over the Reed Jobseeker API
(reed.co.uk), and it SHALL be registered only when the `REED_API_KEY` environment
variable is set — like `usajobs`, the key is a secret read from the environment and
never stored in a board file. The adapter SHALL be boardless (one API, no per-tenant
board id) and SHALL declare itself an `aggregator`, taking each posting's employer
from the API payload and remaining a value in the source facet.

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

#### Scenario: Registered only when the key is configured

- **WHEN** `REED_API_KEY` is unset
- **THEN** `All()` does NOT register `reed`
- **AND WHEN** `REED_API_KEY` is set
- **THEN** `All()` registers `reed` and it appears in the source facet

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
