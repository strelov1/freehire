# job-geography Specification

## Purpose
TBD - created by archiving change ingest-job-geography. Update Purpose after archive.
## Requirements
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

### Requirement: Geography derivation also reads the job title for a restriction marker

When the `location` string alone resolves to `work_mode=remote` with no country and no
region, the system SHALL additionally scan the job `title` for an explicit geography
restriction marker before falling back to the description. A title-embedded marker (e.g. a
bracketed or parenthetical suffix naming a country, US state grouping, or macro-region, such
as `[Remote-US]` or `(Location - Australia or New Zealand)`) SHALL resolve against the same
curated country/region dictionaries the location parser uses, and SHALL NOT guess: a title
suffix that does not resolve against the dictionaries yields no geography from this step and
falls through to the description-based check. This step SHALL run only when `location` left
geography unpinned — a `location` that already resolves a country or region is never
overridden by the title.

#### Scenario: A title-embedded country suffix resolves a bare-remote posting

- **WHEN** a job's `location` is a bare `Remote` marker (no country, no region) and its
  `title` ends with a bracketed country suffix such as `[Remote-US]`
- **THEN** the derived `countries` include `us` and the `regions` include `north_america`

#### Scenario: A title-embedded macro-region phrase resolves a bare-remote posting

- **WHEN** a job's `location` is a bare `Remote` marker and its `title` contains a
  parenthetical restriction such as `(Location - Australia or New Zealand)`
- **THEN** the derived `countries` include `au` and `nz` (or the derived `regions` include
  `apac`, per the existing macro-region vocabulary)

#### Scenario: A title with no resolvable marker falls through unchanged

- **WHEN** a job's `location` is a bare `Remote` marker and its `title` contains no token the
  curated dictionaries can resolve
- **THEN** this step yields no geography and derivation proceeds to the description-based
  check exactly as before this requirement existed

#### Scenario: A location-resolved country is never overridden by the title

- **WHEN** a job's `location` already resolves to a country or region
- **THEN** the title is not consulted for geography, regardless of its content

### Requirement: Description-based geography restriction recognizes region-scoping prose

When `location` and `title` both leave geography unpinned, the description-based restriction
check SHALL match not only citizenship/work-authorization phrasing but also an explicit,
role/candidate-qualified "based/located/hiring/restricted/remote role within
`<country/region>`" scoping statement (for example: "a fully remote role within New Zealand,
Australia East Coast, or nearby time zones", "candidates based in the United States, Canada,
Argentina, or Brazil"). A match SHALL resolve the named countries/regions against the same
curated dictionaries as the location parser and SHALL NOT guess a geography from prose that
names no resolvable country/region token. The anchor phrases SHALL be role- or
candidate-qualified rather than a bare preposition ("based in"/"located in" alone) — an
unqualified anchor is ambiguous between a role restriction and an unrelated company-HQ
mention ("Our company is based in Berlin, but this role is fully remote and open
worldwide"), and matching the HQ sentence as if it were the role's own restriction is exactly
the kind of mislabeling this capability exists to prevent.

#### Scenario: Region-scoping description prose resolves a bare-remote posting

- **WHEN** a job's `location` and `title` leave geography unpinned, and its `description`
  states the role is "a fully remote role within New Zealand, Australia East Coast, or
  nearby time zones"
- **THEN** the derived `countries` include `nz` — the clean dictionary token — while the
  noisy tokens ("Australia East Coast", "nearby time zones") resolve nothing rather than a
  guess, the same never-guess contract the location parser itself follows

#### Scenario: A multi-country scoping list resolves every named country

- **WHEN** a description states the role is "open to candidates based in the United States,
  Canada, Argentina, or Brazil"
- **THEN** the derived `countries` include `us`, `ca`, `ar`, and `br`

#### Scenario: A company-HQ mention is not mistaken for a role restriction

- **WHEN** a description states "Our company is based in Berlin, Germany, but this role is
  fully remote and open worldwide" — a company-location statement, not a role restriction
- **THEN** this step yields no geography, since the qualifying anchor phrases match only a
  role- or candidate-referring statement, never a bare "based in"/"located in"

#### Scenario: Non-restriction prose is still not mistaken for a signal

- **WHEN** a description mentions a country or region only incidentally (e.g. "we serve
  customers across Europe") with no role/based/located restriction phrasing
- **THEN** this step yields no geography, unchanged from prior behavior

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

### Requirement: Work mode is resolved by precedence across sources

`work_mode` is a scalar, so it SHALL be resolved by precedence, not union. It is
derived at ingest into `jobs.work_mode` from three sources, most authoritative
first: (1) the adapter's STRUCTURED work mode (a workplace-type enum or explicit
remote flag from the ATS), (2) a marker in the parsed **location** string, and
(3) a conservative phrase match in the job **description**. A lower source fills
`work_mode` only when every higher source left it empty; the parser never guesses,
so a description with no clear work-arrangement phrase yields nothing. At read time
the served `work_mode` SHALL be the stored `jobs.work_mode` only; the LLM-derived
`enrichment.work_mode` SHALL NOT override it.

#### Scenario: Structured adapter work mode beats the parser

- **WHEN** an adapter reports a structured `work_mode=hybrid` for a posting whose
  location text would parse as `remote`
- **THEN** the stored `jobs.work_mode` is `hybrid`

#### Scenario: The location marker beats the description

- **WHEN** a job has no structured work mode, a location that parses to `remote`,
  and a description that mentions a hybrid arrangement
- **THEN** the derived `work_mode` is `remote` (location wins; description only fills)

#### Scenario: The description fills when location is silent

- **WHEN** a job has no structured work mode, a location with no work-mode marker
  (e.g. a bare city), and a description stating "this is a fully remote position"
- **THEN** the derived `work_mode` is `remote`

#### Scenario: A noisy description token does not trigger a false positive

- **WHEN** a job's description contains incidental tokens like "distributed
  systems" or "hybrid cloud" but no actual work-arrangement phrase, and no
  structured or location signal
- **THEN** the derived `work_mode` is empty

#### Scenario: The ingest value is served regardless of the LLM

- **WHEN** a job has `jobs.work_mode=onsite` from ingest and
  `enrichment.work_mode=remote` from the LLM, and is read
- **THEN** the resolved top-level `work_mode` is `onsite`

### Requirement: The public job object exposes geography and work mode as a top-level facet

The public job object SHALL expose geography as top-level `regions` and
`countries` fields carrying the deterministic (jobs-column) values, and
`work_mode` as a top-level field carrying the deterministic value, each reported
exactly once. The `enrichment.regions`, `enrichment.countries`, and
`enrichment.work_mode` fields SHALL NOT additionally appear as independent fields
in the served object. The stored `enrichment` JSONB SHALL be left untouched (the
enrichment worker's data is preserved for future discovery use).

#### Scenario: Geography and work mode appear once, at the top level

- **WHEN** a client reads a job whose enrichment contained `regions` and `work_mode`
- **THEN** the returned object carries top-level `regions`/`countries`/`work_mode`
  from the jobs columns and does not separately repeat those fields under
  `enrichment`

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
