package location

import (
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/vocab"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		location string
		want     Geo
	}{
		{
			name:     "named country yields code, region and remote mode",
			location: "Remote - Germany",
			want:     Geo{Countries: []string{"de"}, Regions: []string{"eu"}, WorkMode: "remote"},
		},
		{
			name:     "country shorthand USA",
			location: "Remote - USA",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, WorkMode: "remote"},
		},
		{
			name:     "plain country name states no work mode",
			location: "United States",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			name:     "macro region name yields region without country",
			location: "Remote - Europe",
			want:     Geo{Regions: []string{"eu"}, WorkMode: "remote"},
		},
		{
			// JobTech/Platsbanken ads end in ", Sverige" with an unknown municipality; the
			// native country word must resolve so they are not left geography-less.
			name:     "swedish native country word with unknown city",
			location: "Hallstahammar, Västmanlands län, Sverige",
			want:     Geo{Countries: []string{"se"}, Regions: []string{"eu"}},
		},
		{
			name:     "multiple locations union and dedup",
			location: "Remote - UK or Europe",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"eu", "uk"}, WorkMode: "remote"},
		},
		// LLM-mined city gaps: city-only strings the dict previously left geography-less.
		{
			name:     "polish spelling of warsaw resolves to PL",
			location: "Warszawa",
			want:     Geo{Countries: []string{"pl"}, Regions: []string{"eu"}, Cities: []string{"Warsaw"}},
		},
		{
			name:     "polish city with diacritics",
			location: "Łódź, Poland",
			want:     Geo{Countries: []string{"pl"}, Regions: []string{"eu"}, Cities: []string{"Łódź"}},
		},
		// Countries that carried a region in countryToRegion but no name in nameToCountry:
		// placeable once identified, yet unable to identify themselves. See
		// TestEveryPlaceableCountryHasAName, which guards the two maps against drifting apart.
		{
			name:     "honduras named in full resolves to its code and region",
			location: "San Pedro Sula, Honduras",
			want:     Geo{Countries: []string{"hn"}, Regions: []string{"latam"}, Cities: []string{"San Pedro Sula"}},
		},
		{
			name:     "rwanda named in full resolves to its code and region",
			location: "Kigali, Rwanda",
			want:     Geo{Countries: []string{"rw"}, Regions: []string{"africa"}, Cities: []string{"Kigali"}},
		},
		{
			// Before the country name existed, "laos" fell through to the long-tail city
			// branch and matched a Vietnamese city via its GeoNames alternate names. Naming
			// the country makes the token resolve first, and the city-agreement check then
			// rejects the unrelated city (la != vn) instead of emitting it.
			name:     "naming laos stops it resolving to an unrelated vietnamese city",
			location: "Laos",
			want:     Geo{Countries: []string{"la"}, Regions: []string{"apac"}},
		},
		// Regression guards for the added names: every added entry is a full word, and the
		// subdivision table is consulted BEFORE the bare two-letter country code, so the
		// US-state readings of these tokens must be untouched by the new countries la/mn/mo.
		{
			// A CONTESTED city name must not cost the subdivision code its reading.
			// "Taft" is claimed by Iran, the Philippines and the US, so the dictionary
			// states no country for it — but "CA" here is still California, resolved from
			// the preceding token rather than from the city's own country. Raised in
			// review as a suspected regression; measured identical before and after the
			// contested-alias change, and nailed down here so it stays that way.
			name:     "contested city keeps the state code readable",
			location: "Taft, CA",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Taft"}},
		},
		{
			name:     "LA stays louisiana rather than laos",
			location: "Baton Rouge, LA",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Baton Rouge"}},
		},
		{
			name:     "MN stays minnesota rather than mongolia",
			location: "Minneapolis, MN",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Minneapolis"}},
		},
		{
			name:     "MO stays missouri rather than macao",
			location: "St. Louis, MO",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"St. Louis"}},
		},
		{
			name:     "unambiguous UK city",
			location: "Manchester",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"Manchester"}},
		},
		{
			name:     "accented montreal resolves to CA",
			location: "Montréal, QC",
			want:     Geo{Countries: []string{"ca"}, Regions: []string{"north_america"}, Cities: []string{"Montreal"}},
		},
		{
			name:     "australian beacon city",
			location: "Brisbane",
			want:     Geo{Countries: []string{"au"}, Regions: []string{"apac"}, Cities: []string{"Brisbane"}},
		},
		{
			name:     "new zealand beacon city",
			location: "Auckland",
			want:     Geo{Countries: []string{"nz"}, Regions: []string{"apac"}, Cities: []string{"Auckland"}},
		},
		{
			// A remote job that resolves no geography at all is open-anywhere: it joins the
			// global bucket (its remoteness is still carried by WorkMode, which the separate
			// work-type facet filters on — the global region never displaces it).
			name:     "bare remote with no geography yields global",
			location: "Remote",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			name:     "explicit anywhere yields global and remote",
			location: "Remote - Anywhere",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			// The global fallback is driven by the detected work mode, not the literal word
			// "remote": a WFH marker with no place resolves the same way.
			name:     "work-from-home marker with no place yields global",
			location: "Work from home",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			name:     "international marker yields global and remote",
			location: "Remote - International",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			name:     "hybrid marker with city",
			location: "Hybrid - London",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"London"}, WorkMode: "hybrid"},
		},
		{
			name:     "onsite marker in parentheses keeps the city",
			location: "Berlin (On-site)",
			want:     Geo{Countries: []string{"de"}, Regions: []string{"eu"}, Cities: []string{"Berlin"}, WorkMode: "onsite"},
		},
		{
			name:     "hybrid wins over a remote marker in the same string",
			location: "Hybrid / Remote - London",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"London"}, WorkMode: "hybrid"},
		},
		{
			name:     "country buried among unknown tokens",
			location: "Burlington, Massachusetts, United States; Remote",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Burlington"}, WorkMode: "remote"},
		},
		{
			name:     "Central Asia: Uzbek district, city, country (Uzbek spelling)",
			location: "Yunusobod, Toshkent, Uzbekistan",
			want:     Geo{Countries: []string{"uz"}, Regions: []string{"cis"}, Cities: []string{"Tashkent", "Yunusobod"}},
		},
		{
			name:     "Central Asia: remote Kazakhstan",
			location: "Remote - Kazakhstan",
			want:     Geo{Countries: []string{"kz"}, Regions: []string{"cis"}, WorkMode: "remote"},
		},
		{
			name:     "CIS: Baku via city and country",
			location: "Baku, Azerbaijan",
			want:     Geo{Countries: []string{"az"}, Regions: []string{"cis"}, Cities: []string{"Baku"}},
		},
		{
			name:     "country-only Georgia is the US state, not the country (no false ge)",
			location: "Atlanta, Georgia, United States",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Atlanta"}},
		},
		{
			name:     "empty location",
			location: "",
			want:     Geo{},
		},
		{
			name:     "unresolvable token guesses nothing",
			location: "Zzqxville",
			want:     Geo{},
		},
		{
			// A hyphenated word whose first segment happens to be a 2-letter code
			// ("in") must not emit a phantom country: no other segment is geography.
			// It resolves no country, so the remote fallback puts it in global.
			name:     "hyphenated word is not a leading bare code",
			location: "Remote or in-house",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			// "De-Witt" (a place, but not a geo dash-export) must not add a phantom
			// "de" country; the real "NY" token still resolves.
			name:     "hyphenated place name keeps only the resolvable token",
			location: "De-Witt, NY",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			// A real dash-export with a bare leading code stays resolvable because a
			// following segment ("houston") is geography that corroborates it.
			name:     "geographic dash-export with bare leading code preserved",
			location: "TX-Houston",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			// Name-leading dash-export is unaffected by the bare-code gate.
			name:     "geographic dash-export with name leading segment preserved",
			location: "United States-Utah-Roy",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			// A hyphenated city whose inner segment is a bare code ("on", Ontario) never
			// misfires a phantom country: cityDict resolves the WHOLE name to the real
			// city, so the geography that lands is Stoke-on-Trent's own (gb/uk) and never
			// a stray "ca" from the "on" segment. That is the point of this case — the
			// country is right, not merely absent.
			name:     "hyphenated city does not misfire",
			location: "stoke-on-trent",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"Stoke-on-Trent"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.location)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.location, got, tt.want)
			}
		})
	}
}

