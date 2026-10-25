package location

import (
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
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
			// native country word must resolve so they are not left geography-less. The
			// municipality is below the main city table's floor, so it is named through the
			// small-places table once the country word has placed the line.
			name:     "swedish native country word with small municipality",
			location: "Hallstahammar, Västmanlands län, Sverige",
			want:     Geo{Countries: []string{"se"}, Regions: []string{"eu"}, Cities: []string{"Hallstahammar"}},
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
			want:     Geo{Countries: []string{"pl"}, Regions: []string{"eu"}, Cities: []string{"Warsaw"}, Metros: []string{"Warsaw (PL)"}},
		},
		{
			name:     "polish city with diacritics",
			location: "Łódź, Poland",
			want:     Geo{Countries: []string{"pl"}, Regions: []string{"eu"}, Cities: []string{"Łódź"}, Metros: []string{"Łódź (PL)"}},
		},
		// Countries that carried a region in countryToRegion but no name in nameToCountry:
		// placeable once identified, yet unable to identify themselves. See
		// TestEveryPlaceableCountryHasAName, which guards the two maps against drifting apart.
		{
			name:     "honduras named in full resolves to its code and region",
			location: "San Pedro Sula, Honduras",
			want:     Geo{Countries: []string{"hn"}, Regions: []string{"latam"}, Cities: []string{"San Pedro Sula"}, Metros: []string{"San Pedro Sula (HN)"}},
		},
		{
			name:     "rwanda named in full resolves to its code and region",
			location: "Kigali, Rwanda",
			want:     Geo{Countries: []string{"rw"}, Regions: []string{"africa"}, Cities: []string{"Kigali"}, Metros: []string{"Kigali (RW)"}},
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
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Baton Rouge"}, Metros: []string{"Baton Rouge (US)"}},
		},
		{
			name:     "MN stays minnesota rather than mongolia",
			location: "Minneapolis, MN",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Minneapolis"}, Metros: []string{"Minneapolis (US)"}},
		},
		{
			name:     "MO stays missouri rather than macao",
			location: "St. Louis, MO",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"St. Louis"}, Metros: []string{"St. Louis (US)"}},
		},
		{
			name:     "unambiguous UK city",
			location: "Manchester",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"Manchester"}, Metros: []string{"Manchester (GB)"}},
		},
		{
			name:     "accented montreal resolves to CA",
			location: "Montréal, QC",
			want:     Geo{Countries: []string{"ca"}, Regions: []string{"north_america"}, Cities: []string{"Montreal"}, Metros: []string{"Montréal (CA)"}},
		},
		{
			name:     "australian beacon city",
			location: "Brisbane",
			want:     Geo{Countries: []string{"au"}, Regions: []string{"apac"}, Cities: []string{"Brisbane"}, Metros: []string{"Brisbane (AU)"}},
		},
		{
			name:     "new zealand beacon city",
			location: "Auckland",
			want:     Geo{Countries: []string{"nz"}, Regions: []string{"apac"}, Cities: []string{"Auckland"}, Metros: []string{"Auckland (NZ)"}},
		},
		{
			// A remote job that resolves no geography at all states nothing about where
			// it is open to; it stays in the "region not specified" bucket (regions=none),
			// not global. Global is reserved for an explicit open-anywhere marker (below).
			name:     "bare remote with no geography yields no region",
			location: "Remote",
			want:     Geo{WorkMode: "remote"},
		},
		{
			name:     "explicit anywhere yields global and remote",
			location: "Remote - Anywhere",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			// A WFH marker with no place is the same "nothing stated" case as a bare
			// "Remote" location: "wfh" is a work-mode marker, not an open-anywhere one.
			name:     "work-from-home marker with no place yields no region",
			location: "Work from home",
			want:     Geo{WorkMode: "remote"},
		},
		{
			name:     "international marker yields global and remote",
			location: "Remote - International",
			want:     Geo{Regions: []string{"global"}, WorkMode: "remote"},
		},
		{
			name:     "hybrid marker with city",
			location: "Hybrid - London",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"London"}, WorkMode: "hybrid", Metros: []string{"London (GB)"}},
		},
		{
			name:     "onsite marker in parentheses keeps the city",
			location: "Berlin (On-site)",
			want:     Geo{Countries: []string{"de"}, Regions: []string{"eu"}, Cities: []string{"Berlin"}, WorkMode: "onsite", Metros: []string{"Berlin (DE)"}},
		},
		{
			name:     "hybrid wins over a remote marker in the same string",
			location: "Hybrid / Remote - London",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"London"}, WorkMode: "hybrid", Metros: []string{"London (GB)"}},
		},
		{
			name:     "country buried among unknown tokens",
			location: "Burlington, Massachusetts, United States; Remote",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Burlington"}, WorkMode: "remote", Metros: []string{"Boston (US)"}},
		},
		{
			name:     "Central Asia: Uzbek district, city, country (Uzbek spelling)",
			location: "Yunusobod, Toshkent, Uzbekistan",
			want:     Geo{Countries: []string{"uz"}, Regions: []string{"cis"}, Cities: []string{"Tashkent", "Yunusobod"}, Metros: []string{"Tashkent (UZ)"}},
		},
		{
			name:     "Central Asia: remote Kazakhstan",
			location: "Remote - Kazakhstan",
			want:     Geo{Countries: []string{"kz"}, Regions: []string{"cis"}, WorkMode: "remote"},
		},
		{
			name:     "CIS: Baku via city and country",
			location: "Baku, Azerbaijan",
			want:     Geo{Countries: []string{"az"}, Regions: []string{"cis"}, Cities: []string{"Baku"}, Metros: []string{"Baku (AZ)"}},
		},
		{
			name:     "country-only Georgia is the US state, not the country (no false ge)",
			location: "Atlanta, Georgia, United States",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Atlanta"}, Metros: []string{"Atlanta (US)"}},
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
			// It resolves no country, so the location states no region either.
			name:     "hyphenated word is not a leading bare code",
			location: "Remote or in-house",
			want:     Geo{WorkMode: "remote"},
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
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"Stoke-on-Trent"}, Metros: []string{"Stoke-on-Trent (GB)"}},
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
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Austin"}, Metros: []string{"Austin (US)"}},
		},
		{
			name:     "US state code CA is California, not Canada",
			location: "San Francisco, CA",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"San Francisco"}, Metros: []string{"San Francisco Bay Area (US)"}},
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
			want:     Geo{Countries: []string{"ca"}, Regions: []string{"north_america"}, Cities: []string{"Toronto"}, Metros: []string{"Toronto (CA)"}},
		},
		{
			name:     "Canadian full province name",
			location: "Vancouver, British Columbia",
			want:     Geo{Countries: []string{"ca"}, Regions: []string{"north_america"}, Cities: []string{"Vancouver"}, Metros: []string{"Vancouver (CA)"}},
		},
		{
			name:     "Washington DC resolves to us",
			location: "Washington, DC",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Washington"}, Metros: []string{"Washington (US)"}},
		},
		{
			name:     "country Georgia is never misread as the US state",
			location: "Tbilisi, Georgia",
			want:     Geo{Countries: []string{"ge"}, Regions: []string{"cis"}, Cities: []string{"Tbilisi"}, Metros: []string{"Tbilisi (GE)"}},
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
			want:     Geo{Countries: []string{"il"}, Regions: []string{"mena"}, Cities: []string{"Tel Aviv"}, Metros: []string{"Tel Aviv (IL)"}},
		},
		{
			name:     "Vientiane, LA is Laos, not Louisiana",
			location: "Vientiane, LA",
			want:     Geo{Countries: []string{"la"}, Regions: []string{"apac"}, Cities: []string{"Vientiane"}, Metros: []string{"Vientiane (LA)"}},
		},
		{
			name:     "Panama City, PA is Panama, not Pennsylvania",
			location: "Panama City, PA",
			want:     Geo{Countries: []string{"pa"}, Regions: []string{"latam"}, Cities: []string{"Panama City"}, Metros: []string{"Panama City (PA)"}},
		},
		{
			// Regression guard: a genuine US city still wins its colliding state code
			// when the preceding token corroborates it.
			name:     "Chicago, IL stays Illinois, not Israel",
			location: "Chicago, IL",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Chicago"}, Metros: []string{"Chicago (US)"}},
		},
		{
			// Same collision, dash-delimited instead of comma-delimited: the lead
			// segment's context is the resolved TAIL, not a preceding comma-token.
			name:     "IL-Cupertino is California, not Israel",
			location: "IL-Cupertino",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			name:     "Carmel, IN stays Indiana, not India",
			location: "Carmel, IN",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Carmel"}, Metros: []string{"Indianapolis (US)"}},
		},
		{
			name:     "Brownsburg, IN stays Indiana, not India",
			location: "Brownsburg, IN",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Brownsburg"}, Metros: []string{"Indianapolis (US)"}},
		},
		{
			name:     "Auburn, IN, US stays Indiana, not India",
			location: "Auburn, IN, US",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Auburn"}},
		},
		{
			name:     "Wilmington, DE stays Delaware, not Germany",
			location: "Wilmington, DE",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Wilmington"}, Metros: []string{"Philadelphia (US)"}},
		},
		{
			name:     "Boise, ID stays Idaho, not Indonesia",
			location: "Boise, ID",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Boise"}, Metros: []string{"Boise (US)"}},
		},
		{
			name:     "Bangalore, IN stays India, not Indiana",
			location: "Bangalore, IN",
			want:     Geo{Countries: []string{"in"}, Regions: []string{"apac"}, Cities: []string{"Bengaluru"}, Metros: []string{"Bengaluru (IN)"}},
		},
		{
			name:     "Mumbai, MH, IN stays India, not Indiana",
			location: "Mumbai, MH, IN",
			want:     Geo{Countries: []string{"in"}, Regions: []string{"apac"}, Cities: []string{"Mumbai"}, Metros: []string{"Mumbai (IN)"}},
		},
		{
			name:     "Berlin, DE stays Germany, not Delaware",
			location: "Berlin, DE",
			want:     Geo{Countries: []string{"de"}, Regions: []string{"eu"}, Cities: []string{"Berlin"}, Metros: []string{"Berlin (DE)"}},
		},
		{
			name:     "Jakarta, ID stays Indonesia, not Idaho",
			location: "Jakarta, ID",
			want:     Geo{Countries: []string{"id"}, Regions: []string{"apac"}, Cities: []string{"Jakarta"}, Metros: []string{"Jakarta (ID)"}},
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
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Moscow"}, Metros: []string{"Moscow (RU)"}},
		},
		{
			name:     "city marker prefix is stripped",
			location: "г Москва",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Moscow"}, Metros: []string{"Moscow (RU)"}},
		},
		{
			name:     "hyphenated Cyrillic city",
			location: "Санкт-Петербург",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Saint Petersburg"}, Metros: []string{"Saint Petersburg (RU)"}},
		},
		{
			name:     "multi-word Cyrillic city",
			location: "Нижний Новгород",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Nizhniy Novgorod"}, Metros: []string{"Nizhniy Novgorod (RU)"}},
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
			name:     "bare Удалённо yields remote mode, no region",
			location: "Удалённо",
			want:     Geo{WorkMode: "remote"},
		},
		{
			name:     "Cyrillic hybrid marker with city",
			location: "Москва, гибрид",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Moscow"}, WorkMode: "hybrid", Metros: []string{"Moscow (RU)"}},
		},
		{
			name:     "CIS: Minsk maps to Belarus / cis",
			location: "Минск",
			want:     Geo{Countries: []string{"by"}, Regions: []string{"cis"}, Cities: []string{"Minsk"}, Metros: []string{"Minsk (BY)"}},
		},
		{
			name:     "Central Asia: Tashkent maps to Uzbekistan",
			location: "Ташкент",
			want:     Geo{Countries: []string{"uz"}, Regions: []string{"cis"}, Cities: []string{"Tashkent"}, Metros: []string{"Tashkent (UZ)"}},
		},
		{
			name:     "Ukrainian spelling Київ maps to Ukraine / eu",
			location: "Київ",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Kyiv"}, Metros: []string{"Kyiv (UA)"}},
		},
		{
			name:     "Ukrainian spelling Львів maps to Ukraine / eu",
			location: "Львів",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Lviv"}, Metros: []string{"Lviv (UA)"}},
		},
		{
			name:     "Russian spelling Харьков maps to Ukraine / eu",
			location: "Харьков",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Kharkiv"}, Metros: []string{"Kharkiv (UA)"}},
		},
		{
			name:     "Latin Lviv maps to Ukraine / eu without a country token",
			location: "Lviv",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Lviv"}, Metros: []string{"Lviv (UA)"}},
		},
		{
			name:     "Ukrainian spelling of the country yields the code without a city",
			location: "Україна",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}},
		},
		{
			name:     "Ukrainian city marker м. is stripped like the Russian г.",
			location: "м. Львів",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}, Cities: []string{"Lviv"}, Metros: []string{"Lviv (UA)"}},
		},
		{
			// The leading "г" is part of the city name, not the "г." city marker. The
			// name resolves whole, so its own geography (ru/cis) is what lands.
			name:     "city starting with г is not mistaken for the marker",
			location: "Грозный",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Grozny"}, Metros: []string{"Grozny (RU)"}},
		},
		{
			name:     "city starting with м is not mistaken for the marker",
			location: "Мурманск",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, Cities: []string{"Murmansk"}, Metros: []string{"Murmansk (RU)"}},
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
		{"Shanghai, Shanghai, cn", Geo{Countries: []string{"cn"}, Regions: []string{"apac"}, Cities: []string{"Shanghai"}, Metros: []string{"Shanghai (CN)"}}},
		{"Riyadh, sa", Geo{Countries: []string{"sa"}, Regions: []string{"mena"}, Cities: []string{"Riyadh"}, Metros: []string{"Riyadh (SA)"}}},
		{"Lisboa, Lisboa, pt", Geo{Countries: []string{"pt"}, Regions: []string{"eu"}, Cities: []string{"Lisbon"}, Metros: []string{"Lisbon (PT)"}}},
		{"São Paulo, SP, br", Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"São Paulo"}, Metros: []string{"São Paulo (BR)"}}},
		// Beacon cities.
		{"San Francisco", Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"San Francisco"}, Metros: []string{"San Francisco Bay Area (US)"}}},
		{"Athens, Attica, Greece", Geo{Countries: []string{"gr"}, Regions: []string{"eu"}, Cities: []string{"Athens"}, Metros: []string{"Athens (GR)"}}},
		{"Seoul, South Korea", Geo{Countries: []string{"kr"}, Regions: []string{"apac"}, Cities: []string{"Seoul"}, Metros: []string{"Seoul (KR)"}}},
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
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Florianópolis"}, Metros: []string{"Florianópolis (BR)"}},
		},
		{
			name:     "city with embedded country still resolves the facet",
			location: "Florianópolis, Brazil",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Florianópolis"}, Metros: []string{"Florianópolis (BR)"}},
		},
		{
			// A long-tail city absent from the hand-curated maps. "Recife" is claimed by
			// exactly one country in the dataset, so it now states br/latam on its own —
			// no country token needed. Nothing is guessed: a name two countries share
			// still states nothing (see the contested cases below).
			name:     "unambiguous long-tail city states its own country",
			location: "Recife",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Recife"}, Metros: []string{"Recife (BR)"}},
		},
		{
			// With an explicit country token the geography resolves deterministically too.
			name:     "long-tail city with a country token resolves geography",
			location: "Recife, Brazil",
			want:     Geo{Countries: []string{"br"}, Regions: []string{"latam"}, Cities: []string{"Recife"}, Metros: []string{"Recife (BR)"}},
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
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"Birmingham"}, Metros: []string{"Birmingham (GB)"}},
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
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, Cities: []string{"London", "Woonsocket"}, Metros: []string{"London (GB)"}},
		},
		{
			// With no other signal in the line, the city is all there is — and it speaks.
			// This is the case the whole rule exists for: ~39% of geographically
			// unpinned production postings name a real place and nothing else.
			name:     "lone long-tail city speaks when nothing else does",
			location: "Colorado Springs",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Colorado Springs"}, Metros: []string{"Colorado Springs (US)"}},
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
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, Cities: []string{"Colorado Springs", "Eden Prairie"}, Metros: []string{"Colorado Springs (US)", "Minneapolis (US)"}},
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
			// The curated override pins the English facet spelling over GeoNames "Köln", and
			// the metro table, keyed by the GeoNames name, still finds the city.
			name:     "curated override spelling wins",
			location: "Köln",
			want:     Geo{Countries: []string{"de"}, Regions: []string{"eu"}, Cities: []string{"Cologne"}, Metros: []string{"Köln (DE)"}},
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

