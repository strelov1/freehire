# job-geography (delta)

## MODIFIED Requirements

### Requirement: Geography output uses controlled vocabularies

Region codes emitted by the parser SHALL be drawn from the same controlled
vocabulary the enrichment contract defines for `regions` — a single, consistent
**macro-region** level: `global`, the macro-regions (`north_america`, `latam`,
`eu`, `uk`, `mena`, `africa`, `apac`), and the post-Soviet `cis` grouping.
Country codes SHALL NOT be emitted as regions: country-level reach lives in the
separate `countries` facet, so the United States maps to the `north_america`
region and Russia (with Belarus, Moldova, the Caucasus, and Central Asia) to the
`cis` region. The parser, the enrichment contract, and the search facet SHALL
share this one set of values. Country codes SHALL be ISO 3166-1 alpha-2. The
`work_mode` hint SHALL be a member of the enrichment contract's `work_mode`
vocabulary (`remote`, `hybrid`, `onsite`) or empty. A value outside these
vocabularies SHALL never be emitted.

#### Scenario: Parser output validates against the controlled vocabularies

- **WHEN** any location string is parsed
- **THEN** every emitted region is a member of the controlled region vocabulary,
  every emitted country is a valid ISO 3166-1 alpha-2 code, and the work_mode is
  a member of the work-mode vocabulary or empty

#### Scenario: The United States maps to the north_america region

- **WHEN** a location resolving to the United States is parsed (e.g. `United
  States`, a `City, ST ZIP` form, or a US state code)
- **THEN** the countries are `[us]` and the regions are `[north_america]` — never
  a `us` region

#### Scenario: Russia and the post-Soviet space map to the cis region

- **WHEN** a location resolving to Russia, Belarus, or a Central Asian republic
  is parsed (e.g. `Москва`, `Минск`, `Remote - Kazakhstan`)
- **THEN** the region is `[cis]` — never a standalone `ru` or `central_asia`
  region — while the country stays its own ISO code
