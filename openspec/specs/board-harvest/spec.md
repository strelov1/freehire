# board-harvest Specification

## Purpose
TBD - created by archiving change gupy-board-discovery. Update Purpose after archive.
## Requirements
### Requirement: The harvest tool validates candidate boards against the live platform API

The `harvest-boards` host tool SHALL expand a board file (`sources/<provider>.yml`)
only with boards it has live-validated: each candidate board SHALL be probed
against the platform's official public API and kept only if the API reports at
least one open job, so the committed file is the project's own validated fact set
rather than a redistributed dataset. A candidate that is absent, closed, or
unreachable SHALL be skipped, never abort the run. A kept board SHALL be appended
to the provider's board file with the company name the platform reports (or the
board id when the platform exposes none), de-duplicated against the boards already
in the file.

#### Scenario: A candidate with open jobs is kept

- **WHEN** a candidate board is probed and the platform API reports one or more
  open jobs
- **THEN** the board is appended to `sources/<provider>.yml` with the reported
  company name (or the board id as a fallback)

#### Scenario: A candidate with no open jobs is skipped

- **WHEN** a candidate board is probed and the platform API reports zero jobs or
  is unreachable
- **THEN** the board is not appended and the run continues with the other
  candidates

#### Scenario: An already-known board is not duplicated

- **WHEN** a candidate board id already appears in `sources/<provider>.yml`
- **THEN** it is filtered out before probing and not appended again

### Requirement: A provider may supply its own candidate boards by discovery

The harvest tool SHALL allow a provider whose boards are not available as a seed
list to discover its candidate boards from the platform API, by implementing an
opt-in discovery capability. When a provider supports discovery and the tool is
run with no seed file, the tool SHALL obtain the candidate boards from discovery
instead of from a seed list; every discovered candidate SHALL then pass through
the same live-validation, de-duplication, and append steps as a seeded candidate.
A provider that does not support discovery SHALL continue to require a seed file.

#### Scenario: Discovery supplies candidates when no seed is given

- **WHEN** the tool is run for a provider that supports discovery and no seed file
  is given
- **THEN** the candidate boards come from the provider's discovery, and each is
  live-validated, de-duplicated, and appended exactly as a seeded candidate would be

#### Scenario: A provider without discovery still needs a seed

- **WHEN** the tool is run for a provider that does not support discovery and no
  seed file is given
- **THEN** the tool reports a usage error and makes no changes

### Requirement: Gupy boards are discovered from the global jobs feed

The harvest tool SHALL support discovering Gupy boards from Gupy's global jobs
feed. Discovery SHALL page the feed (across all companies, not filtered to any job
category) collecting each posting's distinct numeric `companyId`, stopping when a
page returns no postings or a bounded maximum is reached. Each discovered
`companyId` SHALL be validated by querying the company's feed for its open-job
count and its reported career-page name, kept only when it has at least one open
job, with the name falling back to the `companyId` when the feed reports none.

#### Scenario: Gupy discovery collects distinct companies from the feed

- **WHEN** Gupy discovery pages the global jobs feed and the pages name several
  companies, some more than once
- **THEN** each distinct `companyId` is collected once as a candidate board

#### Scenario: Gupy discovery stops at an empty page

- **WHEN** a Gupy feed page returns no postings
- **THEN** discovery stops paging and returns the companies collected so far

#### Scenario: A discovered Gupy company is validated and named

- **WHEN** a discovered `companyId` is probed and its feed reports open jobs and a
  career-page name
- **THEN** the board is kept with that name; a company whose feed reports no name
  falls back to the `companyId`, and one with no open jobs is skipped

