# Location conventions: per-country location lines

Part of [AGENT-REFERENCE.md](AGENT-REFERENCE.md): sections moved out of it for size.

## Per-country location lines

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
