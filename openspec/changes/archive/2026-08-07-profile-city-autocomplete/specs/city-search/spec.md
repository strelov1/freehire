## ADDED Requirements

### Requirement: City search returns population-ranked prefix matches

The system SHALL expose a search over the existing GeoNames-derived city dictionary
(`city-dictionary` capability) that, given a text query, returns the cities whose
canonical name or any known alias starts with that query (case-insensitively), ordered
most-populous first, deduplicated so a city with several matching aliases appears once.
An empty or whitespace-only query SHALL return no results rather than the whole
dictionary.

#### Scenario: A name prefix matches the canonical entry

- **WHEN** the query is `Flor`
- **THEN** the results include `Florianópolis` (country `br`), ranked ahead of any
  less-populous match sharing the same prefix

#### Scenario: An alias prefix reaches the same canonical entry

- **WHEN** the query is a prefix of a known alias rather than the canonical name (for
  example a curated override or an alternate spelling the dictionary records)
- **THEN** the results include that entry's canonical name exactly once

#### Scenario: A blank query returns nothing

- **WHEN** the query is empty or only whitespace
- **THEN** the search returns zero results

### Requirement: City search can be narrowed to one country

City search SHALL accept an optional ISO 3166-1 alpha-2 country code and, when given,
SHALL restrict results to cities whose dictionary entry carries that country code. When
omitted, results are drawn from every country.

#### Scenario: A country filter excludes cities elsewhere

- **WHEN** the query is `San` and the country filter is `us`
- **THEN** every result's country is `us`, even though the unfiltered query would also
  match cities in other countries

### Requirement: City search results are capped

City search SHALL return at most a fixed number of results (20), the most-populous
matches first, regardless of how many dictionary entries match the query.

#### Scenario: A broad query is truncated, not exhaustive

- **WHEN** a query matches more than 20 dictionary entries
- **THEN** exactly 20 results are returned, the 20 most populous among the matches

### Requirement: City search is served over a public, read-only endpoint

The system SHALL expose city search at `GET /geo/cities` accepting `q` (the query) and
an optional `country`, returning `200` with `{"data": [{"value", "country"}, ...]}` where
`value` is the city's canonical name and `country` is its ISO 3166-1 alpha-2 code, for the
caller to render (the endpoint does not compose a human-readable label itself). The
endpoint SHALL require no authentication, consistent with the other public
geography/facet reference endpoints (e.g. company subindustries).

#### Scenario: An anonymous request is served

- **WHEN** an unauthenticated client requests `GET /geo/cities?q=Berl`
- **THEN** the system responds `200` with matching cities, not `401`

#### Scenario: A result carries its country code for disambiguation

- **WHEN** a search for `Springfield` returns entries from more than one country
- **THEN** each result's `country` distinguishes the otherwise-identical `value`s

### Requirement: The profile's base-city control is a single-value city search picker

The profile edit UI's "where you're based" city control SHALL let the user search and
pick a city via city search rather than type free text, narrowed by the base country
when one is selected. Picking a city SHALL replace any previously picked city rather
than adding to a set — `base.city` remains a single value. The value sent on save SHALL
be the picked result's `value` (the bare canonical name), unchanged from what
`search-profiles`' save contract already accepts.

#### Scenario: Picking a city sets the base city

- **WHEN** a signed-in user with base country `br` searches the city control for `Flor`
  and picks `Florianópolis`
- **THEN** the control shows `Florianópolis` as the selected city and saving stores it
  as `base.city`

#### Scenario: The country narrows the city search

- **WHEN** a signed-in user has selected base country `br` and searches the city control
  for `San`
- **THEN** the suggested cities are restricted to Brazil

#### Scenario: Picking a new city replaces the old one

- **WHEN** a signed-in user has already picked a base city and picks a different one from
  a new search
- **THEN** only the newly picked city remains selected

### Requirement: The profile's relocation-cities control is a multi-value city search picker

The profile edit UI's "where you'd relocate" cities control SHALL let the user search
and pick multiple cities via city search rather than type free text and press Enter,
showing each picked city as a removable chip. The search SHALL NOT be narrowed by any
single country, since the set may span multiple destination countries. The values sent
on save SHALL be the picked results' bare canonical names, unchanged from what
`search-profiles`' save contract already accepts.

#### Scenario: Picking a city adds it to the relocation set

- **WHEN** a signed-in user with relocation open searches the relocation-cities control
  for `Berl` and picks `Berlin`
- **THEN** `Berlin` appears as a chip and saving includes it in `relocation.cities`

#### Scenario: Removing a chip removes it from the set

- **WHEN** a signed-in user removes a previously picked relocation-city chip
- **THEN** that city is absent from `relocation.cities` on the next save

### Requirement: An unrecognized saved city still displays

When a profile's saved `base.city` or a `relocation.cities` entry does not match any
city-search result (for example, free text saved before this capability existed, or a
place absent from the dictionary), the control SHALL still display that saved value
rather than showing it as blank or silently dropping it, and SHALL preserve it on save
if the user does not change it.

#### Scenario: A pre-existing free-text city is preserved and shown

- **WHEN** a signed-in user's profile already has `base.city` set to a value the city
  dictionary does not recognize
- **THEN** opening the profile editor shows that value as the selected city, and saving
  without changing it keeps it unchanged
