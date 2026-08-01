## ADDED Requirements

### Requirement: Every placeable country code is also resolvable by name

Every ISO 3166-1 alpha-2 country code the dictionary can place in a region SHALL also be
resolvable from at least one country name. The two tables MUST NOT drift apart: a code
that carries a region but has no name is a country the parser can describe once something
else has identified it, yet can never identify itself — so a posting or a CV that spells
the country out in full resolves to nothing.

This invariant SHALL be enforced by a test rather than by review, because the failure is
silent: the affected locations simply return empty geography, which is
indistinguishable from a location the dictionary was never meant to cover.

#### Scenario: A country named in full resolves to its code and region

- **WHEN** a location naming a country in full is parsed (e.g. `San Pedro Sula, Honduras`)
- **THEN** the countries include that country's ISO code and the regions include the region
  the dictionary already assigns to that code

#### Scenario: The two dictionaries are verified to be in step

- **WHEN** the location dictionaries are checked
- **THEN** every country code carrying a region has at least one name resolving to it, and
  a code without one fails the check

#### Scenario: Adding country names does not disturb subdivision resolution

- **WHEN** a location whose token is a two-letter subdivision code that collides with a
  country code is parsed (e.g. `LA` for Louisiana, `MN` for Minnesota)
- **THEN** the subdivision still wins and the country it belongs to is emitted, unchanged
  by the presence of a same-coded country's name in the dictionary
