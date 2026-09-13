## RENAMED Requirements

- FROM: `### Requirement: A mentor profile is owned by one account and names one company`
- TO: `### Requirement: A mentor profile is owned by one account and may name a company`

## MODIFIED Requirements

### Requirement: A mentor profile is owned by one account and may name a company

The system SHALL hold at most one mentor profile per user account. A profile MAY name a
company by `companies.slug` — the same key `jobs.company_slug` carries — but is NOT
required to: an independent mentor, or one whose employer is not in the catalogue, MAY
submit with no company at all. A profile SHALL carry a stable public slug used in its
URL, an IANA timezone, a headline, a biography, a topic list, a language list, and the
session parameters (duration, buffers, minimum notice, booking horizon, meeting link).

A submission that names a company SHALL still be checked against the catalogue exactly
as before — only the requirement to name ONE is removed, not the check on a name that IS
supplied.

A submission that supplies no URL slug SHALL NOT be refused for that reason: the system
SHALL derive one from the display name, using the same character rule an explicitly
supplied slug must satisfy, and SHALL resolve a collision on the derived slug with a
numeric suffix rather than refusing the submission. A submission that DOES supply a
slug is unaffected by this: it SHALL be validated and, on a collision, refused, exactly
as before — the automatic suffix retry applies only to a slug the system derived
itself, never to one the mentor chose.

#### Scenario: A second profile for the same account is refused

- **WHEN** an account that already has a mentor profile submits another
- **THEN** the system refuses with a conflict rather than creating a second profile

#### Scenario: A profile naming an unknown company is refused

- **WHEN** a profile is submitted with a `company_slug` no `companies` row carries
- **THEN** the system refuses with a not-found error naming the company
- **AND** no profile row is written

#### Scenario: A profile with no company is accepted

- **WHEN** a profile is submitted with no `company_slug` at all
- **THEN** the system creates the profile with no company
- **AND** the profile is not refused for lacking one

#### Scenario: An empty URL slug is derived from the display name

- **WHEN** a profile is submitted with an empty or whitespace-only URL slug and a
  display name that yields at least one usable character
- **THEN** the system derives a slug from the display name and creates the profile
  with it
- **AND** the response carries the derived slug

#### Scenario: A derived slug that collides gets a numeric suffix

- **WHEN** a profile is submitted with an empty URL slug whose derived form is already
  another profile's slug
- **THEN** the system creates the profile with the smallest free numbered variant of
  that derived slug
- **AND** the submission is not refused for the collision

#### Scenario: A display name with no usable characters still yields a profile

- **WHEN** a profile is submitted with an empty URL slug and a display name that
  sanitizes to nothing (for example, a name with no latin letters or digits)
- **THEN** the system falls back to a fixed base slug, suffixed on collision as usual
- **AND** the submission is not refused

#### Scenario: An explicitly supplied, invalid slug is still refused

- **WHEN** a profile is submitted with a non-empty URL slug that does not match the
  required character rule
- **THEN** the system refuses the submission naming the slug as unusable

#### Scenario: An explicitly supplied slug that collides is still refused

- **WHEN** a profile is submitted with a non-empty URL slug that another profile
  already holds
- **THEN** the system refuses the submission with a conflict
- **AND** it does NOT silently substitute a suffixed variant

### Requirement: The public directory lists approved mentors and can be narrowed

The system SHALL serve a public directory of approved, unpaused mentor profiles, and
SHALL allow it to be narrowed by company, by topic, by language, by seniority, by a
free-text match against a mentor's name or headline, and to mentors with no reviews yet.
Following the project's dropped-filter rule, the directory SHALL report any query
parameter it did not read in `meta.ignored_params`, and SHALL omit that key when there
are none.

A mentor with no company SHALL appear in the unfiltered directory like any other, and
SHALL never match a directory search narrowed by company — the same "unset never
matches a filter" rule seniority already follows.

#### Scenario: The directory excludes profiles that are not publishable

- **WHEN** a visitor lists the mentor directory
- **THEN** only profiles whose status is `approved` and which are not paused appear

#### Scenario: An unrecognised filter is reported, not silently ignored

- **WHEN** a visitor requests the directory with a parameter the endpoint does not read
- **THEN** the results are unnarrowed by it
- **AND** `meta.ignored_params` names that parameter

#### Scenario: A free-text search matches name or headline

- **WHEN** a visitor narrows the directory with a text query
- **THEN** only mentors whose name or headline contains that text, case-insensitively,
  appear

#### Scenario: Narrowing by seniority

- **WHEN** a visitor narrows the directory by a seniority value
- **THEN** only mentors who stated that exact seniority appear
- **AND** a mentor who left seniority unset does not appear

#### Scenario: Narrowing to mentors with no reviews yet

- **WHEN** a visitor narrows the directory to mentors with no reviews yet
- **THEN** only mentors whose review count is zero appear

#### Scenario: A company-less mentor appears in the unfiltered directory

- **WHEN** a visitor lists the mentor directory with no company filter
- **THEN** an approved, unpaused mentor with no company appears alongside the rest

#### Scenario: A company-less mentor never matches a company filter

- **WHEN** a visitor narrows the directory by any company
- **THEN** a mentor with no company does not appear in the results

### Requirement: A vacancy and a company page lead to their mentors

Where the catalogue holds an approved mentor for a company, the system SHALL expose
that fact on that company's vacancies and on the company page, so a seeker reading a
posting can reach a mentor at that employer.

The answer SHALL come from the directory narrowed to that company rather than from a
separate "has a mentor?" endpoint. A second way to ask means a second copy of the
publication predicate, and two copies of a predicate drift.

A mentor with no company SHALL NOT be attributed to any company's vacancy or company
page — this follows directly from the directory's company filter never matching a
company-less mentor, so no separate exclusion is needed.

#### Scenario: A vacancy at a company with a mentor offers the entry point

- **WHEN** a visitor opens a vacancy whose `company_slug` has at least one approved,
  unpaused mentor
- **THEN** the page offers a route to that company's mentors

#### Scenario: A vacancy at a company without mentors offers nothing

- **WHEN** a visitor opens a vacancy whose company has no approved, unpaused mentor
- **THEN** no mentorship entry point is rendered

#### Scenario: A company-less mentor is never offered as a company's entry point

- **WHEN** a visitor opens a vacancy or company page for any company
- **THEN** a mentor with no company is never offered as that company's mentorship
  entry point
