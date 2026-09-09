## MODIFIED Requirements

### Requirement: A mentor profile is owned by one account and names one company

The system SHALL hold at most one mentor profile per user account, and that profile
SHALL name exactly one company by `companies.slug` — the same key `jobs.company_slug`
carries. A profile SHALL carry a stable public slug used in its URL, an IANA timezone,
a headline, a biography, a topic list, a language list, and the session parameters
(duration, buffers, minimum notice, booking horizon, meeting link).

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
