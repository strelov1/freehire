## ADDED Requirements

### Requirement: Geography grouping mappings are exported to the frontend

The system SHALL expose two curated geography mappings to the web client via the
generated contracts: a **country → region** map (the inverse of the location
dictionary's region grouping) and a **city → country** map associating each beacon
city the parser can emit with its ISO 3166-1 alpha-2 country code. Both mappings SHALL
be derived from the existing location dictionary (dictionary-only, never guessed): a
city absent from the beacon set SHALL simply have no country association rather than an
inferred one, and every country present SHALL map to exactly one region from the
controlled region vocabulary. The exports SHALL let the client render a
region → country → city hierarchy without a new API.

#### Scenario: Country to region is exported and exhaustive over the dictionary

- **WHEN** the contracts are generated
- **THEN** a country→region map is emitted in which every country the location
  dictionary groups resolves to exactly one region from the controlled region
  vocabulary

#### Scenario: City to country is exported over the beacon set

- **WHEN** the contracts are generated
- **THEN** a city→country map is emitted associating each beacon city the parser can
  emit with its ISO 3166-1 alpha-2 country code

#### Scenario: An unmapped city has no country association

- **WHEN** a city value is not part of the beacon-city dictionary
- **THEN** the exported city→country map contains no entry for it (no guessed country)
