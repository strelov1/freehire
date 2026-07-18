## ADDED Requirements

### Requirement: Precomputed per-company open/growth scalar

The rollup SHALL maintain a per-company scalar table
`insights_company_growth(company_slug, open_count, open_count_prev)` to back a
ranked leaderboard without per-request aggregation over the full catalogue, where
`open_count` is the company's current count of open canonical jobs and
`open_count_prev` is that count as of 30 days earlier. Only canonical, attributable
rows SHALL be counted (`company_slug <> '' AND duplicate_of IS NULL`), consistent
with `insights_company_stats` and `companies.job_count`. The 30-day window SHALL be
the same constant the existing `insights_*` rollups use.

The table SHALL be rebuilt by `cmd/rollup-company` in the **same transaction** as
`insights_company_stats`, as an atomic delete-and-reinsert, so a reader never sees
one rebuilt without the other.

#### Scenario: Scalar reflects current and prior open counts

- **WHEN** a company has 10 open canonical jobs now and had 4 open 30 days ago
- **THEN** its `insights_company_growth` row has `open_count = 10` and
  `open_count_prev = 4`

#### Scenario: Only canonical rows counted

- **WHEN** a company has open jobs that are repost copies (`duplicate_of` set) or
  have an empty `company_slug`
- **THEN** those jobs do not contribute to `open_count` or `open_count_prev`

#### Scenario: Rebuilt atomically with the per-day rollup

- **WHEN** `cmd/rollup-company` runs
- **THEN** both `insights_company_stats` and `insights_company_growth` are replaced
  within one transaction, or neither is (a failure leaves both prior tables intact)