// TestParseStateFirstSubdivisions covers the colliding codes whose US-state reading
// dominates this catalogue (measured 2026-10-21 by cmd/report-geo-gaps: of the open
// postings stored under Morocco for a trailing ", MA", the model read 168 of 227 as
// US; Moldova/MD 131 of 132; Panama/PA 227 of 239). For these, a city the
// dictionary does not place in the colliding COUNTRY is enough to read the state —
// "Cambridge, MA" is Massachusetts even though GeoNames calls Cambridge British.
// The country reading still wins when the city is that country's own.
func TestParseStateFirstSubdivisions(t *testing.T) {
	tests := []struct {
		location string
		want     []string
	}{
		// The state, now.
		{"Cambridge, MA", []string{"us"}},
		{"Birmingham, AL", []string{"us"}},
		{"Montgomery County, PA", []string{"us"}},
		{"Annapolis Junction, MD", []string{"us"}},
		{"Fort Meade, MD", []string{"us"}},
		{"Oak Brook, IL", []string{"us"}},
		{"Lone Tree, MN", []string{"us"}},
		{"Statewide, LA", []string{"us"}},
		{"Slu Saint Louis, MO", []string{"us"}},
		{"Portland, ME", []string{"us"}},
		{"Brighton MA", []string{"us"}},
		// The country, still: the city is that country's own.
		{"Tel Aviv, IL", []string{"il"}},
		{"Haifa, IL", []string{"il"}},
		{"Casablanca, MA", []string{"ma"}},
		{"Rabat, MA", []string{"ma"}},
		{"Tirana, AL", []string{"al"}},
		{"Chisinau, MD", []string{"md"}},
		{"Panama City, PA", []string{"pa"}},
		{"Ulaanbaatar, MN", []string{"mn"}},
		{"Podgorica, ME", []string{"me"}},
		{"Vientiane, LA", []string{"la"}},
		// A bare code has no city to judge by and keeps its old reading.
		{"MA", []string{"ma"}},
		{"IL", []string{"il"}},
		// Codes whose COUNTRY reading dominates are untouched: an unknown city
		// beside them still reads as the country.
		{"Unknownville, DE", []string{"de"}},
		{"Unknownville, IN", []string{"in"}},
		{"Unknownville, CO", []string{"co"}},
	}
	for _, tt := range tests {
		t.Run(tt.location, func(t *testing.T) {
			got := Parse(tt.location).Countries
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q).Countries = %v, want %v", tt.location, got, tt.want)
			}
		})
	}
}

