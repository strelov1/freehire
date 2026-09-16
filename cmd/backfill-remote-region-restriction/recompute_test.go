package main

import (
	"slices"
	"testing"
)

func TestRecomputeGeography(t *testing.T) {
	tests := []struct {
		name          string
		title         string
		location      string
		description   string
		wantCountries []string
		wantRegions   []string
	}{
		{
			name:          "the quanata report: title-embedded restriction",
			title:         "Senior Back End Engineer [Remote-US]",
			location:      "remote",
			wantCountries: []string{"us"},
			wantRegions:   []string{"north_america"},
		},
		{
			name:          "the creative-fabrica report: multi-country title restriction",
			title:         "Senior Backend Engineer (Go) (Location - Australia or New Zealand)",
			location:      "Remote",
			wantCountries: []string{"au", "nz"},
			wantRegions:   []string{"apac"},
		},
		{
			name:          "the kard-financial report: description-only restriction",
			title:         "Senior Software Engineer II, Customer Experience",
			location:      "Remote",
			description:   "We are a fully remote company hiring in the US, Canada, Argentina, or Brazil only.",
			wantCountries: []string{"ar", "br", "ca", "us"},
			wantRegions:   []string{"latam", "north_america"},
		},
		{
			name:          "a genuinely open bare-remote posting stays unresolved",
			title:         "Senior Software Developer",
			location:      "Remote",
			description:   "We build great software as a fully distributed team.",
			wantCountries: nil,
			wantRegions:   nil,
		},
		{
			name:          "a resolved location is untouched",
			title:         "Engineer [Remote-US]",
			location:      "Remote - Germany",
			wantCountries: []string{"de"},
			wantRegions:   []string{"eu"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCountries, gotRegions := recomputeGeography(tt.title, tt.location, tt.description)
			if !slices.Equal(gotCountries, tt.wantCountries) {
				t.Errorf("countries = %v, want %v", gotCountries, tt.wantCountries)
			}
			if !slices.Equal(gotRegions, tt.wantRegions) {
				t.Errorf("regions = %v, want %v", gotRegions, tt.wantRegions)
			}
		})
	}
}

func TestIsBareGlobal(t *testing.T) {
	tests := []struct {
		name      string
		countries []string
		regions   []string
		want      bool
	}{
		{"the exact shape this pass corrects", nil, []string{"global"}, true},
		{"a resolved country is never bare global", []string{"us"}, []string{"global"}, false},
		{"global alongside a real region is left alone", nil, []string{"global", "eu"}, false},
		{"no geography at all is not bare global (nothing to correct)", nil, nil, false},
		{"a plain resolved region", nil, []string{"eu"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBareGlobal(tt.countries, tt.regions); got != tt.want {
				t.Errorf("isBareGlobal(%v, %v) = %v, want %v", tt.countries, tt.regions, got, tt.want)
			}
		})
	}
}