// TestParseNorthAmerica covers the US "City, ST ZIP" and Canadian "City, Province"
// ATS formats: a trailing US state / Canadian province code or full name resolves
// the country (and its region) even when the city is unknown, and a US ZIP code is
// a standalone "us" signal. The country Georgia must never be misread as the US
// state (the code "ga" carries the state; the name stays out of the dictionary).
func TestParseNorthAmerica(t *testing.T) {
	tests := []struct {
		name     string
		location string
		want     Geo
	}{
		{
			name:     "US City, ST ZIP",
			location: "Lake Worth, TX 76135",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Lake Worth Beach"}},
		},
		{
			name:     "US City, ST",
			location: "Austin, TX",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Austin"}},
		},
		{
			name:     "US state code CA is California, not Canada",
			location: "San Francisco, CA",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"San Francisco"}},
		},
		{
			name:     "US full state name",
			location: "Remote - California",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, WorkMode: "remote"},
		},
		{
			name:     "US no-comma City ST",
			location: "Austin TX",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			name:     "bare US ZIP is a us signal",
			location: "94105",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			name:     "Canadian province code maps to north_america",
			location: "Toronto, ON",
			want:     Geo{Countries: []string{"ca"}, Regions: []string{"north_america"}, Cities: []string{"Toronto"}},
		},
		{
			name:     "Canadian full province name",
			location: "Vancouver, British Columbia",
			want:     Geo{Countries: []string{"ca"}, Regions: []string{"north_america"}, Cities: []string{"Vancouver"}},
		},
		{
			name:     "Washington DC resolves to us",
			location: "Washington, DC",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Washington"}},
		},
		{
			name:     "country Georgia is never misread as the US state",
			location: "Tbilisi, Georgia",
			want:     Geo{Countries: []string{"ge"}, Regions: []string{"cis"}, Cities: []string{"Tbilisi"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.location)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.location, got, tt.want)
			}
		})
	}
}

