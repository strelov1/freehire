## MODIFIED Requirements

### Requirement: The harvest tool validates candidate boards against the live platform API

The `harvest-boards` host tool SHALL expand a board file (`sources/<provider>.yml`)
only with boards it has live-validated: each candidate board SHALL be probed
against the platform's own live surface — its official public API, or, for a
self-hosted platform that exposes no API, the portal's own served pages — and kept
only if that surface reports at least one open job, so the committed file is the
project's own validated fact set rather than a redistributed dataset. A candidate
that is absent, closed, or unreachable SHALL be skipped, never abort the run. A
kept board SHALL be appended to the provider's board file with the company name
the platform reports (or the board id when the platform exposes none),
de-duplicated against the boards already in the file.

Live jobs alone do not make a board the right board. When the seed names the employer a
candidate is expected to belong to, and the platform reports a company name of its own, the
two SHALL agree — compared after normalizing case, punctuation and legal-form suffixes — or
the candidate SHALL be rejected and counted separately from an unreachable one, so a
mismatch is visible as a mismatch rather than as an absent board. A seed that names no
expected employer SHALL be validated on live jobs alone, and a platform that reports no name
of its own SHALL keep taking the seed's name as its label.

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

#### Scenario: A self-hosted portal is validated against its own pages

- **WHEN** a candidate belongs to a platform that publishes no vendor API, and its
  portal page lists one or more open postings
- **THEN** the candidate is validated from that page and kept exactly as an
  API-validated candidate would be

#### Scenario: A live board owned by a different employer is rejected

- **WHEN** the seed expects a candidate to belong to one employer, and the platform reports
  open jobs under a different company name
- **THEN** the board is not appended, and the run reports it as a name mismatch rather than
  as a skipped or unreachable candidate

#### Scenario: Legal-form suffixes and punctuation do not break agreement

- **WHEN** the seed's expected employer and the platform's reported name differ only in
  case, punctuation, or a legal-form suffix
- **THEN** the names are treated as agreeing and the board is kept

#### Scenario: A seed without an expected employer is unaffected

- **WHEN** a seed entry names no expected employer
- **THEN** the candidate is validated on live jobs alone, exactly as before
