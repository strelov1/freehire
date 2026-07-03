## ADDED Requirements

### Requirement: The country→region grouping is exported to the frontend

The system SHALL expose the curated **country → region** map (the inverse of the
location dictionary's region grouping) to the web client via the generated contracts,
so the client can group countries under their macro-region in the location filter
without a new API. The map SHALL be derived from the existing location dictionary
(dictionary-only, never guessed): every country the dictionary groups SHALL map to
exactly one region from the controlled region vocabulary.

#### Scenario: Country to region is exported and exhaustive over the dictionary

- **WHEN** the contracts are generated
- **THEN** a country→region map is emitted in which every country the location
  dictionary groups resolves to exactly one region from the controlled region
  vocabulary
