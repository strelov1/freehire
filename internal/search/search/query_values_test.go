package search

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"
)

func TestIgnoredValues(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  map[string][]string
	}{
		{"invalid visa boolean", "visa_sponsorship=yes", map[string][]string{"visa_sponsorship": {"yes"}}},
		{"valid visa boolean", "visa_sponsorship=%20TRUE", nil},
		{"invalid seniority modifier", "seniority_include_unspecified=maybe", map[string][]string{"seniority_include_unspecified": {"maybe"}}},
		{"invalid experience modifier", "experience_include_unspecified=maybe", map[string][]string{"experience_include_unspecified": {"maybe"}}},
		{"invalid experience seniority modifier", "experience_only_seniority_unspecified=maybe", map[string][]string{"experience_only_seniority_unspecified": {"maybe"}}},
		{"valid boolean modifiers", "seniority_include_unspecified=True&experience_include_unspecified=%20TRUE&experience_only_seniority_unspecified=True", nil},
		{"clean request", "work_mode=remote&seniority=senior", nil},
		{"unknown value", "work_mode=foo", map[string][]string{"work_mode": {"foo"}}},
		{"valid and unknown mixed, comma-joined", "work_mode=remote,foo", map[string][]string{"work_mode": {"foo"}}},
		{"case and whitespace the filter could still match", "work_mode=Remote&seniority=%20senior", nil},
		{"exclude form reported under its own name", "seniority_exclude=guru", map[string][]string{"seniority_exclude": {"guru"}}},
		{"regions unspecified sentinel is valid", "regions=none", nil},
		{"retired category is translated, not ignored", "category=management", nil},
		{"boolean facet", "requires_clearance=flase&is_tech=tech", map[string][]string{"requires_clearance": {"flase"}}},
		{"is_tech is tech or non_tech, not a boolean", "is_tech=non_tech&is_tech_exclude=true", map[string][]string{"is_tech_exclude": {"true"}}},
		{"reality class", "reality=likely-evergreen&reality=ghost", map[string][]string{"reality": {"ghost"}}},
		{"open-ended facets are not checked", "countries=zz&cities=atlantis&source=nowhere", nil},
		{"unknown skill", "skills=golangg", map[string][]string{"skills": {"golangg"}}},
		{"skill alias the filter resolves is known", "skills=golang,Go&skills_exclude=reactjs", nil},
		{"unknown excluded skill", "skills_exclude=phpp", map[string][]string{"skills_exclude": {"phpp"}}},
		{"repeated value reported once", "work_mode=foo&work_mode=foo", map[string][]string{"work_mode": {"foo"}}},
		{"stray comma is not a value", "work_mode=remote,", nil},
		{"unknown perk", "perks=401k_match,free_beer", map[string][]string{"perks": {"free_beer"}}},
		{"unknown engagement", "engagement=ir35_outside,ir35_maybe", map[string][]string{"engagement": {"ir35_maybe"}}},
		{"remote reach region checked like regions", "work_mode_remote_regions=eu,mars&work_mode_remote_countries=zz", map[string][]string{"work_mode_remote_regions": {"mars"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if got := IgnoredValues(v); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("IgnoredValues(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestIgnoredValuesIsBounded(t *testing.T) {
	v := url.Values{}
	for i := range 3 * maxUnknownParamsReported {
		v.Add("work_mode", fmt.Sprintf("bad%02d", i))
	}
	for facet := range closedFacets {
		v.Add(facet, "zzz-not-a-value")
		v.Add(facet+"_exclude", "zzz-not-a-value")
	}
	got := IgnoredValues(v)
	if len(got) != maxUnknownParamsReported {
		t.Errorf("reported %d params, want %d", len(got), maxUnknownParamsReported)
	}
	for param, values := range got {
		if len(values) > maxUnknownParamsReported {
			t.Errorf("%s: reported %d values, want at most %d", param, len(values), maxUnknownParamsReported)
		}
	}
}

// Every closed facet must be a facet the filter reads and carry a non-empty vocabulary:
// an empty one would report every value, and a facet the filter does not read would name
// a value the request never filtered on.
func TestClosedFacetsAreRealFacetsWithAVocabulary(t *testing.T) {
	for facet, known := range closedFacets {
		if _, ok := StringFacets[facet]; !ok {
			t.Errorf("%s: not in StringFacets", facet)
		}
		if known("zzz-not-a-value") {
			t.Errorf("%s: accepts anything", facet)
		}
	}
}

func TestUnknownFacetValue(t *testing.T) {
	cases := []struct {
		param, value string
		want         bool
	}{
		{"category", "site-reliability", true},
		{"category", "backend", false},
		{"skills", "golangg", true},
		{"skills", "golang", false},
		{"skills_exclude", "golangg", true},
		{"cities", "atlantis", false}, // open facet: nothing to be unknown against
		{"q", "anything", false},
	}
	for _, tc := range cases {
		if got := UnknownFacetValue(tc.param, tc.value); got != tc.want {
			t.Errorf("UnknownFacetValue(%q, %q) = %v, want %v", tc.param, tc.value, got, tc.want)
		}
	}
}

func TestIgnoredValuesCapsValuesInSortedOrder(t *testing.T) {
	v := url.Values{}
	for i := 3 * maxUnknownParamsReported; i >= 0; i-- {
		v.Add("work_mode", fmt.Sprintf("bad%02d", i))
	}
	want := make([]string, maxUnknownParamsReported)
	for i := range want {
		want[i] = fmt.Sprintf("bad%02d", i)
	}
	if got := IgnoredValues(v)["work_mode"]; !reflect.DeepEqual(got, want) {
		t.Errorf("reported %v, want sorted prefix %v", got, want)
	}
}

func TestIgnoredValuesChecksFacetModesAsWholeScalars(t *testing.T) {
	for facet := range StringFacets {
		param := facet + "_mode"
		for _, value := range []string{"invalid", "and,or", ""} {
			v := url.Values{param: {value, "and"}}
			want := map[string][]string{param: {value}}
			if got := IgnoredValues(v); !reflect.DeepEqual(got, want) {
				t.Errorf("%s=%q: got %v, want %v", param, value, got, want)
			}
		}
		for _, value := range []string{"and", "OR", " And "} {
			if got := IgnoredValues(url.Values{param: {value, "invalid"}}); got != nil {
				t.Errorf("%s=%q: got %v, want no ignored values", param, value, got)
			}
		}
	}
}
