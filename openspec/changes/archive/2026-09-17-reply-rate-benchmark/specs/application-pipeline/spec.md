## ADDED Requirements

### Requirement: Personal reply-rate benchmark on the pipeline endpoint

`GET /api/v1/me/tracking/pipeline` SHALL, for a caller with at least one connected mailbox,
compute the caller's own observable/answered application counts using the same definitions
`company-hiring-signal` uses (observable = the application belongs to a user with a connected
mailbox; answered = a non-retracted `employer_reply` event exists for it) and include them
alongside the global response rate `company-hiring-signal` publishes, gated by the same
ten-application sample floor applied to the caller's own count.

The field SHALL be absent — never zero, never an estimate — when any of the following holds: the
caller has no connected mailbox, the caller's own observable count is under ten, or the global
response rate itself is not currently available. A personal rate with nothing to compare it
against is not the benchmark this requirement serves.

#### Scenario: Caller clears both gates

- **WHEN** the caller has fifteen observable applications, six of them answered, and the global
  response rate is available
- **THEN** the response includes both the caller's own rate and the global rate

#### Scenario: Caller below their own sample gate

- **WHEN** the caller has four observable applications
- **THEN** the reply-rate field is absent, regardless of whether the global rate is available

#### Scenario: Caller has no connected mailbox

- **WHEN** the caller has twenty applied jobs but no connected mailbox
- **THEN** the reply-rate field is absent

#### Scenario: Global rate not yet available

- **WHEN** the caller clears their own sample gate but the global response rate has not yet been
  published (e.g. before the rollup's first run after this feature ships)
- **THEN** the reply-rate field is absent

## MODIFIED Requirements

### Requirement: Pipeline tab in the tracking section

The tracking section SHALL offer a **Pipeline** tab at `/my/tracking/pipeline` alongside the Board, List and Calendar tabs, with Board remaining the default. The Pipeline tab SHALL render the application distribution as a single-level Sankey diagram of the four groups, each band carrying a per-stage breakdown so a settled group shows what settled it, together with Interview Rate and Offer Rate donut cards, using hand-built SVG with no new frontend dependency. The group labels SHALL be the generated ones, identical to the board's column labels. The tab SHALL be available only to signed-in users, inheriting the section's existing authentication gating. When the pipeline endpoint's personal reply-rate benchmark field is present, the tab SHALL also render a reply-rate comparison card (the caller's own rate beside the global rate); when the field is absent, the tab SHALL render exactly as before, with no partial or placeholder card.

#### Scenario: Signed-in user opens the Pipeline tab

- **WHEN** a signed-in user opens `/my/tracking/pipeline`
- **THEN** they see the Sankey diagram and the Interview Rate / Offer Rate donut cards

#### Scenario: A settled group shows what settled it

- **WHEN** the Closed group contains a mix of accepted, rejected and withdrawn applications
- **THEN** the band's breakdown shows each outcome's share of that group

#### Scenario: Group labels match the board

- **WHEN** the Pipeline tab and the Board tab render the same account's data
- **THEN** both use identical group labels

#### Scenario: Empty state

- **WHEN** the caller has no tracked applications
- **THEN** the tab renders its empty state, with no reply-rate card

#### Scenario: Default tab unchanged

- **WHEN** a signed-in user opens the tracking section
- **THEN** the Board tab is still shown by default

#### Scenario: Reply-rate card renders when the benchmark is available

- **WHEN** the pipeline response carries the personal reply-rate benchmark field
- **THEN** the Pipeline tab shows the comparison card beside the existing donut cards

#### Scenario: No reply-rate card when the benchmark is absent

- **WHEN** the pipeline response carries no personal reply-rate benchmark field
- **THEN** the Pipeline tab renders exactly as it did before this change, with no card and no
  loading placeholder in its place
