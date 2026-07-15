package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/enrich"
)

func TestParseInsightsSort(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "open", false},
		{"open", "open", false},
		{"growth", "growth", false},
		{"bogus", "", true},
	}
	for _, c := range cases {
		got, err := parseInsightsSort(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("parseInsightsSort(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseInsightsSort(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseInsightsLimit(t *testing.T) {
	cases := []struct {
		in      string
		want    int32
		wantErr bool
	}{
		{"", insightsDefaultLimit, false},
		{"50", 50, false},
		{"200", insightsMaxLimit, false},
		{"0", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"201", 0, true},
	}
	for _, c := range cases {
		got, err := parseInsightsLimit(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("parseInsightsLimit(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseInsightsLimit(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseCountry(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"de", "de", false},
		{"DE", "de", false},
		{"USA", "", true},
		{"D1", "", true},
		{"d", "", true},
	}
	for _, c := range cases {
		got, err := parseCountry(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("parseCountry(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseCountry(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseCategoryAndSeniorityAgainstVocab(t *testing.T) {
	validCat := enrich.CategoryValues[0]
	validSen := enrich.SeniorityValues[0]

	if got, err := parseCategory(""); err != nil || got != "" {
		t.Errorf("parseCategory(\"\") = %q, %v, want \"\", nil", got, err)
	}
	if got, err := parseCategory(validCat); err != nil || got != validCat {
		t.Errorf("parseCategory(%q) = %q, %v, want %q, nil", validCat, got, err, validCat)
	}
	if _, err := parseCategory("definitely-not-a-category"); err == nil {
		t.Error("parseCategory(unknown) err = nil, want a rejection")
	}

	if got, err := parseSeniority(validSen); err != nil || got != validSen {
		t.Errorf("parseSeniority(%q) = %q, %v, want %q, nil", validSen, got, err, validSen)
	}
	if _, err := parseSeniority("overlord"); err == nil {
		t.Error("parseSeniority(unknown) err = nil, want a rejection")
	}
}

func TestResolveVelocityFacet(t *testing.T) {
	if k, v, err := resolveVelocityFacet("", "", ""); err != nil || k != "all" || v != "" {
		t.Errorf("no facet = %q/%q/%v, want all//nil", k, v, err)
	}
	if k, v, err := resolveVelocityFacet("engineering", "", ""); err != nil || k != "category" || v != "engineering" {
		t.Errorf("category facet = %q/%q/%v, want category/engineering/nil", k, v, err)
	}
	if k, v, err := resolveVelocityFacet("", "", "DE"); err != nil || k != "country" || v != "DE" {
		t.Errorf("country facet = %q/%q/%v, want country/DE/nil", k, v, err)
	}
	if _, _, err := resolveVelocityFacet("engineering", "senior", ""); err == nil {
		t.Error("two facets err = nil, want a rejection (single-dimensional rollup)")
	}
}
