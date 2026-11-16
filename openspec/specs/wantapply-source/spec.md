# wantapply-source Specification

## Purpose

Collect original WantApply vacancies from the independent global and Cyprus catalogues,
using the public listing the website itself reads.

## Requirements

### Requirement: Independent regional catalogues

The `wantapply` source SHALL read `https://wantapply.com/api/jobs` with
`filters={"domain":"tech"}`. The `wantapply-cy` source SHALL read
`https://wantapply.cy/api/jobs` with `filters={"jobLocations":["CYP"]}`.
Both are boardless multi-employer sources. A transport failure SHALL NOT switch domains.
The global source uses the Scrapling browser in production; Cyprus retains proxy egress.

#### Scenario: Preserve published identity

- **WHEN** either listing returns `url="backend-at-acme"`
- **THEN** its adapter yields `ExternalID="backend-at-acme"` and that source's own domain URL
- **AND** it does not replace the existing slug identity with the API's UUID

### Requirement: Complete, bounded pagination

The adapter SHALL request pages of 50, the frontend's actual maximum, and follow
`hasNextPage`. A crawl SHALL succeed only after observing an explicit collection,
explicit pagination metadata, a stable nonnegative `total`, and exactly that many
unique usable published vacancies. A missing collection, request failure, repeated
posting, changed total, count mismatch, empty nonterminal page, or page ceiling SHALL
fail the crawl without returning a partial catalogue.

#### Scenario: A later page fails

- **WHEN** a later request fails after an earlier page was read
- **THEN** Fetch returns an error and no partial jobs, preventing a source-wide close

#### Scenario: A verified empty catalogue

- **WHEN** the API returns `data=[]`, `hasNextPage=false`, and `total=0`
- **THEN** Fetch succeeds with an empty result

### Requirement: Inline posting content

The adapter SHALL read sanitized rich descriptions, employer names, publication timestamps (creation time as fallback),
expiration dates, structured country codes, remote/workplace types, and employment types
from the listing. Employer names use the company profile name, falling back to the posting's company-name field. It SHALL refresh
all listed bodies each pass without an individual HTML request or a seen-set hydration gate.

#### Scenario: An employer edits a description

- **WHEN** the next listing contains an edited description for an existing slug
- **THEN** the pipeline receives the new body under the same posting identity

### Requirement: Original postings survive company coverage

Both sources SHALL retain original postings even when another source has an open vacancy
at the same employer. Posting-level deduplication SHALL still apply.

#### Scenario: Another vacancy does not establish duplication

- **WHEN** WantApply publishes a Backend Engineer vacancy and another source covers the employer
- **THEN** the pipeline saves the vacancy rather than discarding it by company coverage

### Requirement: Verified source-wide unseen sweep

Both adapters SHALL carry the full-catalogue capability because incomplete reads fail.
A vacancy absent from a complete published listing SHALL age out through the ordinary
source-wide unseen sweep. Neither source is self-closing.
