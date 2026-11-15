package location

import (
	"reflect"
	"testing"
)

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
