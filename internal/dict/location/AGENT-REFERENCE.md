# Location conventions

## Scope

Curated dictionary deriving ISO 3166-1 alpha-2 country codes, region codes, and a work-mode hint from a free-text ATS location string.

## Always true

- **Two entry points, by design.** `Parse` answers "where is the WORK" (a job's location
  line); `ParseResidence` answers "where is the PERSON" (a CV's). They are distinct types,
  not a flag, because the same string means opposite things on the two sides and a flag can
  be defaulted wrong at a call site. Candidate text goes through `ParseResidence`, never
  `Parse`.
- **A person is never located globally.** `ParseResidence` is `Parse` minus the `global`
  region and minus the work-mode hint; `Residence` never carries `global`, even from the
  dictionary's own `worldwide`/`anywhere`/`international` entries (`nameToRegion`). Work
  mode is a preference on the profile (`location_preferences.work_modes`), not geography.
- **A bare `Remote` with no other signal is NOT global**: it yields empty geography (the
  "region not specified" bucket, `regions=none`); `global` comes only from
  `nameToRegion`'s open-anywhere words. Do not reintroduce a "remote and no geography →
  `global`" default: moderator reports (job_reports 29-34) showed region-restricted
  postings (`[Remote-US]` title suffix, "hiring in the US … only") served as worldwide. `jobderive.Derive`'s
  precedence chain rescues a bare-remote posting, in order: `RestrictionFromTitle`
  (an anchor-gated bracket/parenthetical suffix, else with no anchor a place-only trailing
  label like "(LATAM)" / "- India" or a leading macro-region, `title_restriction.go`),
  `EligibilityFromDescription` (citizenship phrases), `RegionScopeFromDescription` (anchor-gated "candidates based in
  X" / "hiring in X" / "restricted to X", `region_scope.go`), and last an adapter's
  `FallbackCountries` (Greenhouse's offices — see `internal/ingest/sources/AGENTS.md`),
  which fills only a posting that stated no geography at all, never an explicit
  open-anywhere one. The anchors are role/candidate-qualified on purpose: a bare
  `"based in"`/`"located in"` matched a company-HQ mention ("Our company is based in Berlin, but
  this role is remote").
- **Every code that carries a region must also have a NAME** (`TestEveryPlaceableCountryHasAName`).
  The two maps had drifted for 25 codes ("Honduras", "Rwanda" resolved to nothing). The
  failure is silent — unresolvable and out-of-scope both emit nothing — hence a test.
- **Add country NAMES, never country CODES.** `resolveSubdivision` runs before the bare-code
  branch, so a code would collide with the US/Canada subdivision table (`LA`→Louisiana,
  `MN`→Minnesota). Names are full words and cannot.
- **Non-US/CA region names live in `foreignSubdivisionNames`** (`foreign_subdivisions.go`),
  merged into `nameToCountry` at init. Names only; a name with a second reading
  (`Punjab`, `Victoria`, `Berkshire`) is withheld, and the contested territories (Crimea,
  Sevastopol, Donetsk, Luhansk, Zaporizhzhia, Kherson) are left out on purpose. A
  duplicate of a `nameToCountry` key panics at load.
- **Australian state CODES are the one exception, and only after a place**
  (`australianStateCodes`; SEEK writes "Perth WA", "Sydney NSW" as one token, and WA used to
  become Washington, NT the Northwest Territories). Never read as a bare token.
  NSW/VIC/QLD/TAS resolve after any place; the shared WA/SA/NT/ACT only after a city the
  dictionary puts in Australia ("Seattle, WA" stays Washington). The code is cut off
  the token so the place keeps its city and metro, and is never recorded as a stated
  subdivision (the metro table keys those to US/CA codes).
- **A US/CA or Australian code before a DIFFERENT country that closes the location is
  that country's subdivision** (`foreignSubdivisionCode`): "Breda, NB, Netherlands" is
  Noord-Brabant, "Tijuana, BC, Mexico" Baja California.
  The town's own reading (curated name or main city table, never small places — most
  European cities have a US namesake there) must be the closing country, not the code's:
  "Austin, TX, Canada" keeps Texas; an unknown town changes nothing. The country must be
  the LAST token ("Naples, FL, Naples, FL, US" repeats a US place). Measured 2026-10-28:
  252 distinct lines, 4,290 open rows lost a spurious `us`/`ca`.
- **A country, city or region after a preposition is read** (`stripPlacePreposition`):
  "Remote within Canada", "Based in Germany" (the glued preposition used to lose Canada for
  an API customer filtering `countries=ca`). Only `in`/`within`/`from`/`across`/`throughout`,
  only after `placeLeadWords` (a work mode, "anywhere", "based", "work from"…), and only when the rest
  is a country, region or main-table city under its own name, not an alias ("market" is
  Marquette's). A town before the preposition ("Kirchheim in Schwaben") or an
  unknown name keeps the token whole. A line-opening "IN" is also Indiana, so after it only
  a curated name the US has no town of counts ("IN Lebanon", "IN Munster" read nothing);
  state/province names are left to `resolveSubdivision`. Measured 2026-10-28: 309 distinct
  lines, 901 open rows gained a country or region, none lost one.
- **A code that is both a US/CA subdivision and an ISO country is the COUNTRY when it
  closes a location of three or more tokens** (`countryCodeClosingCity`): "Toronto, ON, CA"
  and "Toronto, CA, CA" are Canada; "Tel Aviv-Yafo, Tel Aviv District, IL" Israel. After
  one of that country's own provinces it is the country whatever the town ("London, ON,
  CA"); otherwise the first place must be a curated or main-table city of that country and
  not of the US/CA ("San Jose, CA, CA" stays Californian). Two-token "City, CA" is untouched.
- **A US state and the US remove a foreign namesake of the town**, as a bare state does
  ("Paris, TX"): "Valencia, CA, US", "Dublin, CA, US", "Lebanon, PA, US" name no Spain,
  Ireland or Lebanon. Also when the code is read as the closing country's subdivision
  (`subdivisionBeforeCountry`), and for a colliding code beside a named US when the US has a
  place of that name ("Jakarta, ID, United States" keeps Indonesia). Measured 2026-10-28: 4,271
  distinct lines, 42,933 open rows corrected (~34.9k Canadian rows lost a spurious `us`);
  18.3k more ("Fresno, CA, US") gained a metro via the now-recorded state (`metroFor`).
- **A Japanese address states `jp`** (`location_ja.go`) when a token is one of the 47
  prefectures in Japanese script WITH its 都/道/府/県 suffix ("港区, 東京都"), opens with one
  and continues to a municipality (市/区/郡/町/村; not a Hong Kong shop "北海道牧場餐廳"), or
  carries 〒. Bare "東京", a ward/city alone or a prefecture mid-token state nothing. No
  city or metro (the city tables are Latin-script). Measured 2026-10-30: all ~43.8k open HRMOS postings and ~20k of
  HERP's had no country.
- **A small place names a city, never a country** (`smallplaces.go`, `smallplaces.tsv`,
  written by `cmd/gen-cities` beside `cities1000.tsv`). The main table stops at 15,000
  people; the German crawl names smaller towns ("Billerbeck, Nordrhein-Westfalen"). A token
  neither table resolves is held as a per-country candidate and named only once the line
  states that country ("Hasbergen" alone states nothing). The same pending path reads
  "Rottenburg am Neckar" (town before am/an der/bei…) and "Köln-Niehl" (main-table
  city opening a dash, longest prefix first). A token that opened a parenthesis is never a
  small place ("Schönebeck (Elbe)"). Loaded lazily. Measured 2026-10-25, German crawl: city
  76.1% → 94.0%; the residue is mostly ADM4-only municipalities absent from `cities1000`.
- **A site is not a town.** K-12 boards (applitrack) write the SITE where a town goes. A
  token carrying `school`, `academy` or `district` is not a "City-District" dash
  (`namesInstitution`; "Harrison-Morton Middle School" was Scranton), and a
  `siteDescriptors` word is cut with the site word ("Central Office" was Central,
  Louisiana). A site that names its town still reads it ("Lincoln School, Palatine, IL").
  Measured 2026-10-30: 600 applitrack sites, rows with a city 640 → 344, every drop wrong;
  the top 40,000 other strings (7.7M rows) changed in six lines, all correct.
- **A city read through another place's ALIAS answers to the line's country and state**
  (`alias_place.go`, `addAliasCity`). `cityDict` hands each GeoNames alias (old, foreign,
  colloquial) to the most populous carrier, so "Harrison" was Scranton, "Frisco" San
  Francisco, even in "Harrison, NJ". cities1000 has no historical flag, and "own name beats
  alias" is wrong for live exonyms (Calcutta, Bombay), so the rule reads the LINE: an alias
  city whose country the line does not state is not named; one whose metro-table admin1 is
  none of the stated US/CA subdivisions is replaced by the place whose OWN name the token
  is there (`townIn`, punctuation and St/Saint folded: "St Petersburg, FL"), or by nothing
  — never by another alias ("Newton, NC" would become Wilmington). A bare alias with no
  stated geography still reads ("Calcutta"). Measured 2026-10-31, top 40,000 strings (7.68M
  rows): 126k rows changed city, no country; ~226 lost a right city, each behind a
  pre-existing country misread ("St. Gallen, SG" is Singapore) or D.C. not being recorded
  as a subdivision.
- **A curated city override renames the PLACE** (`loadCityDict`), not only its alias, and
  `metroFor` maps the curated name back to the GeoNames key of the metro table. Renaming
  only the alias split Frankfurt across two names and left it with no metro.
- **A subdivision name that is also ANOTHER city's alias states no city**
  (`regionNotCity`: the Land Brandenburg vs Brandenburg an der Havel).
  `TestSubdivisionCityAliasesAreDecided` makes every new overlap a decision.
- **Area words and postal prefixes are cut before lookup** (`areaPrefixes`/`areaSuffixes`,
  `postalPrefixRE`): "Großraum/Region/Raum/Umkreis/Standort/Kanton X", "X und Umgebung", a
  continental postcode before its town ("85462 Eitting", "CH-3400 Burgdorf"). The code
  states no country — four and five digits are half of Europe's formats.
- **Metros are dict-only and need one stated country.** `Geo.Metros` comes from
  `metros.tsv` (`cmd/gen-metros`, GeoNames + GHS-FUA). A city with a stated US/CA
  subdivision is looked up exactly; a bare city only when every same-named place in that
  country shares one metro. The table lists every place the city dictionary can read, with
  an empty metro for one in no FUA, and "no metro" counts as a value: bare "Jackson" is
  Jackson NJ (Philadelphia) and Jackson MS (none), so nothing. An exact key repeated under
  two values is dropped the same way; "no metro" is never returned. Regenerating needs
  `backfill-derive` + `reindex`.
- **A colliding state code defaults per code, by measurement.** `IL`, `MA`, `PA`… are both
  a US state and a country; the default is the country unless the preceding city is a known
  US/CA place. For `stateFirstSubdivisions` (pa, ma, md, il, mn, al, mo, la, me) the state
  wins after any city not placed in that country — without it ~29k US postings sat under
  Morocco, Panama, Moldova, Albania ("Cambridge, MA"). A code joins the set only on a
  `cmd/report-geo-gaps` measurement.
- **A full UK postcode states `gb`, like a US ZIP states `us`** (`stripUKPostcode`), and is
  cut so the place before it resolves ("London SW1A 2HB"). Only full outward + inward form
  under a real postcode area; an inward code that reads as an ordinal ("B2 2nd floor")
  does not count, and GY/JE/IM are Crown dependencies, not `gb`. Found via Civil Service
  Jobs ("Durham, North East England, DH1 3HU" was US) — also why `durham` left the curated
  city map and England's ITL 1 region names are in `foreignSubdivisionNames`.
- **A country name that is also a US city/state is withheld.** `georgia` (the state) and
  `palestine` (Palestine, Texas) are absent; "Tbilisi, Georgia" still resolves to `ge`
  through the city.
- **Indian location shapes** (`india.go`), per comma-token before lookup:
  country-state-city exports ("IN-KA-Bangalore") become the city; a
  revenue division or municipal form ("Bangalore North", "Pune City") becomes the city only
  when the rest is an INDIAN table city, never "Rural" (Bangalore Rural is another
  district); an office line opening with a curated Indian city ("Chennai DLF") is that
  city. "Delhi NCR"/"NCR"/"Greater Delhi Area" state the country and the Delhi metro, NO
  city (it would be a guess). "Pan India"/"Anywhere in India" state
  the country; their "anywhere" is not a remote marker. Indian names drop GeoNames'
  transliteration marks ("Thāne" → "Thane") in city table, search index and metro table
  alike (`foldIndianName`); `cityAliasPins` pins two aliases GeoNames hands to a neighbour
  (Visakhapatnam, Secunderabad). A line naming the US reads a colliding code as the state
  ("Hobart, IN, US" is Indiana). Measured 2026-10-25, 63,305 Indian postings: city 78.3% →
  84.4%; the residue (localities, office codes like "TPIN Malad", states alone) correctly
  states no city.
- It is a curated dictionary, not a geocoder: it resolves high-frequency names/shorthands and emits nothing for what it cannot resolve (never guesses).
- Region and work-mode values come from `vocab.RegionValues`/`vocab.WorkModeValues`, so parser, enrichment payload and search facet share one set.
- Geography is a Meilisearch facet (`regions`/`countries`/`work_mode` filterable), not a Postgres column filter.
- `work_mode` is dict-only: `jobview` serves the `jobs` column alone; the LLM's `enrichment.work_mode` is never merged (it stays raw in the JSONB).
- `countries`/`regions` are a dict-then-LLM hybrid (`jobview.geoFacet`), the one deliberate exception to dict-only: the dictionary wins when it pins a place; the LLM's `enrichment.countries`/`regions` fill only the unpinned (global/unspecified) bucket.

### Work-arrangement prose (description)

`WorkModeFromDescription` reads phrase lists in order English → German → Dutch → Spanish →
Portuguese → French → Croatian/Serbian (`workmode_bcs.go`) → Polish → Indian; a later list
runs only when no earlier one matched. German through Polish check remote FIRST (the
reverse of English): these languages state remote only as a totality that contains the
hybrid phrase. Dutch through Polish share `guardedPhraseMatches` (negation before,
perk/refusal guards after).

- **German** (`workmode_de.go`): home working is a BENEFIT, hence hybrid ("Homeoffice
  möglich", "2 Tage Homeoffice", "mobiles Arbeiten"); remote needs a totality ("100%
  Homeoffice", "vollständig remote"). Folded markup, word boundary at the end ("mobile
  Arbeitsmaschinen"), negation window before ("kein Homeoffice möglich"). Absent:
  bare `remote`/`hybrid`, `vor Ort`, `deutschlandweit`.
- **Dutch** (`workmode_nl.go`): "hybride werken", "2 dagen thuiswerken",
  "thuiswerkvergoeding" are hybrid; remote needs "volledig remote" / "100% thuiswerken". A
  phrase in the same sentence as "buitenland" or "per jaar" is a bounded perk and passed
  over. Absent: bare `hybride` (cloud), `op afstand`, `niet remote` (remote support).
  Measured 2026-10-25, 42k Dutch postings: work mode 3.5% → 16.4%.
- **Spanish** (`workmode_es.go`), on accent-folded text, order remote, hybrid, onsite:
  "100% remoto"/"100% teletrabajo" remote; "modalidad híbrida", "3 días presenciales",
  "2 días de home office", perks-list "teletrabajo" hybrid; "modalidad presencial", "100%
  presencial", "sin teletrabajo" onsite. Negations ("sin", "no", the hedge "evaluar"),
  refusals after ("no corresponde") and seasonal perks ("teletrabajo completo en verano")
  withdraw a phrase. Absent: bare `remoto`, `híbrido`, `presencial` (tutoring "clases
  presenciales", ~5k rows), `home office` (German and English prose reach this list).
  Phrases shared with Portuguese ("modelo híbrido") are read here. Argentina writes "esquema"/"jornada laboral", "Modalidad : híbrida" (spaced
  colon), and shares ("40% presencial", "80% remoto") or weekly visits ("1/2 asistencias
  semanales"), which are hybrid — read before the list under its guards
  (`spanishSplitWeek`). "Ubicación: Remoto" is absent (Torre's template, above hybrids). Measured 2026-10-27, 39k Spanish
  postings: work mode from any source 48.7% → 56.8%; agrees with the adapter's structured
  value on 86% of rows carrying both. Argentina (6,145 postings): 35.2% → 40.3%.
- **Portuguese** (`workmode_pt.go`): labelled lines ("Modelo de trabalho: híbrido",
  "Atuação: 100% remota", spaced colon too). "home office" is
  REMOTE as the model ("Modelo de trabalho: home office") and nothing as a benefit
  ("auxílio home office"); counted office days ("2x por semana presencial") are hybrid.
  Absent: bare `remoto`/`trabalho remoto`, `teletrabalho`, `sistema híbrido` (internship
  boilerplate on ~200 empregos postings), bare `presencial`/`modalidade presencial` (a course's modality). Location-line
  markers do read `remoto`, `híbrido`, `presencial` ("Remoto, Remoto, BR"). Shared-guard
  rules: a negation must START a word ("entorno", "bono", "humano" end in "no " and hid
  Spanish hybrids) and governs only its own clause ("não é 100% remota; modelo híbrido" is
  hybrid); the Spanish list also reads "não"/"sem", since Portuguese prose meets it first.
  Measured 2026-10-27, 30,677 BR + 6,931 PT postings: 4.9% → 14.7% (BR), 15.8% → 18.6% (PT);
  24 of 17,500 control rows changed, all correct.
- **French** (`workmode_fr.go`): home working is a BENEFIT, hybrid ("télétravail partiel
  possible", "2 jours de télétravail par semaine", "charte de télétravail"); remote needs a totality ("100 % télétravail", "télétravail
  total", "full-remote"); onsite is a refusal or totality ("pas de télétravail",
  "Télétravail : non", "100 % en présentiel").
  Absent: bare `télétravail`, bare `à distance`, `100 % à distance` and `distanciel`
  (apprenticeship school days, ~900 postings), `présentiel`, bare `hybride`, `mode hybride`
  and `modèle hybride` (vehicles, clouds), `télétravail : 100`
  ("prime télétravail : 100 € / mois"). Negations ("pas", "sans", "aucun") reach 24 bytes
  back; "-friendly" after a remote phrase withdraws it; "les fêtes" bounds a perk.
  Measured 2026-10-28, 24,609 French postings: 0.4% → 12.3% (with structured 31.1% → 34.8%).
- **Polish** (`workmode_pl.go`), diacritics folded, "ł" → "l": labelled lines ("Tryb pracy:
  zdalny", "Model pracy: hybrydowy"); solidjobs' "Praca zdalna: w całości / hybrydowo /
  możliwa częściowo / brak" = remote / hybrid / hybrid / onsite. "Możliwość pracy zdalnej"
  and "N dni w biurze" are hybrid; remote needs a totality ("100% zdalnie", "w pełni
  zdalna"). A choice of models and a share below 100% ("80% zdalnie") are hybrid, read
  before the list. Absent: bare `praca zdalna`/`zdalnie`, `stacjonarnie` (tutoring, shops),
  `hybrydowy` (clouds, cars). Hiring-process words ("Rekrutacja: 100% zdalna",
  "rozmowa") and platform/cloud context withdraw a phrase; "dni w roku" bounds a perk.
  Measured 2026-10-29, 9,835 Polish postings: 0.8% → 36.2% (with structured signal and
  location marker 24.4% → 46.4%); agrees with the structured mode in 1,205 of 1,323.
- **Indian** (`workmode_in.go`), last: "WFO"/"work from office" is onsite; counted days
  decide the rest ("3 days WFO", "Hybrid (3 days office)", "2 days WFH" → hybrid; "5 days
  work from office" → onsite); "hybrid" just before an office marker keeps it hybrid. Bare
  "WFH" is NOT remote (a benefit, a shift, HR's staff); only anchored forms ("permanent
  WFH", "WFH / Remote", "Location: WFH") are. Measured 2026-10-25: 8.6% → 10.7%.

### Per-country location lines

- **Dutch and Flemish** (`location_nl.go`, Dutch blocks of `nameToCountry` and
  `foreignSubdivisionNames`). Recruitee writes the country in the posting's language;
  "Nederland" was the Colorado town (10,801 open rows with no country, 2026-10-25).
  Provinces state the country and no city; `limburg` is in `regionNotCity` and states no
  country (Dutch, Belgian, and a German town). A province named after its capital
  (`provinceCapitals`) is the province when a town precedes it. `stripDutchProvince` cuts
  eures' province code after the town ("Hengelo OV"), a code shared with a US state or ISO
  country (NH, NB, UT, FR…) only when the town is Dutch. Also: "1012 AB" postcodes,
  "regio X", "X en omgeving", and Recruitee's translated remote label ("Werken op afstand")
  as a remote marker, whole line only.
- **Polish** (`location_pl.go`, Polish block of `foreignSubdivisionNames`; measured
  2026-10-29 over 15,488 PL postings: country already 99.6%, city ~97% of lines naming more
  than "Poland"). `postalPrefixRE` cuts eures' "02-797 Warszawa". "Trójmiasto"/"Tricity" and
  "Śląsk"/"GZM" are metros with no city (`polishMetroRegions`, the Delhi NCR shape; bare
  English "Silesia" is Czech too). A voivodeship alone states Poland. "Lódz"/"Łodź" are Łódź.
  Locatives ("w Warszawie") are not read: none in the sample.
- **French** (`location_fr.go`, French block of `foreignSubdivisionNames`; measured
  2026-10-28 over 33k FR postings). `hyphenFolded` reads a compound town written with spaces
  ("Issy les Moulineaux"), rewriting only a token no table knows as written. An
  arrondissement ("Lyon 3e") is the city; "Paris La Défense"
  is La Défense. A five-digit token is the named country's postcode, not a US ZIP, when the
  line names no part of the US or the caller's countries exclude it (`foreignPostcodeLine`).
  A small town's own hyphens are not a dash export ("Meslay-du-Maine" named Maine). Regions
  (current and pre-2016) and departments state France, no city; withheld: Vienne, Jura,
  Manche, Nord, Ardennes, Meuse. Martinique, Guadeloupe, Guyane, Réunion and French
  Polynesia keep their own ISO codes (placed in latam, africa and apac); New
  Caledonia and Mayotte are absent (their codes are North Carolina's and Yukon's). Catalogue-wide: a US state or Canadian province after a
  town the curated tables place abroad names the namesake and withdraws the other country
  ("Paris, TX", "London, ON"), unless the town is itself a province ("Saint-Laurent,
  Quebec, CA"); a held-back contested alias names the line's country's place when that
  country has exactly one so called (`contestedCityIn`: "Valence, France", "León, ES"; not
  Argentina's several San Martíns). A flag emoji is its country ("Lyon 🇫🇷"). FR lines:
  country 90.2% → 94.4%, wrong second country 0.4% → 0.2%, city 78.4% → 84.2%.
- **Spanish and Latin-American** (`location_es.go`, es/mx/co/ar/cl blocks of
  `foreignSubdivisionNames`; measured 2026-10-27 over 39k postings). Autonomous
  communities, the Basque provinces and the islands state Spain, no city; OCC's Mexican states
  state Mexico. A province named after its capital is the province when a place came
  before it (`provinceAfterCity` asks for a place, not a city, so an unresolved town
  counts): between two commas for Spain ("Oliva, Valencia, España"), in Factorial's
  "ESPAÑA/Province/Community", and also in the LAST slot for Mexico ("León, Guanajuato").
  Country names GeoNames gives a capital ("Mexico", "Panama", "Guatemala") and "D.C." are
  `regionNotCity` ("Monterrey, Mexico" named Mexico City too). **A contested alias whose
  city is named differently from the line is held back until the line states that city's
  country** (`addAliasCity`): "León, ES" was León de los Aldama, "Canton, Ohio" Guangzhou —
  ~6.7k wrong city names in the sample. Valencia and Zaragoza are city overrides. infojobs
  and occ carry `FallbackCountries` es/mx (28% and 16% of their postings stated no country).
- **Brazilian state codes and Portuguese districts** (`location_br.go`; measured 2026-10-27
  over 30,677 BR and 6,931 PT postings). Brazilian ATSs write the state code after the town
  ("Curitiba, PR", "Campinas SP"), and most codes spell a country or US
  state (PR Puerto Rico, BA Bosnia…). A code is the Brazilian state when the line names Brazil or the town has a Brazilian reading in any table —
  including a namesake another country gets first, whose country is withdrawn ("Colombo,
  PR", "Concordia, SC" → Concórdia). Not when the line names the US, nor after the country
  the code spells ("Juan Díaz, Panamá, PA"). With no town before it, a code is the state
  only when the next place is the WORD "Brasil"/"Brazil" ("RS, Brasil", gupy/solides with no
  city); "PE, BR" stays a list of countries. Rows stored before the fix keep their wrong
  country: `backfill-derive` keeps a stored one. Wrong country 20.1% → 1.1%; exactly
  Brazil 76.6% → 97.9%. Unambiguous state names state Brazil, no city
  (`foreignSubdivisionNames` + `regionNotCity`: "Ceará" named Fortaleza); names with a
  second reading (Paraná, Acre…) are left out. An Indian state
  code between an Indian city and the country is the state ("Patna, BR, in"). Portugal:
  "Porto" is a city override (it was Bordeaux's alias); a district named after its capital
  is the district between two commas (`portugueseDistrictCapitals`); Madeira and the Azores
  state Portugal. PT city 60.2% → 68.2%. Not built (not seen in the sample): metro-area
  names ("Grande São Paulo", "Grande Lisboa") and a CEP before the town.
- **Argentina** (`location_ar.go`; measured 2026-10-27 over ~12.3k AR postings). The
  autonomous city is "CABA", "Capital Federal"… (city overrides; "CABA" was Chamba's
  alias); pandapelatam writes a barrio ALONE ("Palermo"); conurbation towns sit under
  GeoNames' floor or a namesake ("Martínez" California). A barrio is the city of Buenos
  Aires, a Greater Buenos Aires town is itself, both in metro "Buenos Aires (AR)", only in a line with nothing but Buenos Aires
  context (`lineInBuenosAires`: "Palermo, Italia" keeps Italy); a name another Argentine
  place shares (`sharedArgentineNames`: Pilar, San Martín…) needs Buenos Aires itself in
  the line. GBA/AMBA state the country and metro, no city. A province named after its
  capital is the province between two commas of a line ending in Argentina.
  `metroLabelOverrides` renames GHS-FUA's suburb labels for Mendoza and San Juan.
  **`ParseIn`** carries the adapter's countries: when the line states none, it names a
  barrio, a contested or small town (one-place line only, so "Hermosillo, Sonora" does not
  name the town GeoNames calls Sonora) and the metro; it never adds a country.
  `FallbackCountries` is passed only once the fallback places the posting (a title's
  "[Remote-US]" outranks it). Catalogue-wide fixes from
  this sample: a colliding code after a US town only small places knows is the state
  ("Atkins, AR" was Argentina, 195 lines); a code between a town and a DIFFERENT country the
  town belongs to is that country's subdivision ("Zaragoza, AR, España" Aragón, "Jena, TH,
  Germany"). AR lines: city 65.9% → 75.6%, metro 48.9% → 69.1%, "Buenos Aires (AR)" 31.7% →
  44.2%.
- **Croatian, Serbian, Bosnian, Montenegrin** (`location_bcs.go`, merged into
  `nameToCountry`): country names in both standards, with/without diacritics and in Serbian
  Cyrillic ("Nemačka", "BiH"), so a posting abroad is not filed under the board's fallback; Workday's ISO-3 codes ("SRB"); the Croatian counties
  with and without "županija"; Vojvodina, Republika Srpska and Croatian regions (not
  "Istra", a Russian town too). "Kosovo" is `regionNotCity` (was Kosovo Polje).
  `cityOverrides` pins Pula, Dubrovnik, Leskovac, Sombor (GeoNames gave each alias to a
  hamlet abroad), locatives ("u Beogradu") and Cyrillic city names. "SAD" and "Island" are
  absent (English words). Sólides' "Town, ST" read "BA"/"RS" as Bosnia/Serbia, so the
  adapter appends "Brasil".
- **Slovenian** (`location_si.go`; measured 2026-10-29 over 2,900 MojeDelo postings and
  3,621 ESS vacancies). Both boards write the town alone with Slovenia as the structured
  country, so `ParseIn` names most towns via small places. Added: statistical and
  traditional regions, stating Slovenia, no city; "Celotna Slovenija" and countries in
  Slovenian; whole-line remote labels "Delo od doma"/"Delo na daljavo" (`remoteLabels`); area forms
  ("Ljubljana in okolica", "okolica Celja") via `areaPrefixes`/`areaSuffixes`; a Ljubljana
  district after the city; and
  seats/villages missing from cities1000 (`slovenianSmallPlaces`, city only beside
  Slovenia). "Ribnica" is Moldovan first, so a Slovenia-placed line names the Slovenian
  town through `contestedSmallPlaces` — a hand list, since small places keeps one row per
  name and country. Towns with TOWN STATUS below the main table's floor (`slovenianTowns`,
  51) are cities that state Slovenia when nothing else does, except nine another country
  also has (Jesenice, Piran…). Withheld (second readings): "Koroška", "Štajerska",
  "Primorska", "Kras", "ZDA", "Svica". MojeDelo placed 78.2% → 91.3%, city 77.3% →
  84.8%; ESS placed 98.2% → 100%.

## How it works

`jobderive.Derive` calls `ParseIn` with the adapter's countries; output reaches users through Meilisearch facets, so a dictionary change needs `cmd/backfill-derive` then `cmd/reindex`. The backfill keeps a stored `work_mode` (`internal/ingest/rederive` passes it back to `Derive`, which keeps a set mode; a `remote` explicitly contradicted by arrangement prose becomes `hybrid` (`HybridArrangementStated`, explicit work-model phrases and known-city hybrid headers, with negation, refusal and experience guards) or `onsite` (the existing remote-denial matcher)), so ordinary work-mode rule changes reach only rows with none. The contradiction tier also repairs existing remote rows on that pass; other corrections need a targeted repair before the reindex.

## Limitations

- **The country dictionary is incomplete: 138 of 252 ISO countries carry a region**
  (2026-11-08); the rest (Cuba, Syria, DR Congo, many dependencies) resolve to nothing. GeoNames `countryInfo.txt` supplies names, but regions
  are a business taxonomy (`uk` split from `eu`, `cis` spanning the Caucasus and Central
  Asia, `mena` across Africa and Asia), so a continent→region mapping would misplace
  Turkey, Egypt, Kazakhstan, the UK and Russia. Placing them is human work.
- **The next coverage gain is in tokenization, not the country list.** Over 100 production
  CV locations, all 38 countries were covered; unresolved strings failed on `·` not being a
  separator (`separatorReplacer`) and the LinkedIn `Greater X` prefix (deliberately not an
  area prefix: "Greater London" and "Greater Manchester" are subdivision names).
