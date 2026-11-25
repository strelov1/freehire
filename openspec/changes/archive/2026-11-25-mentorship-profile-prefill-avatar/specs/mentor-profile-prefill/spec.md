## Purpose

Gives a candidate who has not yet created a mentor profile a read-only, best-effort
starting point drawn from data they already gave the product elsewhere, so the create
form does not start from nothing.

## ADDED Requirements

### Requirement: Suggestions are composed per field, independently

The system SHALL answer a suggestions request with one candidate value per mentor
profile field, each resolved independently from its own source: display name,
headline, bio and languages from the candidate's résumé; topics from the user
profile's specializations; timezone from the account. A field whose source is absent
or empty SHALL simply be omitted from the response rather than reported as an error or
filled with an empty placeholder.

#### Scenario: A candidate with a full résumé and profile gets every field suggested

- **WHEN** a candidate with a complete résumé, timezone and specializations who has no
  mentor profile requests suggestions
- **THEN** the response carries a suggested name, headline, bio, languages, topics and
  timezone

#### Scenario: A candidate with no usable source data gets an empty, successful answer

- **WHEN** a candidate with no résumé, no specializations and no account timezone
  requests suggestions
- **THEN** the request succeeds
- **AND** the response carries no field suggestions

### Requirement: A company suggestion requires an exact catalog match

The system SHALL suggest a company only when the candidate's current employer, as held
in their experience bank, resolves through the same slug normalization the profile
create flow uses to a company that exists in the company catalog. An employer that
does not resolve to an existing company SHALL be omitted from the response rather than
guessed at or normalized further.

#### Scenario: A current employer matching the catalog is suggested

- **WHEN** a candidate's experience bank names a current employer whose normalized slug
  matches an existing company
- **THEN** the response suggests that company

#### Scenario: An unmatched employer is omitted, not guessed

- **WHEN** a candidate's experience bank names a current employer whose normalized slug
  matches no existing company
- **THEN** the response carries no company suggestion

### Requirement: Suggestions never read or alter an existing mentor profile

The system SHALL answer the suggestions request the same way regardless of whether the
caller already has a mentor profile — the endpoint composes only résumé, user-profile,
account and experience-bank data, never the mentor profile itself. The response SHALL
NOT be used to alter an existing profile, and carries no side effect: it changes nothing
the caller has already submitted. A caller who already has a profile gains nothing by
calling it — the create form is the only caller, and it does so only before a profile
exists — but the endpoint refuses no one and needs no mentor-profile lookup to answer.

#### Scenario: An existing mentor's suggestions request has no effect on their profile

- **WHEN** a candidate who already has a mentor profile requests suggestions
- **THEN** the request answers with the same best-effort suggestions any caller would get
- **AND** their existing mentor profile is not read, not altered, and not referenced in
  the response