// TestParseSubdivisionCountryCodeCollision covers the ~16 US/Canada subdivision
// postal codes that are also a curated country's ISO code ("il" Illinois/Israel,
// "la" Louisiana/Laos, "pa" Pennsylvania/Panama, …). The subdivision reading must
// only win when the preceding city is a recognized US/CA place; otherwise the bare
// country code wins, so a foreign city sharing a code with a US state does not get
// mislabeled as that state.
func TestParseSubdivisionCountryCodeCollision(t *testing.T) {
	tests := []struct {
		name     string
		location string
		want     Geo
	}{
		{
			name:     "Tel Aviv, IL is Israel, not Illinois",
			location: "Tel Aviv, IL",
			want:     Geo{Countries: []string{"il"}, Regions: []string{"mena"}, Cities: []string{"Tel Aviv"}},
		},
		{
			name:     "Vientiane, LA is Laos, not Louisiana",
			location: "Vientiane, LA",
			want:     Geo{Countries: []string{"la"}, Regions: []string{"apac"}, Cities: []string{"Vientiane"}},
		},
		{
			name:     "Panama City, PA is Panama, not Pennsylvania",
			location: "Panama City, PA",
			want:     Geo{Countries: []string{"pa"}, Regions: []string{"latam"}, Cities: []string{"Panama City"}},
		},
		{
			// Regression guard: a genuine US city still wins its colliding state code
			// when the preceding token corroborates it.
			name:     "Chicago, IL stays Illinois, not Israel",
			location: "Chicago, IL",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Chicago"}},
		},
		{
			// Same collision, dash-delimited instead of comma-delimited: the lead
			// segment's context is the resolved TAIL, not a preceding comma-token.
			name:     "IL-Cupertino is California, not Israel",
			location: "IL-Cupertino",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.location)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.location, got, tt.want)
			}
		})
	}
}

