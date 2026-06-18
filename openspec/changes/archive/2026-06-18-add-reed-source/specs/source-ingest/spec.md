# source-ingest Specification (delta)

## ADDED Requirements

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
