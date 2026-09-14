## Purpose

Converts an external `name,slug,url` ATS-company inventory CSV into per-provider seed files
that `cmd/harvest-boards` can consume directly, using freehire's own URL-recognition rules
rather than a redistributed dataset's own identifiers.

## ADDED Requirements

### Requirement: A recognized row is written to its provider's seed file
For each CSV data row whose `url` column resolves to a `(provider, board)` pair via the
project's existing URL-recognition rules, the tool SHALL include that board — with the CSV
row's `name` column as its company label — in the output seed file for that provider.

#### Scenario: A recognized board is added to its provider's seed file

- **WHEN** a CSV row's `url` resolves to provider `greenhouse` and board `acme`
- **THEN** the output seed file for `greenhouse` contains an entry with board `acme` and
  company set to that row's `name` column

### Requirement: An unrecognized row is skipped, never fatal
A CSV row whose `url` does not resolve to any known provider (a vanity domain, or a
custom-domain ATS not recognizable by URL alone) SHALL be omitted from every output seed
file and counted in the run's summary. It SHALL NOT stop the run or cause any other row to
be dropped.

#### Scenario: An unrecognized URL is skipped

- **WHEN** a CSV row's `url` is a company's own vanity domain that no known ATS pattern
  matches
- **THEN** the row does not appear in any output seed file, the run's summary counts it as
  unrecognized, and every other row is still processed

### Requirement: Rows are grouped into one seed file per provider
When the input CSV's rows resolve to more than one distinct provider, the tool SHALL write
one output seed file per distinct provider, and each file SHALL contain only the boards
resolved to that provider.

#### Scenario: Two providers in one input produce two seed files

- **WHEN** the input CSV contains rows that resolve to both `greenhouse` and `lever`
- **THEN** the tool writes a separate seed file for `greenhouse` and a separate one for
  `lever`, each containing only its own provider's boards

### Requirement: Output matches the seed shape `cmd/harvest-boards` already consumes
Each output seed file SHALL be a JSON array of objects with a `board` field and a `company`
field, matching the object shape `cmd/harvest-boards` already accepts, so it can be passed
to that tool unmodified.

#### Scenario: A seed file loads directly into harvest-boards

- **WHEN** an output seed file for provider `lever` is passed as the seed argument to
  `cmd/harvest-boards lever <file>`
- **THEN** `cmd/harvest-boards` parses it without error and treats each entry as one
  candidate board with its company label

### Requirement: Duplicate boards collapse to a single entry
When two or more input rows resolve to the same `(provider, board)` pair, the corresponding
output seed file SHALL contain that board exactly once.

#### Scenario: The same board listed twice in the input appears once in the output

- **WHEN** two CSV rows have different `url` values that both resolve to provider `ashby`,
  board `openai`
- **THEN** the output seed file for `ashby` contains exactly one entry for board `openai`

### Requirement: The conversion runs without a database or network access
The tool SHALL perform the entire conversion from the local CSV input alone. It SHALL NOT
require a database connection or make any network request to complete successfully.

#### Scenario: The tool completes with no database and no network reachable

- **WHEN** the tool is run against a valid input CSV with no `DATABASE_URL` set and no
  network connectivity available
- **THEN** it completes successfully and writes the expected output seed files

### Requirement: A structurally invalid input file fails the run
When the input file cannot be parsed as CSV, or is missing one of the required `name`,
`slug`, `url` columns, the tool SHALL exit with a non-zero status and an error identifying
the problem, and SHALL NOT write any output seed file.

#### Scenario: A CSV missing the url column is rejected

- **WHEN** the input file's header row does not include a `url` column
- **THEN** the tool exits non-zero, reports the missing column, and writes no output files
