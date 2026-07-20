## MODIFIED Requirements

### Requirement: Repost clustering counts distinct postings of one role
The system SHALL count how many distinct `external_id`s share one job's `role_fingerprint` within the same company, and expose that count as repost evidence. Because the `role_fingerprint` ignores a location-bearing title suffix, per-city variants of one role share a fingerprint and count together — the repost/mass-posting count reflects the true number of concurrent city postings of that role, not one per city.

#### Scenario: Repeated reposts under new ids are counted
- **WHEN** a company has published six distinct `external_id`s that share one role fingerprint
- **THEN** each such job's evidence reports a repost count of six

#### Scenario: Per-city variants of one role count together
- **WHEN** a company posts one role in several cities, each appending the city to the title, with identical descriptions
- **THEN** the postings share one `role_fingerprint` and each reports a repost count equal to the number of city postings

#### Scenario: A unique role has no repost signal
- **WHEN** a job's role fingerprint is shared by no other posting in the company
- **THEN** its repost count is one and contributes no evergreen signal
