# orphan-company-seed Specification

## Purpose
Deriving a candidate-board worklist from the companies the catalogue holds only through
aggregators: which companies qualify, what candidate board ids each proposes, and the seed
shape handed to the board-harvest tool.

## Requirements
### Requirement: Companies held only through aggregators are the harvest worklist

The `harvest-orphans` host tool SHALL derive its worklist from the catalogue itself: a
company qualifies when it has at least one open posting from a source in the requested
aggregator set and no open posting from any source outside the full aggregator set. A
company whose own ATS is already crawled SHALL therefore never appear in the worklist, and
a company held by two aggregators and no ATS SHALL appear exactly once. The requested
aggregator set SHALL be narrowable to a subset of the aggregators, so a run can target one
segment of the catalogue, while the exclusion test SHALL always consider every aggregator —
a company held by an aggregator outside the requested set is still not ATS-covered, and
narrowing the run must not silently promote it to a first-party source.

#### Scenario: A company held only by aggregators qualifies

- **WHEN** a company's open postings all come from aggregator sources
- **THEN** it appears once in the worklist, with the employer name the catalogue records
  for it

#### Scenario: A company already crawled from its own ATS is excluded

- **WHEN** a company has an open posting from a non-aggregator source
- **THEN** it is absent from the worklist, whatever else holds it

#### Scenario: Narrowing the requested aggregators does not widen the worklist

- **WHEN** a run requests one aggregator, and a candidate company also has open postings
  from a second aggregator outside the requested set
- **THEN** the company still qualifies, because the second source is an aggregator too —
  it is not treated as first-party ATS coverage

### Requirement: Each company proposes name-derived candidate boards

The tool SHALL propose candidate board ids for a company from its name and catalogue slug
alone, never from a fetched website, so discovery does not depend on resolving a domain.
Candidates SHALL be derived by stripping legal-form suffixes from the company name and
rendering the remainder both hyphenated and unseparated, together with the catalogue slug
itself. Candidates SHALL be de-duplicated per company, and a candidate too short to
identify an employer SHALL be dropped — a two-character slug matches an unrelated tenant far
more often than the intended one.

#### Scenario: A multi-word name yields both renderings

- **WHEN** a company is named with several words and a legal-form suffix
- **THEN** the candidates include the hyphenated and the unseparated forms of the name
  without that suffix, and the catalogue slug, each listed once

#### Scenario: A degenerate candidate is dropped

- **WHEN** a derived candidate is shorter than the minimum length
- **THEN** it is not proposed, and the company still contributes its other candidates

### Requirement: The worklist is emitted as a harvest seed

The tool SHALL write its candidates as a single provider-agnostic seed file in the shape the
harvest tool already reads, each entry pairing a candidate board id with the employer name
expected to own it. One seed SHALL serve every provider: the harvest tool de-duplicates
against the provider's own board file and probes what remains, so the seed carries no
provider of its own. The expected employer name SHALL always be present on every entry,
since it is what the harvest tool's corroboration gate tests.

#### Scenario: One seed feeds every provider

- **WHEN** the tool has built candidates for the worklist
- **THEN** it writes one seed file of board/company pairs, with no provider recorded, ready
  to be run against any provider in turn

#### Scenario: Every entry names its expected employer

- **WHEN** a candidate is written to the seed
- **THEN** it carries the employer name from the catalogue, never an empty name
