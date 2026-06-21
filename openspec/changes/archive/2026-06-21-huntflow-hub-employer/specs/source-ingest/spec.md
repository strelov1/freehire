## ADDED Requirements

### Requirement: A board entry MAY be marked as a community hub whose employer comes from each posting

The system SHALL support an optional `hub` flag on a board-file entry that marks a board-based
provider's board as a community/agency hub: a board that lists vacancies on behalf of many
partner companies rather than a single employer. Unlike a boardless aggregator (one global feed,
no board id), a hub entry still names a board id and still requires a `company` — that configured
company is the hub's own name and the per-vacancy fallback employer. When the flag is absent or
false, the provider's existing behaviour SHALL be unchanged and each job's company SHALL be the
configured entry company.

For a hub entry, the provider SHALL resolve each vacancy's employer from the posting itself
rather than from the configured company, falling back to the configured company when the posting
carries no resolvable employer (for example the hub's own internal roles).

#### Scenario: A non-hub entry attributes every job to the configured company

- **WHEN** a board file lists an entry without the `hub` flag (or with `hub: false`)
- **THEN** each crawled job's company is the entry's configured `company`, exactly as before

#### Scenario: A hub entry attributes each job to the employer named in the posting

- **WHEN** a board file lists an entry with `hub: true`
- **THEN** each crawled job's company is resolved from that vacancy's own payload, and only a
  vacancy with no resolvable employer falls back to the configured `company`

### Requirement: The huntflow adapter resolves a hub board's employer from the vacancy division

For a `huntflow` board marked as a community hub, the adapter SHALL set each vacancy's company
from the vacancy's `division` breadcrumb. Huntflow encodes `division` leaf-first as
`<sub-team> · … · <Company> · Partners · Vacancies`, where the literal `Partners` folder
contains every partner company; the employer is therefore the segment immediately preceding the
`Partners` segment. The adapter SHALL accept either separator Huntflow emits (`·` U+00B7 in the
list payload, `•` U+2022 in the detail payload). When the division has no `Partners` segment, or
no segment precedes it, or the resolved segment is empty, the adapter SHALL fall back to the
configured entry company. A non-hub `huntflow` board's behaviour is unchanged (company is the
configured entry company).

#### Scenario: The employer is the segment before the Partners folder

- **WHEN** a hub vacancy's division is `Mirai · Partners · Vacancies`
- **THEN** the job's company is `Mirai`

#### Scenario: A deeper sub-team does not displace the employer

- **WHEN** a hub vacancy's division is `Remote · Sparkland · Partners · Vacancies`
- **THEN** the job's company is `Sparkland` (the segment before `Partners`), not the deeper
  `Remote` sub-team

#### Scenario: Either separator is accepted

- **WHEN** a hub vacancy's division uses the bullet separator, e.g. `Fluently • Partners • Vacancies`
- **THEN** the job's company is `Fluently`

#### Scenario: A division without a partner structure falls back to the hub company

- **WHEN** a hub vacancy's division is empty or lacks a `Partners` segment
- **THEN** the job's company is the configured entry company (the hub's own name)
