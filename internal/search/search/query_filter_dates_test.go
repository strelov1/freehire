package search

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestFilterFromValues_PostedWithinDays(t *testing.T) {
	// now is injected so the cutoff is deterministic. posted_within_days=N restricts
	// to posted_ts >= now - N*86400 (posted within the last N days).
	now := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	cutoff := now.Unix() - 7*86400

	got := normalizeGroups(t, filterFromValues(vals("posted_within_days=7"), now))
	want := [][]string{{fmt.Sprintf("posted_ts >= %d", cutoff)}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posted_within_days=7: got %v, want %v", got, want)
	}

	// It ANDs with other facets as its own group.
	got = normalizeGroups(t, filterFromValues(vals("seniority=senior&posted_within_days=7"), now))
	want = [][]string{
		{`enrichment.seniority = "senior"`},
		{fmt.Sprintf("posted_ts >= %d", cutoff)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("composed: got %v, want %v", got, want)
	}
}

func TestFilterFromValues_PostedWithinDaysInvalidIgnored(t *testing.T) {
	// Absent, empty, zero, negative, and non-numeric values impose no date restriction.
	now := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	for _, q := range []string{"", "posted_within_days=", "posted_within_days=0", "posted_within_days=-3", "posted_within_days=soon"} {
		if got := filterFromValues(vals(q), now); got != nil {
			t.Errorf("filterFromValues(%q) = %v, want nil (no date filter)", q, got)
		}
	}
}

func TestFilterFromValues_OpenWithinDays(t *testing.T) {
	// open_within_days=N restricts to created_ts >= now - N*86400 — how long the
	// posting has been in the catalogue, which is a different question from
	// posted_within_days and answered by a different field.
	now := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	cutoff := now.Unix() - 7*86400

	got := normalizeGroups(t, filterFromValues(vals("open_within_days=7"), now))
	want := [][]string{{fmt.Sprintf("created_ts >= %d", cutoff)}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("open_within_days=7: got %v, want %v", got, want)
	}

	// It ANDs with other facets as its own group. normalizeGroups sorts, so the
	// expectation is in sorted order, not insertion order.
	got = normalizeGroups(t, filterFromValues(vals("seniority=senior&open_within_days=7"), now))
	want = [][]string{
		{fmt.Sprintf("created_ts >= %d", cutoff)},
		{`enrichment.seniority = "senior"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("composed: got %v, want %v", got, want)
	}
}

func TestFilterFromValues_BothDateBoundsCompose(t *testing.T) {
	// The two bounds are independent and AND together. This is the case the pair
	// exists for: a wide "open within 30 days" alongside a narrow "posted within 3",
	// which a single bound cannot express.
	now := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	openCutoff := now.Unix() - 30*86400
	postedCutoff := now.Unix() - 3*86400

	got := normalizeGroups(t, filterFromValues(vals("open_within_days=30&posted_within_days=3"), now))
	want := [][]string{
		{fmt.Sprintf("created_ts >= %d", openCutoff)},
		{fmt.Sprintf("posted_ts >= %d", postedCutoff)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("both bounds: got %v, want %v", got, want)
	}
}

func TestFilterFromValues_OpenWithinDaysInvalidIgnored(t *testing.T) {
	// Same rule as posted_within_days: absent, empty, zero, negative, and non-numeric
	// values impose no restriction rather than matching nothing.
	now := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	for _, q := range []string{"open_within_days=", "open_within_days=0", "open_within_days=-3", "open_within_days=soon"} {
		if got := filterFromValues(vals(q), now); got != nil {
			t.Errorf("filterFromValues(%q) = %v, want nil (no date filter)", q, got)
		}
	}
}

func TestFilterFromValues_AbsurdDayCountsImposeNoBound(t *testing.T) {
	// A day count large enough to overflow time.Duration used to WRAP: the cutoff
	// landed in the future and the query matched nothing, so the most permissive input
	// anyone could type produced the most restrictive result. Both bounds share the
	// arithmetic, so both are checked.
	now := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	for _, param := range []string{"open_within_days", "posted_within_days"} {
		for _, n := range []int{maxWithinDays + 1, 106752, 200000, math.MaxInt32} {
			q := fmt.Sprintf("%s=%d", param, n)
			if got := filterFromValues(vals(q), now); got != nil {
				t.Errorf("filterFromValues(%q) = %v, want nil (bound beyond the catalogue is no bound)", q, got)
			}
		}
		// The largest honoured value still produces a cutoff in the PAST — the guard
		// must sit below the wrap, not at it.
		q := fmt.Sprintf("%s=%d", param, maxWithinDays)
		if got := filterFromValues(vals(q), now); got == nil {
			t.Errorf("filterFromValues(%q) = nil, want the bound at the documented maximum", q)
		}
	}
}

// The cut-offs round now down to the hour, so requests within one hour build the same
// filter (facetd caches answers by filter).
func TestFilterFromValues_DayCutoffsRoundToTheHour(t *testing.T) {
	early := time.Date(2026, 6, 19, 14, 0, 1, 0, time.UTC)
	late := time.Date(2026, 6, 19, 14, 59, 59, 0, time.UTC)
	for _, q := range []string{"posted_within_days=1", "open_within_days=7"} {
		a := normalizeGroups(t, filterFromValues(vals(q), early))
		b := normalizeGroups(t, filterFromValues(vals(q), late))
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s: %v at 14:00:01 and %v at 14:59:59, want one filter", q, a, b)
		}
	}
	got := normalizeGroups(t, filterFromValues(vals("posted_within_days=1"), late))
	want := [][]string{{fmt.Sprintf("posted_ts >= %d", time.Date(2026, 6, 18, 14, 0, 0, 0, time.UTC).Unix())}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posted_within_days=1 at 14:59:59: got %v, want %v", got, want)
	}
}
