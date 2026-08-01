# Location conventions

## Scope
Curated dictionary deriving ISO 3166-1 alpha-2 country codes, region codes, and a work-mode hint from a free-text ATS location string.

## Always true
- **Two entry points, and the split is the point.** `Parse` answers "where is the WORK" (a
  job's location line); `ParseResidence` answers "where is the PERSON" (a CV's). They are
  distinct types, not a flag, because the same string means opposite things on the two
  sides and a flag can be defaulted wrong at a call site. Candidate text goes through
  `ParseResidence` and never through `Parse`.
- **A person is never located globally.** `ParseResidence` is `Parse` minus the `global`
  region and minus the work-mode hint. `global` reaches the result by TWO paths — the
  bare-remote fallback in `Parse` and the dictionary's own `worldwide`/`anywhere` entries —
  so the rule is phrased against the value, not against either mechanism. Work mode is a
  preference and lives on the profile (`location_preferences.work_modes`), not in geography.
- **Every code that carries a region must also have a NAME** (`TestEveryPlaceableCountryHasAName`).
  The two maps had drifted apart for 25 codes, so "Honduras" and "Rwanda" resolved to
  nothing. The failure is silent — an unresolvable country and an out-of-scope one both
  emit nothing — which is why it is a test rather than a review item.
- **Add country NAMES, never country CODES.** `resolveSubdivision` runs before the bare-code
  branch, so a code would collide with the US/Canada subdivision table (`LA`→Louisiana,
  `MN`→Minnesota, `MO`→Missouri). Names are full words and cannot.
- **A country name that is also a US city/state is deliberately withheld.** `georgia` is
  absent (the state); `palestine` is absent (Palestine, Texas) in favour of the unambiguous
  long forms. `Tbilisi, Georgia` still resolves to `ge` — through the city, not the country
  name. Precision over recall, the same rule the eligibility phrases follow.
- It is a curated dictionary, not a geocoder — it resolves high-frequency names/shorthands and emits nothing for what it can't resolve (never guesses).
- Region and work-mode values are drawn from the shared controlled vocabulary (`vocab.RegionValues`/`vocab.WorkModeValues`), so the parser, the enrichment payload, and the search facet all speak one set of values.
- Geography is exposed as a Meilisearch facet (`regions`/`countries`/`work_mode` are filterable attributes), not a Postgres column filter.
- A dictionary change needs a re-derive (`cmd/backfill-derive`) and a `cmd/reindex` to reach existing jobs.
- `work_mode` is dict-only — `jobview` serves the `jobs` column alone, the LLM's `enrichment.work_mode` is never merged in (it stays raw in the JSONB).
- `countries`/`regions` are a dict-then-LLM hybrid (`jobview.geoFacet`): the dictionary wins when it pins a place, and the LLM's `enrichment.countries`/`regions` fill only the unpinned (global/unspecified) bucket, so a dictionary-silent remote role still gets a geographic reach rather than none. This is the one deliberate exception to dict-only among the facets.

## How it works
`internal/location` parses the free-text location string from an ATS posting (e.g. "Berlin, Germany" or "Remote - US") and derives structured geography from it. The dictionary is curated for high-frequency names and shorthands; anything it cannot resolve is left empty rather than guessed. Because geography lives as Meilisearch facets (not Postgres column filters), the dictionary output flows through the search index, not through SQL WHERE clauses. This means a dictionary update is a two-step propagation: re-derive the facet columns on existing jobs (`cmd/backfill-derive`), then rebuild the search index (`cmd/reindex`). The production facets follow a deliberate split: `work_mode` is purely dict-driven (the LLM's work-mode guess is never served), while `countries`/`regions` are a hybrid where the dictionary pins what it can and the LLM fills the gaps for remote/unspecified roles.

## Limitations
- **The country dictionary is far from complete: 133 of 252 ISO countries carry a region.**
  111 resolve to nothing (Afghanistan, Cuba, Haiti, Jamaica, Syria, Madagascar, DR Congo,
  Namibia, plus a long tail of dependencies). Generating them is only half-possible:
  GeoNames `countryInfo.txt` supplies names, but this project's regions are a business
  taxonomy (`uk` split from `eu`, `cis` spanning the Caucasus and Central Asia, `mena`
  cutting across Africa and Asia), so a continent→region mapping would misplace Turkey,
  Egypt, Kazakhstan, the UK and Russia. The valuable half of that work is human.
- **The next coverage gain is in tokenization, not in the country list.** Measured over 100
  production CV locations: 38 distinct countries, all covered; the unresolved strings fail
  on `·` not being a separator, on the LinkedIn `Greater X` prefix, and on non-US
  subdivisions (`subdivisionToCountry` covers only the US and Canada, so Indian states do
  not resolve).
