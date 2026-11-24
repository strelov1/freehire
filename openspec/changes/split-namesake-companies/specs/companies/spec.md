## ADDED Requirements

### Requirement: A board's employer domain keeps namesakes apart

The system SHALL record, per job board, the domain of the board's employer when the ATS or a
curator states it. A board whose postings a moderator has decided belong to a different
employer than their name suggests SHALL be resolved to that employer's slug on every later
crawl, before name-based resolution. A lookup by domain SHALL answer the company whose
boards name that domain before a company whose company information merely states it.

#### Scenario: A domain lookup reaches the employer that owns the domain

- **WHEN** the Ashby board `exa` names `exa.ai` and its postings are stored under `exa-labs`,
  while the namesake slug `exa` states `exa.ai` in its company information
- **THEN** `GET /companies/by-domain?domain=exa.ai` answers `exa-labs`

#### Scenario: A split board stays split

- **WHEN** a moderator approves that the Greenhouse board `motive` is a different employer
  from the company `motive`
- **THEN** the board's open postings move to a new, frozen slug, and the next crawl of that
  board writes its postings under the new slug, not under `motive`

#### Scenario: A website contradicting the board's domain is not written

- **WHEN** a company-info writer carries the website `gomotive.com` for the slug `motive`,
  whose only boards name `wearemotive.com`
- **THEN** the website is not written onto `motive`
