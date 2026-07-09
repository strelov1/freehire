## ADDED Requirements

### Requirement: The city dictionary is generated from GeoNames at build time

The system SHALL derive its city dictionary from the GeoNames `cities15000`
dataset (populated places with population ≥ 15,000) via a build-time generator
(`cmd/gen-cities`), and SHALL embed the result in the binary so that runtime
resolution performs no network access and no live geocoding. Each generated entry
SHALL carry a single canonical English display name, the place's ISO 3166-1
alpha-2 country code, and its lowercased lookup aliases (ASCII name, native name,
and GeoNames alternate names). The generator SHALL be re-runnable and its output
committed to the repository (the standard generated-artifact pattern), so the
build succeeds without invoking it.

#### Scenario: A GeoNames city is present in the generated dictionary

- **WHEN** the dictionary is generated from `cities15000`
- **THEN** `Florianópolis` (population ≥ 15k) has an entry mapping its lowercase
  aliases to canonical name `Florianópolis` and country code `br`

#### Scenario: Runtime resolution is offline

- **WHEN** the location parser resolves a city at runtime
- **THEN** it reads only the embedded dataset and makes no network request

### Requirement: A city alias resolves to its most-populous place

A city alias shared by multiple GeoNames places SHALL resolve to the single
most-populous place, giving one deterministic canonical display name for the
`cities` facet rather than a pick among equals. The entry's country code is used
by the parser only to reject an unrelated city on a country/region token (the
agreement check); the dictionary never contributes a country to the parser's
output, so a most-populous pick can never mis-file an ambiguous bare city under
the wrong country.

#### Scenario: A shared alias resolves to the largest city's display name

- **WHEN** an alias occurs for several GeoNames places of differing population
- **THEN** the generated dictionary keeps only the entry for the most-populous
  place under that alias

### Requirement: City names colliding with common words are excluded

The generator SHALL exclude city aliases that collide with common words or
work-mode/geography tokens the parser already handles (a curated stoplist plus the
existing work-mode and open-anywhere markers), so that ingesting a location never
misfires a city facet from an ordinary word. Excluded aliases SHALL emit no city,
country, or region — the parser keeps its "never guess" bias.

#### Scenario: A common-word alias is not treated as a city

- **WHEN** a GeoNames place name equals a stoplisted common word (e.g. `Of`,
  `Remote`, `Worldwide`)
- **THEN** that alias is absent from the generated dictionary and resolves no
  geography

### Requirement: Curated overrides layer on the generated base

The system SHALL retain a small hand-curated set of city aliases that GeoNames
does not cover or spells differently (ATS shorthands and campus/office names such
as `Cupertino`), applied as explicit overrides on top of the generated base. An
override SHALL win over a generated entry for the same alias.

#### Scenario: A curated alias GeoNames lacks still resolves

- **WHEN** a location names a curated city alias absent from `cities15000`
- **THEN** the parser resolves it to the curated canonical name and country
