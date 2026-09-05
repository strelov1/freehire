## MODIFIED Requirements

### Requirement: The structured résumé is a typed, sanitized contract

The structured résumé SHALL be a typed value covering the candidate's contact basics, a professional summary, work-experience entries (title, company, location, a structured start/end period, a one-line context summary, achievement highlights, and per-role technology stack), education entries, languages, links, a flat skills list, portfolio projects (name, link, highlights), and an estimated total years of experience. A work-experience or education entry's period boundary SHALL be a structured year/(optional month) value plus a boolean marking an ongoing entry, not a free-form string — the system SHALL interpret whatever the CV prints (a bare year, a month and year, or an "ongoing" phrasing) into that structure rather than reproducing the source text verbatim. Before it is persisted or served, the system SHALL sanitize all model output to the contract: every string length is bounded, every array cardinality is capped, every date's year is bounded to a plausible range and its month (when present) to 1–12, the total-years estimate is coerced to a non-negative bounded integer, and empty entries are dropped. The system MUST NOT persist or serve a value outside these bounds, so untrusted CV text cannot inject unbounded or malformed content.

#### Scenario: Out-of-bounds model output is coerced before persistence

- **WHEN** the LLM returns over-long strings, an oversized list of entries, an implausible years value, or a date with an out-of-range year or month
- **THEN** the sanitized structured résumé has bounded strings, a capped number of entries, a coerced years value, and any invalid date cleared rather than persisted as given, and only the sanitized value is persisted and served

#### Scenario: Fields not present in the CV are omitted, not invented

- **WHEN** the CV does not state a field (e.g. no education section)
- **THEN** that part of the structured résumé is empty rather than fabricated

#### Scenario: Rich work-history detail is captured

- **WHEN** a role in the CV lists a location, achievement bullets, and a technology stack, and the CV has a skills section and portfolio projects
- **THEN** the structured résumé captures that role's location, highlights, and stack (alongside title/company/period), and populates the top-level skills list and projects entries — so a CV seeded from it is complete

#### Scenario: A year-only date is captured without a fabricated month

- **WHEN** a CV states only a year for a role's start or end (e.g. "2024"), with no month
- **THEN** the structured résumé's period for that boundary carries the year with no month, rather than defaulting to January or any other assumed month

#### Scenario: An ongoing role is marked current, not given a fabricated end date

- **WHEN** a CV describes a role as ongoing ("Present", "Current", or the role has no end stated)
- **THEN** the structured résumé marks that entry's period as current with no end date, rather than inventing an end year
