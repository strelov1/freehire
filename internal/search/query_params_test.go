package search

import (
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