// TestStateFirstSubdivisionsAreColliding guards the set against dead entries: a
// code that is not a colliding subdivision never reaches the check that reads it.
func TestStateFirstSubdivisionsAreColliding(t *testing.T) {
	for code := range stateFirstSubdivisions {
		if _, ok := collidingSubdivisions[code]; !ok {
			t.Errorf("stateFirstSubdivisions[%q] is not a colliding subdivision code", code)
		}
	}
}

func TestParseUKPostcode(t *testing.T) {
	for _, tc := range []struct {
		location string
		want     []string
	}{
		// The postcode states the country; the place it follows still resolves.
		{"London SW1A 2HB", []string{"gb"}},
		{"This position is based at HM Prison Frankland, Durham, DH1 5YD.", []string{"gb"}},
		{"Birmingham, West Midlands (England), B1 2AX", []string{"gb"}},
		{"NMS Church House, HD1 1SG, NMS LG, The Church View Centre, DN1 1AF", []string{"gb"}},
		{"Belfast BT1 5GS", []string{"gb"}},
		// England's statistical regions name the country without a postcode.
		{"Durham, North East England", []string{"gb"}},
		{"East of England, Yorkshire and the Humber", []string{"gb"}},
		// Not postcodes: an outward code alone, an ordinal, a Crown dependency, a US ZIP.
		{"Unit B2 2nd floor", nil},
		{"St Helier JE2 3AB", nil},
		{"Cambridge, MA 02139", []string{"us"}},
		// Durham has two readings and is not guessed; the state settles it.
		{"Durham", nil},
		{"Durham, NC", []string{"us"}},
	} {
		if got := Parse(tc.location).Countries; !slices.Equal(got, tc.want) {
			t.Errorf("Parse(%q).Countries = %v, want %v", tc.location, got, tc.want)
		}
	}
}

