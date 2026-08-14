## ADDED Requirements

### Requirement: An aggregator posting is not saved when its company already has non-aggregator coverage

The system SHALL, when ingesting a posting from a provider `sources.ProviderKind` classifies
as `KindAggregator`, skip saving that posting when the company (matched by EXACT
`company_slug` string equality — no hyphen-folding; see the "exact match only" scenario
below for why) already has at least one OPEN posting from a source that is NOT in
`sources.AggregatorProviders(sources.Taxonomy())`. A skipped posting SHALL be counted in a
dedicated `Stats.ATSCovered` counter, distinct from `Stats.Rejected`.

#### Scenario: Aggregator posting for a covered company is skipped

- **WHEN** an aggregator-provider board yields a posting for a company that already has an
  open posting from a non-aggregator source
- **THEN** the posting is not saved, and `Stats.ATSCovered` is incremented for that board

#### Scenario: Aggregator posting for an uncovered company is saved normally

- **WHEN** an aggregator-provider board yields a posting for a company with no open posting
  from any non-aggregator source
- **THEN** the posting is saved exactly as it is today, and `Stats.Ingested` is incremented

#### Scenario: A streaming aggregator source is gated the same as a buffered one

- **WHEN** an aggregator provider that fetches via a streaming source (postings emitted one at
  a time rather than as one fetched batch) emits a posting for a covered company
- **THEN** the posting is not saved, and `Stats.ATSCovered` is incremented — the gate applies
  regardless of whether the provider's adapter buffers or streams

#### Scenario: Coverage matches EXACT company_slug only, unlike the reindex suppression pass

- **WHEN** an aggregator posting's `company_slug` and an existing non-aggregator posting's
  `company_slug` differ only by hyphenation (e.g. `cfoinsights` vs `cfo-insights`)
- **THEN** this gate does NOT treat the company as covered and does NOT skip the aggregator
  posting — the live lookup compares `company_slug` values exactly as computed, with no
  folding (a live Meili filter cannot compute the reindex pass's `replace(company_slug, '-',
  '')` fold at query time, and folding the query value instead would break exact matches
  too — see design.md's "Coverage definition"). `aggregator-ats-dedup`'s periodic reindex
  pass remains the mechanism that catches this case, on its own schedule

### Requirement: The coverage gate only evaluates for aggregator-classified providers

The system SHALL NOT apply the coverage gate to a posting from a provider that
`sources.ProviderKind` classifies as `KindATS`, `KindCompany`, or `KindOther`, even when a
coverage lookup is configured.

#### Scenario: An ATS board is unaffected by the coverage gate

- **WHEN** a board file for a `KindATS` provider (e.g. `greenhouse`) is ingested and a
  coverage lookup is configured
- **THEN** every posting that passes the existing catalogue filter is saved as it is today,
  regardless of what the coverage lookup would report

### Requirement: The gate is company-level, not per-posting-title

The system SHALL skip every posting from an aggregator-provider board for a covered company,
regardless of whether that specific posting's title matches any posting from the
non-aggregator source.

#### Scenario: A role the ATS board does not list is still skipped

- **WHEN** a company has an open non-aggregator posting for one role, and the same company's
  aggregator posting is for a different, unrelated role
- **THEN** the aggregator posting is still skipped (not saved), because coverage is decided
  per company, not per title

### Requirement: A missing coverage lookup disables the gate without failing ingest

The system SHALL behave exactly as it does without this change (write every posting that
passes the existing catalogue filter) when no coverage lookup is configured for the run.

#### Scenario: Coverage lookup is not configured

- **WHEN** the ingest `Runner` has no coverage lookup wired (e.g. `MEILI_MASTER_KEY` unset, or
  a test fake that does not implement it)
- **THEN** aggregator postings are saved exactly as they were before this change, and no error
  is raised
