package location

import (
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/enrich"
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
		{
			name:     "bare remote yields work mode but no geography",
			location: "Remote",
			want:     Geo{WorkMode: "remote"},
		},
		{
			name:     "explicit anywhere yields global and remote",
			location: "Remote - Anywhere",
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
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, WorkMode: "hybrid"},
		},
		{
			name:     "onsite marker in parentheses keeps the city",
			location: "Berlin (On-site)",
			want:     Geo{Countries: []string{"de"}, Regions: []string{"eu"}, WorkMode: "onsite"},
		},
		{
			name:     "hybrid wins over a remote marker in the same string",
			location: "Hybrid / Remote - London",
			want:     Geo{Countries: []string{"gb"}, Regions: []string{"uk"}, WorkMode: "hybrid"},
		},
		{
			name:     "country buried among unknown tokens",
			location: "Burlington, Massachusetts, United States; Remote",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}, WorkMode: "remote"},
		},
		{
			name:     "Central Asia: Uzbek district, city, country (Uzbek spelling)",
			location: "Yunusobod, Toshkent, Uzbekistan",
			want:     Geo{Countries: []string{"uz"}, Regions: []string{"cis"}},
		},
		{
			name:     "Central Asia: remote Kazakhstan",
			location: "Remote - Kazakhstan",
			want:     Geo{Countries: []string{"kz"}, Regions: []string{"cis"}, WorkMode: "remote"},
		},
		{
			name:     "CIS: Baku via city and country",
			location: "Baku, Azerbaijan",
			want:     Geo{Countries: []string{"az"}, Regions: []string{"cis"}},
		},
		{
			name:     "country-only Georgia is the US state, not the country (no false ge)",
			location: "Atlanta, Georgia, United States",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			name:     "empty location",
			location: "",
			want:     Geo{},
		},
		{
			name:     "unresolvable token guesses nothing",
			location: "Atlantis",
			want:     Geo{},
		},
		{
			// A hyphenated word whose first segment happens to be a 2-letter code
			// ("in") must not emit a phantom country: no other segment is geography.
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
			// A hyphenated city whose inner segment is a bare code ("on") never misfires.
			name:     "hyphenated city does not misfire",
			location: "stoke-on-trent",
			want:     Geo{},
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
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			name:     "US City, ST",
			location: "Austin, TX",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			name:     "US state code CA is California, not Canada",
			location: "San Francisco, CA",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
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
			want:     Geo{Countries: []string{"ca"}, Regions: []string{"north_america"}},
		},
		{
			name:     "Canadian full province name",
			location: "Vancouver, British Columbia",
			want:     Geo{Countries: []string{"ca"}, Regions: []string{"north_america"}},
		},
		{
			name:     "Washington DC resolves to us",
			location: "Washington, DC",
			want:     Geo{Countries: []string{"us"}, Regions: []string{"north_america"}},
		},
		{
			name:     "country Georgia is never misread as the US state",
			location: "Tbilisi, Georgia",
			want:     Geo{Countries: []string{"ge"}, Regions: []string{"cis"}},
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
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}},
		},
		{
			name:     "city marker prefix is stripped",
			location: "г Москва",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}},
		},
		{
			name:     "hyphenated Cyrillic city",
			location: "Санкт-Петербург",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}},
		},
		{
			name:     "multi-word Cyrillic city",
			location: "Нижний Новгород",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}},
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
			name:     "bare Удалённо yields remote mode, no geography",
			location: "Удалённо",
			want:     Geo{WorkMode: "remote"},
		},
		{
			name:     "Cyrillic hybrid marker with city",
			location: "Москва, гибрид",
			want:     Geo{Countries: []string{"ru"}, Regions: []string{"cis"}, WorkMode: "hybrid"},
		},
		{
			name:     "CIS: Minsk maps to Belarus / cis",
			location: "Минск",
			want:     Geo{Countries: []string{"by"}, Regions: []string{"cis"}},
		},
		{
			name:     "Central Asia: Tashkent maps to Uzbekistan",
			location: "Ташкент",
			want:     Geo{Countries: []string{"uz"}, Regions: []string{"cis"}},
		},
		{
			name:     "Ukrainian spelling Київ maps to Ukraine / eu",
			location: "Київ",
			want:     Geo{Countries: []string{"ua"}, Regions: []string{"eu"}},
		},
		{
			name:     "city starting with г is not mistaken for the marker",
			location: "Грозный",
			want:     Geo{},
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
// every region the parser emits is a member of enrich.RegionValues and every
// work mode a member of enrich.WorkModeValues — the parser never invents a value
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
			if !slices.Contains(enrich.RegionValues, r) {
				t.Errorf("Parse(%q) emitted region %q outside RegionValues", s, r)
			}
		}
		if got.WorkMode != "" && !slices.Contains(enrich.WorkModeValues, got.WorkMode) {
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
		{"Shanghai, Shanghai, cn", Geo{Countries: []string{"cn"}, Regions: []string{"apac"}}},
		{"Riyadh, sa", Geo{Countries: []string{"sa"}, Regions: []string{"mena"}}},
		{"Lisboa, Lisboa, pt", Geo{Countries: []string{"pt"}, Regions: []string{"eu"}}},
		{"São Paulo, SP, br", Geo{Countries: []string{"br"}, Regions: []string{"latam"}}},
		// Beacon cities.
		{"San Francisco", Geo{Countries: []string{"us"}, Regions: []string{"north_america"}}},
		{"Athens, Attica, Greece", Geo{Countries: []string{"gr"}, Regions: []string{"eu"}}},
		{"Seoul, South Korea", Geo{Countries: []string{"kr"}, Regions: []string{"apac"}}},
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
