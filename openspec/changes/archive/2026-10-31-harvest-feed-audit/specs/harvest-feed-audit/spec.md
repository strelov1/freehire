## ADDED Requirements

### Requirement: Audit known boards without writes

The command SHALL support a read-only feed audit for Scalis and Teamtailor. Without a seed it SHALL select known catalog boards; with a seed it SHALL audit the supplied boards without requiring a database. It MUST reject apply mode and invalid limits before network or database access.

#### Scenario: Existing board is selected
- **WHEN** an operator audits a board already in the catalog
- **THEN** that board is included rather than filtered as already known

#### Scenario: Explicit seed needs no database
- **WHEN** a seed file is provided
- **THEN** only its deduplicated boards are audited and no database is opened

### Requirement: Validate feed structure and canonical identity

The audit SHALL verify actual XML structure and preserve native IDs. Scalis location rows SHALL be grouped by canonical vacancy URL. Teamtailor RSS SHALL paginate with bounded termination, treating failed or repeated pages as unverified.

#### Scenario: Multiple locations share a Scalis vacancy
- **WHEN** two rows link to one native vacancy with location-suffixed references
- **THEN** the report counts two rows and one job under its original ID

#### Scenario: Feed walk fails after a successful page
- **WHEN** a later RSS page fails or repeats IDs
- **THEN** the audit reports failure rather than a complete partial inventory

### Requirement: Compare evidence without automatic migration

The audit SHALL emit JSON lines with feed and adapter request counts, canonical ID differences, normalized body differences and per-field availability/loss counts. It MUST distinguish unavailable feeds from empty feeds and failed comparisons. It MUST NOT change source identities or select a feed for ingestion.

#### Scenario: Adapter silently skips a failed detail
- **WHEN** an adapter returns jobs after a transport error
- **THEN** the baseline is unverified and the report does not claim matching coverage

#### Scenario: Feed loses a description or date
- **WHEN** a matched job has a body or publication date in the adapter but not the feed
- **THEN** the report exposes the loss

### Requirement: Bound the audit

The audit SHALL bound selected boards, feed pagination and configurable positive per-board runtime, use provider egress and pace requests. Failures SHALL remain visible in output and process status.

#### Scenario: More boards than the requested limit
- **WHEN** selected boards exceed the positive limit
- **THEN** the command reports the selected and audited counts and audits only the bounded deterministic subset
