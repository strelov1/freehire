## MODIFIED Requirements

### Requirement: Salary band insights

The system SHALL expose a public, unauthenticated `GET /api/v1/insights/salary`
endpoint that returns salary distribution bands (at minimum the 25th, 50th, and
75th percentiles) for a role, reported separately per currency and normalized pay
period, computed only from jobs that disclose a single, comparable salary figure.
The endpoint SHALL accept role scoping (category and/or seniority) and geography
scoping, and SHALL respond with the standard envelope.

A posting's salary SHALL be its EFFECTIVE salary: the moderator-set figure when one
exists, else the figure the source states in a structured field, else the figure
enrichment extracted — taking currency, period and amounts from the same tier. The
same resolution SHALL serve every salary rollup, so two salary figures on one page
can never disagree about which salary a posting has.

#### Scenario: Salary bands for a role

- **WHEN** a client requests `GET /api/v1/insights/salary?category=engineering&seniority=senior`
- **THEN** the response is `200` with `data` entries each carrying `currency`,
  `period`, `p25`, `p50`, `p75`, and the contributing `sample_size`

#### Scenario: Currencies never mixed

- **WHEN** jobs in the requested scope disclose salaries in multiple currencies
- **THEN** each currency yields its own band entry and figures from different
  currencies are never combined into one percentile

#### Scenario: Small samples suppressed

- **WHEN** a currency/period band for the requested scope has fewer contributing
  jobs than the configured minimum sample size
- **THEN** that band is omitted rather than returned with an unreliable or
  potentially identifying figure

#### Scenario: A source salary counts without enrichment

- **WHEN** an open posting was never enriched but its source states a structured
  salary
- **THEN** that salary contributes to its role's band

#### Scenario: The manual figure wins

- **WHEN** a posting carries a manual salary, a source salary and an enrichment
  salary that all differ
- **THEN** only the manual figure, in the manual currency and period, contributes
