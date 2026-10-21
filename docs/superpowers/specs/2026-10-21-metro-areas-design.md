# Metro areas in the location filter

## Problem

A user searching for work around Sacramento has to pick Sacramento, Folsom, Roseville,
Elk Grove and Rancho Cordova one by one, and has to know those towns exist. The cities
facet cannot be expanded into a metro at query time either: it stores a bare city NAME,
so "Roseville" is Roseville CA, MI and MN at once, and a name list for "Sacramento"
would pull in Detroit's suburbs.

The request came from a user (Joe Taormina, 2026-10-19) and is not US-specific: the
same need exists for Moscow, São Paulo, Campinas and every other agglomeration.

## Decision

Derive a `metros` facet per posting at ingest, from one global source, and filter on it.

### Source: GHS-FUA R2019A

The European Commission JRC's Functional Urban Areas (GHS-FUA, `GHS_FUA_UCDB2015_GLOBE_R2019A`,
CC BY 4.0): 9,031 commuting-zone polygons covering the whole world (USA 260, Brazil 301,
Russia 175), GeoPackage, World Mollweide (EPSG:54009), 1 km resolution, epoch 2015.

Chosen over per-country official lists (US CBSA, Canada CMA, Eurostat FUA, IBGE) because
it is ONE method for every country, and Russia has no official list with a membership
table at all. Measured against the file before choosing (point-in-polygon on GeoNames
coordinates):

| Place | FUA |
|---|---|
| Folsom, Roseville CA, Elk Grove, Rancho Cordova | Sacramento |
| Roseville MI | Detroit |
| Khimki, Podolsk | Moscow |
| Hortolândia, Sumaré | Campinas |
| Davis CA | Davis (its own FUA) |
| Zelenograd | Zelenograd (its own FUA) |
| Oakland, Cupertino | "San Jose" (the whole Bay Area) |

Known weaknesses, and what handles each:

- **Source names are sometimes wrong for a user.** The Bay Area is named "San Jose";
  the San Diego side of the border FUA is named "Tijuana". Handled by the naming rule
  plus a small override map (below).
- **Commuting boundaries differ from administrative ones.** Davis is not in Sacramento,
  Zelenograd is not in Moscow. Accepted: the data measures where people commute, which
  is what a job search asks. The override map can merge a FUA into another when a case
  proves wrong in practice; none is merged up front.
- **2015 epoch.** Agglomerations move slowly; accepted.

### Generator: `cmd/gen-metros`

A run-by-hand generator, modelled on `cmd/gen-cities`, whose output is committed so the
build never depends on it. Pure Go: GeoPackage is SQLite, geometry is GPKG-wrapped WKB,
and the Mollweide forward projection plus ray-casting point-in-polygon are a few dozen
lines. No GDAL.

Inputs: the GHS-FUA GeoPackage and the GeoNames `cities1000` dump (the same dump
`gen-cities` reads, adding latitude, longitude and admin1 columns).

For every GeoNames place: project its coordinates, find the FUA containing it.

**Metro identity is (FUA, country)**, not the FUA alone: a cross-border FUA becomes one
metro per country, so a US posting in San Diego is never labelled with a Mexican city.

**Metro name** = the most populous GeoNames place of that country inside the FUA, unless
`metroOverrides` names it (e.g. the Bay Area → "San Francisco Bay Area"). The override map
is the only hand-curated part and argues each entry beside it.

**Metro value** (the facet value, the URL value) = `<name> (<CC>)`, e.g. `Sacramento (US)`,
`Moscow (RU)`, `Campinas (BR)` — readable, like the cities facet, so the UI needs no lookup
table. The value carries no comma on purpose: the web client joins a facet's values with commas in the URL. The country suffix keeps same-named metros in different countries apart (Valencia
ES/VE). Two metros of one country sharing a name get the admin1 code appended by the
generator (`Springfield MA (US)`), which fails loudly if that still collides. Display
label: "<name> area".

Output: `internal/dict/location/metros.tsv`, one row per place:
`canonical city name (lowercase) · country · admin1 · metro value`.
Every place the city dictionary can read (population ≥ 15,000) is written; a place in no
FUA carries an empty metro value. Omitting those places made rule 2 below guess: GHS-FUA
has no FUA for some sizeable cities (Jackson MS, Gainesville FL, Wilmington NC), so a
small namesake inside some FUA looked unique and bare "Jackson" resolved to Philadelphia.