// TestParseAustralianStateCodes covers the "City STATE" form SEEK writes ("Perth WA",
// "Sydney NSW"): one token with no comma, whose trailing code used to be read against the
// US/Canada table alone, so WA became Washington and NT the Northwest Territories.
func TestParseAustralianStateCodes(t *testing.T) {
	tests := []struct {
		location string
		want     []string
	}{
		// Codes only Australia uses resolve after any place name.
		{"Sydney NSW", []string{"au"}},
		{"Melbourne VIC", []string{"au"}},
		{"Brisbane QLD", []string{"au"}},
		{"Hobart TAS", []string{"au"}},
		{"Bondi Junction NSW", []string{"au"}},
		// Codes another country shares resolve to Australia only beside an Australian city.
		{"Perth WA", []string{"au"}},
		{"Perth, WA", []string{"au"}},
		{"Osborne Park, Perth WA", []string{"au"}},
		{"Adelaide SA", []string{"au"}},
		{"Darwin NT", []string{"au"}},
		{"Darwin, NT", []string{"au"}},
		{"Canberra ACT", []string{"au"}},
		// The other readings stay where no Australian city says otherwise.
		{"Seattle, WA", []string{"us"}},
		{"Spokane WA", []string{"us"}},
		{"Yellowknife, NT", []string{"ca"}},
	}
	for _, tt := range tests {
		t.Run(tt.location, func(t *testing.T) {
			got := Parse(tt.location).Countries
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q).Countries = %v, want %v", tt.location, got, tt.want)
			}
		})
	}
}

// TestParseAustralianStateKeepsTheCity guards what cutting the code off is for: the place
// before it still resolves as a city, with its metro, instead of the whole "perth wa" token
// matching nothing in the city dictionary.
func TestParseAustralianStateKeepsTheCity(t *testing.T) {
	want := Geo{Countries: []string{"au"}, Regions: []string{"apac"}, Cities: []string{"Perth"}, Metros: []string{"Perth (AU)"}}
	for _, location := range []string{"Perth WA", "Perth, WA"} {
		if got := Parse(location); !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q) = %+v, want %+v", location, got, want)
		}
	}
	// The suburb is a small place, named once Perth has placed the line in Australia.
	want.Cities = []string{"Osborne Park", "Perth"}
	if got := Parse("Osborne Park, Perth WA"); !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(%q) = %+v, want %+v", "Osborne Park, Perth WA", got, want)
	}
}
