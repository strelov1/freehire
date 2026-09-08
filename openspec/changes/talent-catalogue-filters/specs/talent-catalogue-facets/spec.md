## ADDED Requirements

### Requirement: The catalogue reports how many members stand behind each filter value

An unauthenticated endpoint SHALL report, for the filter currently applied, the number of
members behind every value of every facet the catalogue reads, plus the total behind the
filter itself.

The response SHALL use the same shape the job-search facet endpoint returns, so a control
rendering one can render the other without learning a second vocabulary.

#### Scenario: Counts for an unfiltered catalogue

- **WHEN** the facets are requested with no filter
- **THEN** every facet value present in the catalogue is reported with the number of members
  carrying it
- **AND** the total equals the number of members the list returns for the same request

#### Scenario: Counts under a filter

- **WHEN** the facets are requested with a seniority filter applied
- **THEN** each remaining value's count is the number of members matching BOTH that value
  and the seniority filter

#### Scenario: A value nobody carries

- **WHEN** no member in the catalogue carries a value
- **THEN** that value is absent from the counts rather than reported as zero

### Requirement: A facet's own selection does not collapse its counts

The counts for a facet SHALL be computed with that facet's own selection removed, while
every other filter still applies.

Without this, selecting one value makes every other value in the same pane read zero, and a
visitor can never add a second — the control silently becomes single-select.

#### Scenario: Adding a second skill

- **WHEN** a visitor has selected one skill and opens the skills pane
- **THEN** other skills still show the number of members who carry them alongside the
  current filter's other constraints
- **AND** selecting a second skill widens the result rather than emptying it

#### Scenario: A different facet still narrows

- **WHEN** a visitor has selected a skill and a seniority
- **THEN** the skill counts reflect the seniority filter
- **AND** the seniority counts reflect the skill filter

### Requirement: Counts describe the current filter, never the whole membership

The endpoint SHALL report counts only for the filter it was given. It SHALL NOT expose a
global distribution of the membership.

A count over a catalogue of people is a disclosure in a way a count over millions of
postings is not. What protects a member is the projection — their card carries no name and
no employer — but a published histogram of the whole membership would be an invitation to
walk the space, which serves nobody the catalogue is for.

#### Scenario: A count that would name one person

- **WHEN** a filter narrows to a single member
- **THEN** the facet value is still offered, so the filter remains usable
- **AND** its number is withheld rather than reported as one

#### Scenario: The list and the counts agree

- **WHEN** the same query is sent to the list and to the facets
- **THEN** the total they report is the same number

### Requirement: The facets endpoint is bounded like the catalogue it describes

The endpoint SHALL be unauthenticated and SHALL draw on the catalogue's own rate-limit
budget rather than the shared public-read one.

A filtering visitor makes two requests where a reader makes one, and the budget that bounds
scraping the catalogue is the budget that should bound scraping its shape.

#### Scenario: Exhausting the budget

- **WHEN** a caller exceeds the catalogue's rate limit through the facets endpoint
- **THEN** the excess requests are refused with 429
- **AND** the reads the rest of the site depends on are unaffected
