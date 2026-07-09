## ADDED Requirements

### Requirement: A resolved city emits both its city facet value and its country/region

The location parser SHALL resolve a city token against the generated city
dictionary (see the `city-dictionary` capability), and a resolved city SHALL
contribute **both** its canonical display name to the `cities` output **and** its
ISO 3166-1 alpha-2 country code (and thereby its region) — from the single
generated source, closing the prior divergence where a city could resolve a
country without emitting a city facet value. A city the dictionary cannot resolve
SHALL emit no city, country, or region (the parser never guesses). The generated
resolution SHALL cooperate with the existing separator tokenization, work-mode
stripping, Russian city-marker stripping, and dash-export handling, so an embedded
city ("Florianópolis, Brazil", "г Москва") still resolves.

#### Scenario: A city resolves both facet and geography

- **WHEN** the location `Florianópolis` is parsed
- **THEN** the `cities` output includes `Florianópolis`, the countries include
  `br`, and the regions include `latam`

#### Scenario: A city with an embedded country still resolves the city facet

- **WHEN** the location `Florianópolis, Brazil` is parsed
- **THEN** the `cities` output includes `Florianópolis` and the countries include
  `br`

#### Scenario: An unresolved city emits nothing

- **WHEN** the location names a place absent from the generated dictionary and the
  curated overrides
- **THEN** the `cities`, countries, and regions outputs are all empty rather than a
  guessed value
