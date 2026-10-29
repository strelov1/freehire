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
- **A US/CA or Australian code before a DIFFERENT country that closes the location is
  that country's subdivision** (`foreignSubdivisionCode`): "Breda, NB, Netherlands" is
  Noord-Brabant, not New Brunswick; "Barcelona, CT, Spain" Catalonia; "Tijuana, BC, Mexico"
  Baja California. The town's own reading (curated name or main city table, never
  small places — most European cities have a US namesake there) must be the closing
  country and not the code's, so "Austin, TX, Canada" keeps Texas and an unknown town
  changes nothing. The country must be the LAST token of its location ("Naples, FL,
  Naples, FL, US" repeats a US place). Measured 2026-10-28: 252 distinct lines, 4,290
  open rows carried the spurious `us`/`ca`, all removals.
- **A country, city or region after a preposition is read** (`stripPlacePreposition`):
  "Remote within Canada", "Based in Germany", "Anywhere in the U.S.", "Remote across
  LATAM". The preposition used to stay glued to the name, so the token resolved to nothing
  — "…, or Remote within Canada or United States" lost Canada for an API customer
  filtering `countries=ca`. Only `in`/`within`/`from`/`across`/`throughout`, only after
  lead-in words (`placeLeadWords`: a work mode, "anywhere", "based", "work from"…), and
  only when the rest is a country, region or city name — a main-table city under its own
  name only, never a GeoNames alias ("market" is Marquette's). A town before the
  preposition ("Kirchheim in Schwaben") or an unknown name keeps the token whole. A bare
  "IN" opening a line is also Indiana's code, so after it only a curated name the US has
  no town of counts ("IN Lebanon", "IN Munster" read nothing); a state or province name is
  left to `resolveSubdivision`, which already reads one closing a token. Measured
  2026-10-28 over every stored location: 309 distinct lines, 901 open rows gained a
  country or region, none lost one.
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
- **Indian location shapes are read in `india.go`**, per comma-token, before lookup:
  country-state-city exports ("IN-KA-Bangalore", "India-Karnataka-Bangalore",
  "IN.TN.Chennai") become their city; a city's revenue division or municipal form
  ("Bangalore North", "Pune City", "Mumbai Suburban") becomes the city, but only when what
  remains is an INDIAN city of the table, and never "Rural" (Bangalore Rural is another
  district); an office line opening with a curated Indian city ("Chennai DLF") is that city.
  "Delhi NCR"/"NCR"/"Greater Delhi Area" state the country and the Delhi metro and NO city —
  naming Delhi, Gurugram or Noida would be a guess. "Pan India"/"Anywhere in India" state
  the country, and their "anywhere" is not a remote marker. Indian display names drop
  GeoNames' transliteration marks ("Thāne" → "Thane") in the city table, the search index
  and the metro table alike (`foldIndianName`); two aliases GeoNames hands to a neighbour
  are pinned without renaming it (`cityAliasPins`: Visakhapatnam, Secunderabad). A line that
  names the United States reads a colliding code beside it as the state ("Hobart, IN, US"
  is Indiana). Measured 2026-10-25 over 63,305 Indian postings: city 78.3% → 84.4%. The
  residue is mostly localities (Malad, Baner, Andheri, Gachibowli), employer office codes
  ("TPIN Malad", "INT Thane") and states alone, which correctly state no city.
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
- **Dutch and Flemish location lines** (`location_nl.go`, the Dutch block of `nameToCountry`
  and of `foreignSubdivisionNames`). Recruitee writes the country in the posting's language,
  and "Nederland" was read as the Colorado town: 10,801 open rows had no country (2026-10-25).
  The provinces state the country and no city; `limburg` is in `regionNotCity` and states no
  country either (Dutch AND Belgian, and a German town). A province named after its capital
  (`provinceCapitals`: Utrecht, Groningen, Antwerpen…) is the province when a town precedes
  it. eures writes a province code after the town ("Hengelo OV", "Hoorn NH");
  `stripDutchProvince` cuts it, and a code shared with a US state or an ISO country (NH, NB,
  UT, FR…) only when the town is Dutch. Also: the "1012 AB" postcode, "regio X", "X en
  omgeving", and Recruitee's translated remote label ("Werken op afstand", "Homeoffice",
  "Poste à distance") as a remote marker, matched only as the whole line.
- **Spanish work-arrangement prose** (`workmode_es.go`) is read after the Dutch list, on the
  folded text with its accents dropped, and in the German order (remote, hybrid, onsite):
  "100% remoto"/"100% teletrabajo" is remote; "modalidad híbrida", "3 días presenciales",
  "2 días de home office" and "teletrabajo" in a perks list are hybrid; "modalidad
  presencial", "100% presencial" and "sin teletrabajo" are onsite. The Dutch matcher is
  shared (`guardedPhraseMatches`): negations ("sin", "no", and the hedge "evaluar"),
  refusals after ("no corresponde") and seasonal perks ("teletrabajo completo en verano")
  withdraw a phrase. Bare `remoto` (remote support), `híbrido` (hybrid stacks), `presencial`
  (a tutoring platform's "clases presenciales o en línea" on ~5k rows) and `home office`
  (German and English prose reach this list) are deliberately absent. Portuguese shares the
  phrasing and benefits ("100% presencial", "modelo híbrido"). Measured 2026-10-27 over 39k
  Spanish-language postings: work mode from any source 48.7% → 56.8%; the description list
  agrees with the adapter's structured value on 86% of the rows that carry both, and most of
  the rest are the adapter's value contradicting the posting's own text.
  Argentina (2026-10-27, 6,145 postings) states the model as an "esquema" ("esquema de
  empleo: presencial", "esquema remoto") or a "jornada laboral", writes "Modalidad : híbrida"
  with a space before the colon, and gives a share ("40% presencial", "80% remoto"), which
  is hybrid, as are weekly office visits ("1/2 asistencias semanales") — read before the
  phrase list and under its guards (`spanishSplitWeek`), so "remoto, 40% presencial" is hybrid.
  "Ubicación: Remoto" is absent: Torre's template writes it above hybrid postings.
  Description-derived work mode 35.2% → 40.3%.
- **Portuguese work-arrangement prose** (`workmode_pt.go`) is read after the Spanish list,
  which already reads the shared phrases ("100% remoto", "modelo híbrido", "100%
  presencial"). Brazilian postings state the model in a labelled line ("Modelo de
  trabalho: híbrido", "Modalidade: presencial", "Atuação: 100% remota"; a spaced colon is
  read too). "home office" is REMOTE when it is the model ("Modelo de trabalho: home
  office", "local de trabalho: home office") and nothing as a benefit ("auxílio home
  office" is listed by remote, hybrid and onsite postings alike); counted office days ("2x
  por semana presencial", "3 dias presenciais e 2 dias home office") are hybrid. Absent: bare
  `remoto`/`trabalho remoto` (support, equipment requirements), bare `teletrabalho` and
  `sistema híbrido` (an internship boilerplate on ~200 empregos postings that says the
  opposite), bare `presencial` and `modalidade presencial` (a course's modality in an
  education requirement). The location-line markers read `remoto`, `híbrido` and
  `presencial` ("Remoto, Remoto, BR"). Two fixes to the shared guard came with it: a
  negation must START a word ("entorno", "bono", "humano", "polígono" end in "no " and had
  hidden stated Spanish hybrids) and governs only its own clause ("não é 100% remota;
  modelo híbrido" is hybrid); the Spanish negations also read "não"/"sem", since
  Portuguese prose meets that list first. Measured 2026-10-27 over 30,677 Brazilian and
  6,931 Portuguese postings, location plus description: work mode 4.9% → 14.7% (BR) and
  15.8% → 18.6% (PT); 24 of 17,500 control rows changed, all correct.
- **French work-arrangement prose** (`workmode_fr.go`) is read after the Portuguese list.
  French lists home working as a BENEFIT, as German does ("télétravail partiel possible",
  "2 jours de télétravail par semaine", "accord/charte/politique de télétravail",
  "subvention télétravail") — all hybrid; remote needs a stated totality ("100 %
  télétravail", "télétravail total", "entièrement en télétravail", "full-remote"); onsite
  is a stated refusal or totality ("pas de télétravail", "Télétravail : non", "100 % en
  présentiel", Indeed's "Lieu du poste : En présentiel"). Absent: bare `télétravail`
  (software that sells it), bare `à distance`, `100 % à distance` and `distanciel` (an
  apprenticeship's school days — "formation à distance" sits on ~900 postings), bare
  `présentiel` (the interview and the training), bare `hybride`, `mode hybride` and
  `modèle hybride` (hybrid vehicles and clouds, a training format, a business model),
  `télétravail : 100` ("prime télétravail : 100 € / mois"). Negations ("pas", "sans",
  "aucun") reach 24 bytes back ("pas de possibilité de télétravail"); "-friendly" after a
  remote phrase withdraws it ("100 % remote-friendly"); "les fêtes" bounds a perk.
  Measured 2026-10-28 over 24,609 French-language postings: the description reader 0.4% →
  12.3%; with the structured signal, 31.1% → 34.8%. Every changed control row was French.
- **French location lines** (`location_fr.go`, the French block of
  `foreignSubdivisionNames`, measured 2026-10-28 over 33k postings placed in France). A
  compound town written with spaces for its hyphens ("Issy les Moulineaux", "Aix en
  Provence") reads through `hyphenFolded`, which only rewrites a token no table knows as
  written. An arrondissement ("Lyon 3e", "Paris 19e Arrondissement", "9th arrondissement
  of Paris") is the city; "Paris La Défense" is La Défense. A five-digit token is a postcode
  of the country the line names, not a US ZIP, when the line names no part of the US or
  the caller's countries exclude it (`foreignPostcodeLine`; "Paris, 75009, France" was
  France AND the US). A small town's own hyphens are not a dash export ("Meslay-du-Maine"
  named Maine, "Porto-Vecchio" Portugal). The regions (current and pre-2016) and the
  departments state France and no city; withheld: Vienne, Jura, Manche, Nord, Ardennes,
  Meuse (each has a second reading). Martinique, Guadeloupe, Guyane, Réunion and French
  Polynesia keep their own ISO codes (placed in latam, africa and apac); New Caledonia and
  Mayotte are absent, their codes being North Carolina's and Yukon's. Two catalogue-wide
  fixes came with it: a US state or Canadian province after a town the curated tables
  placed abroad names its namesake and withdraws the other country ("Paris, TX", "London,
  ON", "Naples, FL" were France, Britain, Italy too — unless the town is itself a province,
  "Saint-Laurent, Quebec, CA"); and a held-back contested alias names the line's own
  country's place of the name when that country has exactly one, literally so called
  (`contestedCityIn`: "Valence, France", "León, ES", "Baden, CH"; not "Conceição", not
  Argentina's several San Martíns). A flag emoji is its country ("Lyon 🇫🇷"). FR-placed
  lines: country 90.2% → 94.4%, a wrong second country 0.4% → 0.2%, city 78.4% → 84.2% (the
  3.6k bare "France" lines can name none). werecruit writes "Provence-Alpes-Côte d'Azur" as
  the region of nearly every posting whatever the town; the town still resolves.
- **Spanish and Latin-American location lines** (`location_es.go`, the Spanish, Mexican,
  Colombian, Argentine and Chilean blocks of `foreignSubdivisionNames`, measured 2026-10-27
  over 39k Spanish-language postings). The autonomous communities, Basque provinces and
  islands state Spain and no city; the Mexican states OCC writes after the town state Mexico.
  A province named after its capital is the province, not a second city, when a place came
  before it: between two commas for Spain ("Oliva, Valencia, España"), also in Factorial's
  "ESPAÑA/Province/Community" export, and also in the LAST slot for Mexico ("León,
  Guanajuato") — `provinceAfterCity` now asks whether a place came first, not whether a city
  did, so an unresolved town before the province still counts. Country names GeoNames gives
  the capital ("Mexico", "Panama", "Puerto Rico", "Guatemala") and "D.C." are
  `regionNotCity`: "Monterrey, Mexico" used to name Mexico City too. **A contested alias whose
  city is named differently from the line is held back until the line states that city's
  country** (`renamedContested`): "León, ES" was León de los Aldama, "Canton, Ohio" Guangzhou,
  "Salem, MA" Jerusalem, "Annapolis" a Brazilian town — ~6.7k wrong city names over the
  Spanish sample, ~550 lines in an 18k-line control. Valencia and Zaragoza are city overrides:
  the curated es country used to suppress their city. infojobs and occ carry
  `FallbackCountries` es/mx (28% and 16% of their open postings stated no country).
- **Brazilian state codes and Portuguese districts** (`location_br.go`, measured 2026-10-27
  over 30,677 Brazilian and 6,931 Portuguese postings). Every Brazilian ATS writes the
  state code after the town ("Curitiba, PR", "Belo Horizonte/MG", "São Paulo - SP",
  "Campinas SP"), and most codes spell a country or a US state: PR Puerto Rico, SC South
  Carolina, BA Bosnia, ES Spain, PE Peru, MT Malta, RS Serbia, MA Massachusetts. 20.1% of
  the Brazilian lines carried a wrong second country, and "Fortaleza, CE" none. A code is
  now the Brazilian state when the line names Brazil or the town before it has a
  Brazilian reading in any table — including a namesake the city table gives to another
  country first, whose country is withdrawn and whose name becomes the Brazilian one
  ("Colombo, PR", "Concordia, SC" → Concórdia, not El Paso). Not when the line names the
  US, and not after the country the code spells ("Juan Díaz, Panamá, PA"). Lines with a
  wrong country 20.1% → 1.1%; exactly Brazil 76.6% → 97.9%. The state names that have no
  second reading state Brazil and no city (`foreignSubdivisionNames` + `regionNotCity`:
  "Rio Grande do Sul" named Rio Grande, "Ceará" Fortaleza, "Bahia" Salvador); Paraná,
  Santa Catarina, Amazonas, Acre and Distrito Federal are left out (an Argentine city, a
  Mexican city, a Venezuelan region, an Israeli city, Mexico City). An Indian state code
  between an Indian city and the country is the state ("Patna, BR, in" was Brazil too).
  Portugal: "Porto" was an alias of Bordeaux, so "Porto, PT" named no city (550 lines in
  the sample) — now a city override; a district named after its capital is the district
  between two commas ("Matosinhos, Porto, PT"; `portugueseDistrictCapitals`); Madeira and
  the Azores state Portugal. Portuguese city coverage 60.2% → 68.2%, two-city lines 4.1% →
  1.2%. Not seen in the sample, so not built: "Grande São Paulo", "ABC Paulista",
  "Região Metropolitana de …", "Grande Lisboa", a CEP before the town.
- **Argentina** (`location_ar.go`, measured 2026-10-27 over ~12.3k open postings placed in
  Argentina). Buenos Aires is rarely written "Buenos Aires": the autonomous city is "CABA",
  "C.A.B.A.", "Capital Federal", "Ciudad de Buenos Aires" (city overrides; "CABA" was
  GeoNames' alias of Chamba), pandapelatam writes a barrio ALONE ("Palermo", "San Nicolás",
  "Recoleta") and the country only in its structured data, and the conurbation's towns are
  under GeoNames' floor or under a namesake ("Martínez" was California's, "Florida" the US
  state). A barrio is the city of Buenos Aires and a town of Greater Buenos Aires is itself,
  both in the metro "Buenos Aires (AR)", and only in a line that names nothing but Buenos
  Aires context (`lineInBuenosAires`), so "Palermo, Italia" and "San Cristóbal, Santa Fe"
  keep their own reading; a name another Argentine place shares (Pilar, San Miguel, San
  Martín, Avellaneda, `sharedArgentineNames`) needs Buenos Aires itself in the line, not just
  the country. GBA/AMBA/Gran Buenos Aires state the country and the metro, no
  city. A province named after its capital is the province between two commas of a line
  ending in Argentina ("Rosario, Santa Fe, Argentina"). GHS-FUA labels Greater Mendoza and
  San Juan by a suburb; `metroLabelOverrides` renames them.
  **`ParseIn(line, countries)`** is how the adapter's country reaches the parser: when the
  line states none, it names a barrio, a contested or small town (in a one-place line only,
  so "Hermosillo, Sonora" does not name the town GeoNames calls Sonora) and the metro;
  it never adds a country. `jobderive.Derive` passes `Countries`, and `FallbackCountries`
  only once the fallback is what places the posting (a title's "[Remote-US]" outranks it).
  Two country-code fixes came out of the same sample and are not Argentine: a colliding
  code after a US town only the small-places table knows is the state ("Atkins, AR" was
  Argentina — 195 lines in the sample), and a code between a town and a DIFFERENT country
  the town belongs to is that country's subdivision ("Zaragoza, AR, España" Aragón,
  "Madrid, MD, Spain", "Jena, TH, Germany"). AR-placed lines: city 65.9% → 75.6%, metro
  48.9% → 69.1%, "Buenos Aires (AR)" 31.7% → 44.2%.
- **Indian work-arrangement prose is a fourth phrase list** (`workmode_in.go`), read only
  when none of the English, German, Dutch, Spanish, Portuguese and French ones matched. "WFO"/"work from office" is onsite;
  counted office days decide the rest ("3 days WFO", "Hybrid (3 days office)" → hybrid;
  "5 days work from office" → onsite; "2 days WFH" → hybrid); a "hybrid" just before an
  office marker keeps it hybrid. Bare "WFH" is NOT remote — it names a benefit ("occasional
  WFH"), a shift or the staff an HR role manages as often as the arrangement — so only
  anchored forms ("permanent WFH", "WFH / Remote", "Location: WFH") are. Measured
  2026-10-25: description-derived work mode on Indian postings 8.6% → 10.7%.
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
