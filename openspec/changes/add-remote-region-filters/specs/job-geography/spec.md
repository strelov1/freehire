## MODIFIED Requirements

### Requirement: Job geography is derived deterministically from the location string

The system SHALL provide a deterministic parser that maps a job's free-text
`location` string to a set of ISO 3166-1 alpha-2 country codes and a set of
region codes. The parser SHALL tokenize the location on the separators `,`, `;`,
`/`, `|`, ` - `, and ` or `, and resolve each token against curated dictionaries:
country/city/shorthand names to country codes, macro-region names to region
codes, and country codes to their region. It SHALL emit only values present in
the controlled vocabularies (see below), deduplicated, and SHALL emit nothing for
tokens it cannot resolve (it never guesses). A bare remote marker (e.g. `Remote`)
with no geographic token SHALL yield empty geography; the `global` region SHALL be
emitted only from an explicit open-anywhere marker (e.g. `Anywhere`, `Worldwide`,
`Global`, `International`), never inferred from a bare `Remote`. The open-anywhere
marker set SHALL include `International` and its close worldwide synonyms.

The parser SHALL also derive a `work_mode` hint from an explicit marker in the
location string — `remote`, `hybrid`, or `onsite` — checked in priority order
hybrid > remote > onsite (the most specific arrangement wins when several markers
co-occur). A location with no work-mode marker SHALL yield an empty work_mode (a
bare city is never assumed to be onsite). The marker scan is independent of the
geography tokens, so a bare `Remote` yields `work_mode=remote` with empty geography.

#### Scenario: A named country yields its code and region

- **WHEN** the location `Remote - Germany` is parsed
- **THEN** the countries are `[de]` and the regions include `eu`

#### Scenario: A bare remote marker yields a work mode but no geography

- **WHEN** the location `Remote` is parsed
- **THEN** the work_mode is `remote` and both countries and regions are empty

#### Scenario: Work mode marker priority

- **WHEN** a location names both a hybrid and a remote marker (e.g.
  `Hybrid / Remote - London`)
- **THEN** the work_mode is `hybrid`

#### Scenario: A macro region name yields a region without a country

- **WHEN** the location `Remote - Europe` is parsed
- **THEN** the regions are `[eu]` and the countries are empty

#### Scenario: Multiple locations union into the result

- **WHEN** the location `Remote - UK or Europe` is parsed
- **THEN** the countries are `[gb]` and the regions include both `uk` and `eu`

#### Scenario: A bare remote marker yields no geography

- **WHEN** the location `Remote` is parsed
- **THEN** both countries and regions are empty

#### Scenario: An explicit open-anywhere marker yields global

- **WHEN** the location `Remote - Anywhere` is parsed
- **THEN** the regions are `[global]`

#### Scenario: The International marker yields global

- **WHEN** the location `Remote - International` (or a close worldwide synonym) is
  parsed
- **THEN** the regions are `[global]` and the work_mode is `remote`

#### Scenario: An unresolvable location yields no geography

- **WHEN** the location is a token absent from every dictionary
- **THEN** both countries and regions are empty rather than a guessed value
