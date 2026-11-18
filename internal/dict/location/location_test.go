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
			// "Lake Worth" is the old name of Lake Worth Beach, Florida; the Texan town is
			// below the city table's floor, so the line names no city (alias_place.go).
			name:     "US City, ST ZIP",
			location: "Lake Worth, TX 76135",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
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
