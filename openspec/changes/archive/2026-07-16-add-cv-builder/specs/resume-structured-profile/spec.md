## MODIFIED Requirements

### Requirement: The structured résumé is a typed, sanitized contract

The structured résumé SHALL be a typed value covering the candidate's contact basics, a professional summary, work-experience entries (title, company, location, dates, a one-line context summary, achievement highlights, and per-role technology stack), education entries, languages, links, a flat skills list, portfolio projects (name, link, highlights), and an estimated total years of experience. Before it is persisted or served, the system SHALL sanitize all model output to the contract: every string length is bounded, every array cardinality is capped, the total-years estimate is coerced to a non-negative bounded integer, and empty entries are dropped. The system MUST NOT persist or serve a value outside these bounds, so untrusted CV text cannot inject unbounded or malformed content.

#### Scenario: Out-of-bounds model output is coerced before persistence

- **WHEN** the LLM returns over-long strings, an oversized list of entries, or an implausible years value
- **THEN** the sanitized structured résumé has bounded strings, a capped number of entries, and a coerced years value, and only the sanitized value is persisted and served

#### Scenario: Fields not present in the CV are omitted, not invented

- **WHEN** the CV does not state a field (e.g. no education section)
- **THEN** that part of the structured résumé is empty rather than fabricated

#### Scenario: Rich work-history detail is captured

- **WHEN** a role in the CV lists a location, achievement bullets, and a technology stack, and the CV has a skills section and portfolio projects
- **THEN** the structured résumé captures that role's location, highlights, and stack (alongside title/company/dates), and populates the top-level skills list and projects entries — so a CV seeded from it is complete
