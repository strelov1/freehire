## ADDED Requirements

### Requirement: A resolved city emits its city facet value without guessing geography

The location parser SHALL resolve a city token against the generated city dictionary
(see the `city-dictionary` capability) and emit the city's canonical display name to
the `cities` output. The dictionary SHALL supply the city facet value ONLY — it SHALL
NOT contribute a country or region of its own, so an ambiguous city name (a spelling
shared across countries, e.g. `Birmingham`) can never guess a geography. Country and
region SHALL come solely from the curated deterministic dictionaries (country/region
names, ISO codes, and US/Canada subdivisions), preserving the parser's "never guesses"
contract; a city that is also a curated country signal (`São Paulo`) therefore still
resolves its country from the curated map. When a curated token already fixed the
country, the city name SHALL be emitted only if the dictionary agrees on that country,
so a country/region token (`USA`) never emits an unrelated city buried in its GeoNames
alternate names. The resolution SHALL cooperate with the existing separator
tokenization, work-mode stripping, Russian city-marker stripping, and dash-export
handling, so an embedded city ("São Paulo, Brazil", "г Москва") still resolves.

#### Scenario: A curated city resolves facet and geography

- **WHEN** the location `Florianópolis` is parsed
- **THEN** the `cities` output includes `Florianópolis`, the countries include `br`,
  and the regions include `latam` (the country comes from the curated dictionary)

#### Scenario: A long-tail city emits its facet name without guessing a country

- **WHEN** the location `Recife` (a city absent from the curated country dictionary) is
  parsed with no other geography token
- **THEN** the `cities` output includes `Recife` while the countries and regions are
  empty (never a guessed value); an explicit `Recife, Brazil` resolves `br`/`latam`

#### Scenario: A country/region token never emits an unrelated city

- **WHEN** the location `USA` is parsed (whose GeoNames alternate names attach it to an
  unrelated foreign city)
- **THEN** the countries are `[us]` and the `cities` output is empty

#### Scenario: An unresolved city emits nothing

- **WHEN** the location names a place absent from the generated dictionary and the
  curated overrides
- **THEN** the `cities`, countries, and regions outputs are all empty rather than a
  guessed value