// TestParseCyrillic covers the RU-segment ATS data, whose location fields are in
// Cyrillic ("Москва"), sometimes prefixed with the Russian city marker "г"
// ("г Москва"), and which name a remote/hybrid mode in Russian ("Удалённо").
func TestParseCyrillic(t *testing.T) {
	tests := []struct {
		name     string
		location string
		want     Geo
	}{
		{
			name:     "Cyrillic city Moscow",
			location: "Москва",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Moscow"}},
		},
		{
			name:     "city marker prefix is stripped",
			location: "г Москва",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Moscow"}},
		},
		{
			name:     "hyphenated Cyrillic city",
			location: "Санкт-Петербург",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Saint Petersburg"}},
		},
		{
			name:     "multi-word Cyrillic city",
			location: "Нижний Новгород",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Nizhniy Novgorod"}},
		},
		{
			name:     "country token Россия resolves even past an unknown city",
			location: "Энск, Россия",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}},
		},
		{
			name:     "abbreviation РФ",
			location: "РФ",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}},
		},
		{
			name:     "Россия with parenthesised remote marker",
			location: "Россия (удалённо)",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, WorkMode: "remote"},
		},
		{
			name:     "bare Удалённо yields remote mode and global",
			location: "Удалённо",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			name:     "Cyrillic hybrid marker with city",
			location: "Москва, гибрид",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Moscow"}, WorkMode: "hybrid"},
		},
		{
			name:     "CIS: Minsk maps to Belarus / cis",
			location: "Минск",
			want:     Geo{Countries: []string{"by"}, Regions: []string{"cis"}, Cities: []string{"Minsk"}},
		},
		{
			name:     "Central Asia: Tashkent maps to Uzbekistan",
			location: "Ташкент",
			want:     Geo{Countries: []string{"uz"}, Regions: []string{"cis"}, Cities: []string{"Tashkent"}},
		},
		{
			name:     "Ukrainian spelling Київ maps to Ukraine / eu",
			location: "Київ",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Kyiv"}},
		},
		{
			name:     "Ukrainian spelling Львів maps to Ukraine / eu",
			location: "Львів",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Lviv"}},
		},
		{
			name:     "Russian spelling Харьков maps to Ukraine / eu",
			location: "Харьков",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Kharkiv"}},
		},
		{
			name:     "Latin Lviv maps to Ukraine / eu without a country token",
			location: "Lviv",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Lviv"}},
		},
		{
			name:     "Ukrainian spelling of the country yields the code without a city",
			location: "Україна",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}},
		},
		{
			name:     "Ukrainian city marker м. is stripped like the Russian г.",
			location: "м. Львів",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Lviv"}},
		},
		{
			// The leading "г" is part of the city name, not the "г." city marker. The
			// name resolves whole, so its own geography (ru/cis) is what lands.
			name:     "city starting with г is not mistaken for the marker",
			location: "Грозный",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Grozny"}},
		},
		{
			name:     "city starting with м is not mistaken for the marker",
			location: "Мурманск",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Murmansk"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.location)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.location, got, tt.want)
			}
		})
	}
}

