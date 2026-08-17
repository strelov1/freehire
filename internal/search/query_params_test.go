package search

import (
	"fmt"
	"net/url"
	"testing"
)

func TestUnknownParams_SuggestsThePluralFacetName(t *testing.T) {
	// The observed failure: a caller reaches for the singular `country`, the
	// filter reads `countries`, and the query silently widens to the whole
	// catalogue.
	got := UnknownParams(url.Values{"country": {"it"}}, nil)

	if len(got) != 1 {
		t.Fatalf("UnknownParams = %#v, want exactly one entry", got)
	}
	if got[0].Param != "country" {
		t.Errorf("Param = %q, want country", got[0].Param)
	}
	if got[0].DidYouMean != "countries" {
		t.Errorf("DidYouMean = %q, want countries", got[0].DidYouMean)
	}
}

func TestUnknownParams_AcceptsTheFilterVocabulary(t *testing.T) {
	// Every facet, both facet conventions, and the scalar filters are read by
	// FilterFromValues, so none of them may be reported as ignored.
	v := url.Values{
		"countries":            {"it"},
		"skills_exclude":       {"python"},
		"seniority_mode":       {"and"},
		"visa_sponsorship":     {"true"},
		"salary_min":           {"1000"},
		"salary_max":           {"9000"},
		"experience_years_min": {"3"},
		"posted_within_days":   {"7"},
	}

	if got := UnknownParams(v, nil); len(got) != 0 {
		t.Errorf("UnknownParams = %#v, want none", got)
	}
}

func TestUnknownParams_AcceptsTheCallersOwnParams(t *testing.T) {
	// Transport params (pagination, sort, response format) belong to the
	// handler, not to the filter vocabulary, so the caller declares them.
	v := url.Values{"q": {"go"}, "limit": {"10"}}

	if got := UnknownParams(v, []string{"q", "limit"}); len(got) != 0 {
		t.Errorf("UnknownParams = %#v, want none", got)
	}
}

func TestUnknownParams_ReportsAStrangerWithoutGuessing(t *testing.T) {
	// A param that resembles nothing in the vocabulary is still reported — the
	// caller learns it was ignored — but gets no invented suggestion.
	got := UnknownParams(url.Values{"utm_source": {"newsletter"}}, nil)

	if len(got) != 1 {
		t.Fatalf("UnknownParams = %#v, want exactly one entry", got)
	}
	if got[0].DidYouMean != "" {
		t.Errorf("DidYouMean = %q, want empty", got[0].DidYouMean)
	}
}

func TestScalarFilters_EachOneStillNarrowsAQuery(t *testing.T) {
	// scalarFilters is a hand-kept list, and UnknownParams trusts it: a name left
	// on it after its filter was removed would quietly vouch for a param nothing
	// reads. Each entry must still produce a filter fragment on its own.
	samples := map[string]string{
		"visa_sponsorship":     "true",
		"salary_min":           "1000",
		"salary_max":           "9000",
		"experience_years_min": "3",
		"posted_within_days":   "7",
	}

	for _, param := range scalarFilters {
		sample, ok := samples[param]
		if !ok {
			t.Errorf("scalarFilters lists %q with no sample value here — add one", param)
			continue
		}
		if filter := FilterFromValues(url.Values{param: {sample}}); filter == nil {
			t.Errorf("%s=%s produced no filter, but scalarFilters claims it is read", param, sample)
		}
	}
}

func TestUnknownParams_OrdersReportDeterministically(t *testing.T) {
	// Map iteration must not leak into the response: two ignored params always
	// come back in the same order.
	v := url.Values{"zebra": {"1"}, "alpha": {"2"}}

	got := UnknownParams(v, nil)
	if len(got) != 2 {
		t.Fatalf("UnknownParams = %#v, want two entries", got)
	}
	if got[0].Param != "alpha" || got[1].Param != "zebra" {
		t.Errorf("order = %q, %q; want alpha, zebra", got[0].Param, got[1].Param)
	}
}

func TestUnknownParams_SkipsTheEmptyName(t *testing.T) {
	// "?=value" parses to a param whose name is the empty string. Reporting it
	// tells the caller nothing — there is no name to correct.
	v, err := url.ParseQuery("=value&countries=it")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}

	if got := UnknownParams(v, nil); len(got) != 0 {
		t.Errorf("UnknownParams = %#v, want none", got)
	}
}

func TestUnknownParams_CapsTheReport(t *testing.T) {
	// These endpoints are public and unauthenticated, and every reported param
	// is echoed back into the response body. Without a cap a hostile query turns
	// each junk param into response bytes; with one, a real mistake (a param or
	// three) is still reported in full.
	flood := url.Values{}
	for i := range 200 {
		flood[fmt.Sprintf("junk%03d", i)] = []string{"x"}
	}

	got := UnknownParams(flood, nil)
	if len(got) != maxUnknownParamsReported {
		t.Fatalf("len = %d, want it capped at %d", len(got), maxUnknownParamsReported)
	}
	// The cap keeps the sorted order rather than an arbitrary slice of the map.
	if got[0].Param != "junk000" {
		t.Errorf("first = %q, want junk000 — the cap must apply after sorting", got[0].Param)
	}
}
