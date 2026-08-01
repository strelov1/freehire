## MODIFIED Requirements

### Requirement: The publisher id is read from the environment only

The system SHALL read the WhatJobs publisher id from the `WHATJOBS_PUBLISHER_ID` environment
variable and MUST NOT accept it from a board file. The crawl registry — the one assembled with
an HTTP client — SHALL contain `whatjobs` only when that variable is set to a non-empty value,
so an environment without the credential cannot start a crawl that would 410 every board. The
taxonomy registry — the one assembled without an HTTP client — SHALL contain `whatjobs`
regardless, carrying an empty publisher id, so the provider's kind, its place in the aggregator
set and its source-facet value do not depend on the local environment.

#### Scenario: Configured environment registers the provider for crawling

- **WHEN** `WHATJOBS_PUBLISHER_ID` is set and the crawl registry is assembled
- **THEN** the registry contains a `whatjobs` adapter carrying that publisher id

#### Scenario: Unconfigured environment omits the provider from the crawl registry

- **WHEN** `WHATJOBS_PUBLISHER_ID` is unset or empty and the crawl registry is assembled
- **THEN** the registry has no `whatjobs` entry, and a board file naming that provider fails
  validation fast

#### Scenario: The taxonomy registry lists the provider without the credential

- **WHEN** `WHATJOBS_PUBLISHER_ID` is unset or empty and the taxonomy registry is assembled
- **THEN** the registry contains a `whatjobs` adapter with an empty publisher id
- **AND** `whatjobs` is classified as an aggregator and is a value in the source facet