// TestParseEmitsOnlyKnownVocabulary guards the controlled-vocabulary invariant:
// every region the parser emits is a member of vocab.RegionValues and every
// work mode a member of vocab.WorkModeValues — the parser never invents a value
// outside the enrichment contract's vocabularies.
func TestParseEmitsOnlyKnownVocabulary(t *testing.T) {
	samples := []string{
		"Remote - Germany", "Remote - UK or Europe", "Remote - Anywhere",
		"Remote - USA", "Remote - Singapore", "Remote - Canada",
		"Hybrid - London", "Berlin (On-site)",
		"Burlington, Massachusetts, United States; Remote", "Remote", "",
	}
	for _, s := range samples {
		got := Parse(s)
		for _, r := range got.Regions {
			if !slices.Contains(vocab.RegionValues, r) {
				t.Errorf("Parse(%q) emitted region %q outside RegionValues", s, r)
			}
		}
		if got.WorkMode != "" && !slices.Contains(vocab.WorkModeValues, got.WorkMode) {
			t.Errorf("Parse(%q) emitted work_mode %q outside WorkModeValues", s, got.WorkMode)
		}
	}
}

// TestParseExpandedCoverage exercises the dictionary expansion: trailing ISO
// country codes, beacon cities, multilingual country names, multilingual
// open-anywhere markers, and work-mode-word stripping inside a token.
func TestParseExpandedCoverage(t *testing.T) {
	tests := []struct {
		location string
		want     Geo
	}{
		// Trailing bare ISO 3166-1 alpha-2 code ("City, Region, code").
		{"Shanghai, Shanghai, cn", Geo{Countries: []string{"cn"}, Regions: []string{"apac"}, Cities: []string{"Shanghai"}}},
		{"Riyadh, sa", Geo{Countries: []string{"sa"}, Regions: []string{"mena"}, Cities: []string{"Riyadh"}}},
		{"Lisboa, Lisboa, pt", Geo{Countries: []string{"pt"}, Regions: []string{"eu"}, Cities: []string{"Lisbon"}}},
		{"São Paulo, SP, br", Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"São Paulo"}}},
		// Beacon cities.
		{"San Francisco", Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"San Francisco"}}},
		{"Athens, Attica, Greece", Geo{Countries: []string{"gr"}, Regions: []string{"eu"}, Cities: []string{"Athens"}}},
		{"Seoul, South Korea", Geo{Countries: []string{"kr"}, Regions: []string{"apac"}, Cities: []string{"Seoul"}}},
		// Country names: English + native + ES/PT/DE.
		{"China", Geo{Countries: []string{"cn"}, Regions: []string{"apac"}}},
		{"Brasil", Geo{Countries: []string{"br"}, Regions: []string{"latam"}}},
		{"España", Geo{Countries: []string{"es"}, Regions: []string{"eu"}}},
		{"Grécia", Geo{Countries: []string{"gr"}, Regions: []string{"eu"}}},
		// Open-anywhere markers, multilingual.
		{"World Wide - Remote", Geo{Regions: []string{"global"}, WorkMode: "remote"}},
		{"по всему миру", Geo{Regions: []string{"global"}}},
		{"weltweit", Geo{Regions: []string{"global"}}},
		// Work-mode word stripped so the place still resolves.
		{"US Remote", Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, WorkMode: "remote"}},
	}
	for _, tt := range tests {
		got := Parse(tt.location)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.location, got, tt.want)
		}
	}
}

