## ADDED Requirements

### Requirement: Loxo board discovery from the search footprint

The system SHALL provide a `harvest-loxo` host tool that consumes operator-supplied
Loxo footprint URLs (careers-page and `/job/<base64>` URLs, read from stdin or a
seed file) and extracts the `(host, slug)` pair from each into a distinct candidate
board set. Because Loxo exposes no public directory of agencies, the operator gathers
the footprint (e.g. via a `site:app.loxo.co` search) and the tool validates and
curates it — mirroring `harvest-boards`, which validates a supplied seed set rather
than scraping a search engine itself.

#### Scenario: Candidates extracted from footprint URLs

- **WHEN** the tool reads Loxo careers-page and job-detail URLs across host variants
- **THEN** it derives one candidate `(host, slug)` per distinct agency board

### Requirement: Live validation of candidate boards

The tool SHALL keep a candidate board only if its careers page live-validates —
returns HTTP 200 and contains at least one `/job/<base64>` link. A candidate that
is absent, empty, or unreachable SHALL be skipped and never abort the run.

#### Scenario: A live board with postings is kept

- **WHEN** a candidate careers page returns 200 with one or more job links
- **THEN** the board is kept as a validated candidate

#### Scenario: A dead or empty candidate is skipped

- **WHEN** a candidate careers page is unreachable, non-200, or has no job links
- **THEN** it is skipped and the run continues with the other candidates

### Requirement: Tech-relevance counting

For each validated board the tool SHALL count how many of its postings classify as
tech (via `internal/classify`) and report that count, so an operator can avoid
seeding boards that carry no tech vacancies.

#### Scenario: Tech count reported per board

- **WHEN** a validated board's postings are sampled
- **THEN** the tool reports the board's total and tech-classified job counts

### Requirement: Draft board-file emission for curation

The tool SHALL emit each kept board as a draft `sources/loxo.yml` entry — `company`
from the board title, `board` as `<host>/<slug>`, and `hub: true` — de-duplicated
against boards already in the file. The tool proposes entries for human curation
and does not itself decide the committed board set.

#### Scenario: Kept board emitted as a hub entry

- **WHEN** a board is validated and kept
- **THEN** it is emitted as a `sources/loxo.yml` entry with `hub: true` and its
  `<host>/<slug>` board id

#### Scenario: An already-known board is not duplicated

- **WHEN** a candidate board id already appears in `sources/loxo.yml`
- **THEN** it is filtered out and not emitted again
