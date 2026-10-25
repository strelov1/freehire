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
  region and minus the work-mode hint. `global` reaches the result only through the
  dictionary's own `worldwide`/`anywhere`/`international` entries (`nameToRegion`) — a
  bare `Remote` with no other signal now yields empty geography (the "region not
  specified" bucket, `regions=none`), never a guessed `global`; the rule is still phrased
  against the value rather than the mechanism, since `Residence` should never carry
  `global` even from the one remaining path. Work mode is a preference and lives on the
  profile (`location_preferences.work_modes`), not in geography.
- **A bare `Remote` with no other signal is NOT global.** Removed the old blanket
  `work_mode=="remote" && no geography` → `global` default: it collapsed "genuinely open
  anywhere" and "we have no idea" into the same claim, and moderator reports
  (`/moderation?tab=reports`, freehire job_reports 29-34) showed real region-restricted
  postings (a title suffix like `[Remote-US]`, or description prose like "hiring in the
  US, Canada, Argentina, or Brazil only") being served as globally open because of it. Two
  more sources now rescue a bare-remote posting before it's allowed to fall through to
  "unspecified": `RestrictionFromTitle` (an anchor-gated bracket/parenthetical suffix scan,
  `title_restriction.go`) and `RegionScopeFromDescription` (an anchor-gated "candidates
  based in X" / "hiring in X" / "restricted to X" clause scan, `region_scope.go`), both
  additive to and consulted after `EligibilityFromDescription`'s existing citizenship-phrase
  rescue, wired into `jobderive.Derive`'s precedence chain. Both anchor lists are
  deliberately narrow and role/candidate-qualified, not bare prepositions — a bare
  `"based in"`/`"located in"` anchor was tried and dropped: it matched a company-HQ "About
  Us" mention ("Our company is based in Berlin, but this role is remote and open
  worldwide") as if it were the role's own restriction, found in code review. Last in that
  chain, below every rescue, is an adapter's `FallbackCountries` (Greenhouse's offices — see
  `internal/ingest/sources/AGENTS.md`), which fills only a posting that stated no geography
  at all, never an explicit open-anywhere one.
- **Every code that carries a region must also have a NAME** (`TestEveryPlaceableCountryHasAName`).
  The two maps had drifted apart for 25 codes, so "Honduras" and "Rwanda" resolved to
  nothing. The failure is silent — an unresolvable country and an out-of-scope one both
  emit nothing — which is why it is a test rather than a review item.
- **Add country NAMES, never country CODES.** `resolveSubdivision` runs before the bare-code
  branch, so a code would collide with the US/Canada subdivision table (`LA`→Louisiana,
  `MN`→Minnesota, `MO`→Missouri). Names are full words and cannot.
- **Non-US/CA region names live in `foreignSubdivisionNames`** (`foreign_subdivisions.go`),
  merged into `nameToCountry` at init — Russian federal subjects, the UK nations and
  unambiguous counties, Australian states, German Länder, Indian states. Names only; a name
  with a second reading this catalogue meets (`Punjab`, `Victoria`, `Berkshire`) is withheld,
  and the contested territories (Crimea, Sevastopol, Donetsk, Luhansk, Zaporizhzhia, Kherson)
  are left out on purpose. A duplicate of a `nameToCountry` key panics at load.
- **Australian state CODES are the one exception, and only after a place.** SEEK writes
  "Perth WA" and "Sydney NSW" as one token with no comma, and the trailing code used to be
  read against the US/CA table alone, so WA became Washington and NT the Northwest
  Territories. `australianStateCodes` is read only after a place name, never as a bare token.
  NSW/VIC/QLD/TAS are Australia's alone and resolve after any place. WA/SA/NT/ACT are shared,
  so they resolve to Australia only when the place is a city the dictionary already puts
  there: "Seattle, WA" stays Washington. The code is cut off the token so the place keeps its
  city and metro, and it is never recorded as a stated subdivision, because the metro table
  keys those to US/CA codes.
- **A small place names a city, never a country** (`smallplaces.go`, `smallplaces.tsv`,
  written by `cmd/gen-cities` beside `cities1000.tsv`). The main table stops at 15,000
  people; the German crawl names towns of a few thousand beside their Land ("Billerbeck,
  Nordrhein-Westfalen"). A token neither table resolves is held as a per-country candidate
  and named only once the rest of the line has stated that country — so "Hasbergen" alone
  states nothing. The same pending path reads "Rottenburg am Neckar" (the town before
  am/an der/im/in der/bei) and "Köln-Niehl" (a main-table city opening a dash, longest
  prefix first). A token that opened a parenthesis is never a small place ("Schönebeck
  (Elbe)"). Loaded lazily: the server rarely needs it. Measured 2026-10-25 over the German
  crawl: postings with a city 76.1% → 94.0%. The residue is mostly German municipalities
  GeoNames carries only as administrative units (ADM4), absent from `cities1000` entirely
  (Borchen, Weinstadt, Spremberg, Wedemark).
- **A curated city override renames the PLACE** (`loadCityDict`), not only its own alias,
  and `metroFor` maps the curated name back to the GeoNames one the metro table is keyed
  by. Renaming one alias split Frankfurt across "Frankfurt" and "Frankfurt am Main" and
  left "Frankfurt"/"Cologne"/"Zurich" with no metro.
- **A subdivision name that is also ANOTHER city's alias states no city**
  (`regionNotCity`): the Land Brandenburg is an alternate name of Brandenburg an der Havel.
  `TestSubdivisionCityAliasesAreDecided` makes every new overlap a decision.
- **Area words and postal prefixes are cut before lookup**: "Großraum/Region/Raum/Umkreis/
  Standort/Kanton X", "X und Umgebung", and a continental postal code before its town
  ("85462 Eitting", "CH-3400 Burgdorf"). The code states no country — four and five digits
  are half of Europe's formats.
- **Metros are dict-only and need one stated country.** `Geo.Metros` comes from
  `metros.tsv` (generated by `cmd/gen-metros` from GeoNames + GHS-FUA). A city with a stated
  US/CA subdivision is looked up exactly; a bare city only when every same-named place in
  that country, in a FUA or not, shares one metro. The table therefore lists every place
  the city dictionary can read, with an empty metro for one in no FUA, and "no metro"
  counts as a value of its own: bare "Jackson" is Jackson NJ (Philadelphia) and Jackson MS
  (none), so it gets nothing. An exact key the table repeats under two values is dropped
  the same way, and "no metro" is never returned as a metro. Regenerating the table needs `backfill-derive` + `reindex` like any
  dictionary change.
- **A colliding state code defaults per code, by measurement.** `IL`, `MA`, `PA`… are both
  a US state and a country. The default reading is the country unless the preceding city is
  a known US/CA place — except for `stateFirstSubdivisions` (pa, ma, md, il, mn, al, mo, la,
  me), where the US-state reading dominates the catalogue: after any city the dictionary does
  not place in that country, the code is the state. Without it ~29k US postings sat under
  Morocco, Panama, Moldova and Albania ("Cambridge, MA": GeoNames calls Cambridge British).
  A code joins the set on a `cmd/report-geo-gaps` measurement, never on intuition.
- **A full UK postcode states `gb`, like a US ZIP states `us`** (`stripUKPostcode`), and is
  cut from its token so the place before it still resolves ("London SW1A 2HB"). Only the
  full outward + inward form, under a real postcode area, counts; an inward code that reads
  as an ordinal ("B2 2nd floor") does not, and GY/JE/IM are Crown dependencies, not `gb`.
  Found through Civil Service Jobs, whose "Durham, North East England, DH1 3HU" resolved to
  the US — which is also why `durham` left the curated city map and England's ITL 1 region
  names joined `foreignSubdivisionNames`.
- **A country name that is also a US city/state is deliberately withheld.** `georgia` is
  absent (the state); `palestine` is absent (Palestine, Texas) in favour of the unambiguous
  long forms. `Tbilisi, Georgia` still resolves to `ge` — through the city, not the country
  name. Precision over recall, the same rule the eligibility phrases follow.
- It is a curated dictionary, not a geocoder — it resolves high-frequency names/shorthands and emits nothing for what it can't resolve (never guesses).
- Region and work-mode values are drawn from the shared controlled vocabulary (`vocab.RegionValues`/`vocab.WorkModeValues`), so the parser, the enrichment payload, and the search facet all speak one set of values.
- Geography is exposed as a Meilisearch facet (`regions`/`countries`/`work_mode` are filterable attributes), not a Postgres column filter.
- A dictionary change needs a re-derive (`cmd/backfill-derive`) and a `cmd/reindex` to reach existing jobs.
- **German work-arrangement prose is a second phrase list** (`workmode_de.go`), read only
  when no English phrase matched. German lists home working as a BENEFIT ("Homeoffice
  möglich", "2 Tage Homeoffice", "mobiles Arbeiten"), which is hybrid, and states remote only
  with an explicit totality ("100% Homeoffice", "vollständig remote"). A remote phrase
  therefore contains a hybrid one, so the German list checks remote FIRST — the reverse of the
  English order. Matched on folded markup with a word boundary at the end ("mobile
  Arbeitsmaschinen" is a trade) and a short negation window before ("kein Homeoffice
  möglich"). Bare `remote`/`hybrid`, `vor Ort` and `deutschlandweit` are deliberately absent.
- **Dutch work-arrangement prose is a third list** (`workmode_nl.go`), read only when neither
  the English nor the German one matched, with the German ordering and for the same reason:
  "hybride werken", "2 dagen thuiswerken" and "thuiswerkvergoeding" are benefits (hybrid);
  remote needs "volledig remote" / "100% thuiswerken". A phrase in the same sentence as
  "buitenland" or "per jaar" is a bounded perk ("twee weken per jaar remote werken vanuit het
  buitenland") and is passed over. Bare `hybride` (cloud, infrastructure), `op afstand` and
  `niet remote` (remote support) are deliberately absent. Measured 2026-10-25 over 42k open
  Dutch postings: work mode 3.5% → 16.4%.
- `work_mode` is dict-only — `jobview` serves the `jobs` column alone, the LLM's `enrichment.work_mode` is never merged in (it stays raw in the JSONB).
- `countries`/`regions` are a dict-then-LLM hybrid (`jobview.geoFacet`): the dictionary wins when it pins a place, and the LLM's `enrichment.countries`/`regions` fill only the unpinned (global/unspecified) bucket, so a dictionary-silent remote role still gets a geographic reach rather than none. This is the one deliberate exception to dict-only among the facets.

## How it works
`internal/dict/location` parses the free-text location string from an ATS posting (e.g. "Berlin, Germany" or "Remote - US") and derives structured geography from it. The dictionary is curated for high-frequency names and shorthands; anything it cannot resolve is left empty rather than guessed. Because geography lives as Meilisearch facets (not Postgres column filters), the dictionary output flows through the search index, not through SQL WHERE clauses. This means a dictionary update is a two-step propagation: re-derive the facet columns on existing jobs (`cmd/backfill-derive`), then rebuild the search index (`cmd/reindex`). The production facets follow a deliberate split: `work_mode` is purely dict-driven (the LLM's work-mode guess is never served), while `countries`/`regions` are a hybrid where the dictionary pins what it can and the LLM fills the gaps for remote/unspecified roles.

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