### Derivation: `location.Parse` emits `Metros`

`Geo` gains `Metros []string` (metro values). For each city token `Parse` already resolves:

1. **City with a subdivision** ("Roseville, CA", "Folsom, California"): look up
   (city, country, admin1) in the metro table. This is what tells Roseville CA from
   Roseville MI. Subdivision → admin1 needs a code map beside `subdivisionToCountry`
   (US and Canada, the only subdivisions the parser reads today).
2. **City without a subdivision**: emit a metro only when the line states exactly one
   country AND every place in the metro table under that (name, country) resolves to the
   same metro. "No metro" counts as a value of its own: a name shared by a place in a
   metro and a place in none (Jackson NJ / Jackson MS) emits nothing, and a name whose
   only value is "no metro" emits nothing. "Folsom" emits Sacramento; a bare "Roseville" emits nothing. (Not the
   cityDict `Contested` flag: it means "the name exists in more than one country", which
   the one-stated-country requirement already settles — checking it would drop correct
   answers like "Moscow, Russia".) A line that states ANY subdivision never takes this
   path: each city is looked up under every stated subdivision and emits only when
   every lookup that finds the place agrees on one metro — "no metro" again counting as
   a disagreeing value. The same rule applies to an exact key repeated in the table.
3. Nothing for an unresolved city. Never guess — the dictionary rule every facet follows.

The result passes through `jobderive.Derive` exactly as `Cities` does, including the
structured-signal override from an adapter: when the adapter supplies cities, metros are
re-derived from them by rule 2 (`location.MetrosForCities`, adapter cities carry no
subdivision) under the final country, and only when there is exactly one; when it
supplies only countries, a metro whose country is not among them is dropped.

### Storage and search

- Migration: `jobs.metros text[] NOT NULL DEFAULT '{}'`, written by both ingest write
  paths and `backfill-derive`.
- Search document: `metros` field; filterable and in the facet list (`client.go`).
- `MergeClosureGeography` unions `metros` like `cities` — a collapsed duplicate must stay
  findable by every metro its hidden rows are open in, or the same bug as #2225 returns.
- Query: `metros` param in `query_filter.go`, joined to `locationFacets` so it ORs with
  regions, countries and cities ("Sacramento area OR Austin" works), excludable like them.
  Added to the `UnknownParams` vocabulary and to `web/static/openapi.yaml`.

### UI

`LocationPane` gets a metro group above cities, fed by the `metros` facet distribution.
Typing "Sacramento" shows "Sacramento area" first, the city below it. Chips, counts and
the include/exclude cycle follow the cities group exactly. The label is the facet value
with its ` (CC)` suffix replaced by " area".

### Attribution

GHS-FUA and GeoNames are both CC BY 4.0. Credit both on the public data-sources page.

## Rollout

1. Meilisearch settings patch (filterable + facet) BEFORE the binary — see
   `internal/search/search/AGENTS.md`; `search-settings-drift` reports a gap.
2. Deploy (release.sh runs the migration).
3. `backfill-derive` (already owed for the dictionary work of 2026-10-20) with
   `BACKFILL_DERIVE_CLOSED_WITHIN_DAYS`, concurrency 2-3, after 03:00 UTC.
4. Full `reindex` — `metros` is not in `content_hash`, so incremental pushes never reach
   existing rows.

Until step 4 finishes, the metro filter matches only postings ingested since deploy. The
UI group shows what the facet distribution holds, so it is never offered empty.

## Tests

- Generator: a tiny synthetic GeoPackage (two polygons, one cross-border) — membership,
  per-country split, naming by population, override.
- `Parse`: "Folsom, CA" → Sacramento (US); "Roseville, CA" → Sacramento (US); "Roseville, MI"
  → Detroit (US); "Roseville" → none; "Khimki" → Moscow (RU); "San Diego, CA" → San Diego (US).
- Search filter: `metros` ORs with `cities`; exclusion works; unknown-param report knows it.
- Closure union includes metros.
- UI: the metro group renders from the distribution and toggles the `metros` facet.

## Out of scope

- Radius search ("within 50 km of Folsom").
- Metro preferences on the profile / saved-search seeding from the profile.
- Subdivision-aware lookup outside the US and Canada.