// TestParseCityFacetExpansion covers the GeoNames-backed city dictionary: a resolved
// city emits both its canonical facet value and its country/region, a genuinely
// long-tail city works, GeoNames alternate-name noise is rejected, stoplisted markers
// never become cities, and a curated override spelling wins.
func TestParseCityFacetExpansion(t *testing.T) {
	tests := []struct {
		name     string
		location string
		want     Geo
	}{
		{
			// The motivating case: a city that used to resolve a country but no city.
			name:     "curated city emits facet + country + region",
			location: "Florianópolis",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Florianópolis"}},
		},
		{
			name:     "city with embedded country still resolves the facet",
			location: "Florianópolis, Brazil",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Florianópolis"}},
		},
		{
			// A long-tail city absent from the hand-curated maps. "Recife" is claimed by
			// exactly one country in the dataset, so it now states br/latam on its own —
			// no country token needed. Nothing is guessed: a name two countries share
			// still states nothing (see the contested cases below).
			name:     "unambiguous long-tail city states its own country",
			location: "Recife",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Recife"}},
		},
		{
			// With an explicit country token the geography resolves deterministically too.
			name:     "long-tail city with a country token resolves geography",
			location: "Recife, Brazil",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Recife"}},
		},
		{
			// A name two countries share states NO country. This is the guard on the
			// unambiguous-city rule: without it "most populous wins" would file a
			// Birmingham (UK) role under the US, which is worse than filing it nowhere.
			name:     "contested city name states no country",
			location: "Birmingham",
			want:     Geo{Cities: []string{"Birmingham"}},
		},
		{
			// Canada and the US both have a Burlington; the same silence applies. (A
			// contested name the CURATED maps already pin — "San Jose", "Valencia" —
			// still resolves from them: this rule only fills what they left empty.)
			name:     "contested city name states no country, second case",
			location: "Burlington",
			want:     Geo{Cities: []string{"Burlington"}},
		},
		{
			// A country token resolves a contested name — the token is authoritative,
			// the city merely agrees with it.
			name:     "contested city with a country token resolves",
			location: "Birmingham, UK",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"Birmingham"}},
		},
		{
			// The city is the LAST word, never a contributing one. "Anna" is a town in
			// Russia; the state and country in the same line are what place this job,
			// and the city must not add a second country beside them. Measured against
			// production, this rule is what took the change from 9 wrong countries per
			// 2000 postings down to 3.
			name:     "stated country wins over a long-tail city elsewhere in the line",
			location: "Anna, Illinois, United States",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Anna"}},
		},
		{
			// Same rule with the signal on the other side of the separator: "Crossroads"
			// is an alias of Woonsocket, RI, but London is what this line is about, so
			// only gb lands. Woonsocket still reaches the CITIES facet — that is
			// pre-existing behaviour and harmless, since a stray city name narrows a
			// search while a stray country would misfile the posting entirely.
			name:     "stated country wins over a long-tail city before it",
			location: "Crossroads - London",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"London", "Woonsocket"}},
		},
		{
			// With no other signal in the line, the city is all there is — and it speaks.
			// This is the case the whole rule exists for: ~39% of geographically
			// unpinned production postings name a real place and nothing else.
			name:     "lone long-tail city speaks when nothing else does",
			location: "Colorado Springs",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Colorado Springs"}},
		},
		{
			// Two individually-unambiguous cities in different countries make the LINE
			// ambiguous. Taking the first would make the answer depend on word order,
			// so they cancel and state nothing. Both orders are asserted because
			// order-dependence is exactly the bug being guarded against.
			name:     "disagreeing long-tail cities state no country",
			location: "Recife, Benidorm",
			want:     Geo{Cities: []string{"Benidorm", "Recife"}},
		},
		{
			name:     "disagreeing long-tail cities state no country, reversed",
			location: "Benidorm, Recife",
			want:     Geo{Cities: []string{"Benidorm", "Recife"}},
		},
		{
			// Agreement is about the country, not the name: two different cities in the
			// same country still speak with one voice.
			name:     "agreeing long-tail cities still state their shared country",
			location: "Colorado Springs, Eden Prairie",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Colorado Springs", "Eden Prairie"}},
		},
		{
			// "usa" appears among a Japanese city's GeoNames alternate names; the
			// country-agreement guard must reject that city while keeping the country.
			name:     "GeoNames alternate-name noise is rejected",
			location: "USA",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			// A stoplisted open-anywhere marker is never a city; it stays a region signal.
			name:     "stoplisted marker never becomes a city",
			location: "Worldwide",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			// The curated override pins the English facet spelling over GeoNames "Köln".
			name:     "curated override spelling wins",
			location: "Köln",
			want:     Geo{Countries: []string{"de"}, Regions: []string{"eu"}, Cities: []string{"Cologne"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.location)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.location, got, tt.want)
			}
		})
	}
}
