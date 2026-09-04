package search

import (
	"fmt"
	"math"
	"net/url"
	"reflect"
	"sort"
	"testing"
	"time"
)

// normalizeGroups makes a Filter result order-insensitive for comparison: the
// fragments within a group and the groups themselves are sorted. FilterFromValues
// iterates a map, so group order is not deterministic, but the AND/OR semantics
// do not depend on order.
func normalizeGroups(t *testing.T, got any) [][]string {
	t.Helper()
	if got == nil {
		return nil
	}
	groups, ok := got.([][]string)
	if !ok {
		t.Fatalf("filter type = %T, want [][]string", got)
	}
	out := make([][]string, len(groups))
	for i, g := range groups {
		cp := append([]string(nil), g...)
		sort.Strings(cp)
		out[i] = cp
	}
	sort.Slice(out, func(i, j int) bool {
		return joinKey(out[i]) < joinKey(out[j])
	})
	return out
}

func joinKey(s []string) string {
	b := ""
	for _, x := range s {
		b += x + "|"
	}
	return b
}

func vals(q string) url.Values {
	v, _ := url.ParseQuery(q)
	return v
}

func TestFilterFromValues_Empty(t *testing.T) {
	if got := FilterFromValues(url.Values{}); got != nil {
		t.Errorf("FilterFromValues(empty) = %v, want nil", got)
	}
	// Params we do not filter on (free-text query, sort) produce no filter.
	if got := FilterFromValues(vals("q=go&sort=posted_at")); got != nil {
		t.Errorf("FilterFromValues(non-facet) = %v, want nil", got)
	}
}

func TestFilterFromValues_SingleFacet(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("seniority=senior")))
	want := [][]string{{`enrichment.seniority = "senior"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_RepeatedFacetIsORed(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("skills=go&skills=rust")))
	want := [][]string{{`skills = "go"`, `skills = "rust"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_Collections(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("collections=yc")))
	want := [][]string{{`collections = "yc"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_IgnoresTheRetiredRoleFacet(t *testing.T) {
	// `role` was a cross-product of `category` and `seniority` and is retired. A stale
	// link still carrying it must build NO filter — the param is reported through
	// meta.ignored_params instead, so the search widens visibly rather than silently.
	if got := normalizeGroups(t, FilterFromValues(vals("role=senior_backend&role_exclude=qa"))); len(got) != 0 {
		t.Errorf("role built %v, want no filter at all", got)
	}

	// It must not take the region down with it: a retired facet beside a live one
	// leaves the live one filtering.
	got := normalizeGroups(t, FilterFromValues(vals("role=founding_engineer&regions=eu")))
	want := [][]string{{`regions = "eu"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_CommaSeparatedIsORed(t *testing.T) {
	// A single comma-joined value resolves the same way as repeated keys.
	got := normalizeGroups(t, FilterFromValues(vals("skills=go,rust")))
	want := [][]string{{`skills = "go"`, `skills = "rust"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("comma-separated: got %v, want %v", got, want)
	}
}

func TestFilterFromValues_CommaSeparatedExclude(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("skills_exclude=java,cpp")))
	want := [][]string{{`skills != "cpp"`}, {`skills != "java"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("comma-separated exclude: got %v, want %v", got, want)
	}
}

func TestFilterFromValues_CommaAndRepeatedKeyMix(t *testing.T) {
	// A comma-joined entry and a repeated key for the same param union together.
	got := normalizeGroups(t, FilterFromValues(vals("skills=go,rust&skills=aws")))
	want := [][]string{{`skills = "aws"`, `skills = "go"`, `skills = "rust"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mixed comma+repeated: got %v, want %v", got, want)
	}
}

func TestFilterFromValues_StrayCommasIgnored(t *testing.T) {
	// A leading/trailing/doubled comma must not produce an empty facet value.
	got := normalizeGroups(t, FilterFromValues(vals("skills=go,,rust,")))
	want := [][]string{{`skills = "go"`, `skills = "rust"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stray commas: got %v, want %v", got, want)
	}
}

func TestFilterFromValues_AndMode(t *testing.T) {
	// skills_mode=and → each value its own AND group (a job must have both).
	got := normalizeGroups(t, FilterFromValues(vals("skills=go&skills=rust&skills_mode=and")))
	want := [][]string{{`skills = "go"`}, {`skills = "rust"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_Exclude(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("regions_exclude=cis")))
	want := [][]string{{`regions != "cis"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_VisaBoolAndNumeric(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("visa_sponsorship=true&salary_min=100000&salary_max=200000&experience_years_min=3")))
	want := [][]string{
		{`enrichment.experience_years_min >= 3`},
		{`enrichment.salary_max <= 200000`},
		{`enrichment.salary_min >= 100000`},
		{`enrichment.visa_sponsorship = true`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_ExperienceYearsMax(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("experience_years_max=3")))
	want := [][]string{{`enrichment.experience_years_min <= 3`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_ExperienceYearsClosedRange(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("experience_years_min=2&experience_years_max=5")))
	want := [][]string{
		{`enrichment.experience_years_min <= 5`},
		{`enrichment.experience_years_min >= 2`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Zero is the entry-level selector, not an absent value: `experience_years_max=0`
// asks for the postings that state no prior experience is required. A guard that
// treats the bound as falsy would silently drop the one filter juniors need.
func TestFilterFromValues_ExperienceYearsMaxZero(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("experience_years_max=0")))
	want := [][]string{{`enrichment.experience_years_min <= 0`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A negative ceiling can only match nothing — `enrichment.experience_years_min` is
// never below zero — so it is a typo, not a query. The contract declares the param
// non-negative; honouring the sign would turn that typo into an empty result page
// that looks like a legitimately narrow search.
// role_type is derived at index time and carried top-level, like `is_tech` and
// `ai_archetype`, so it filters on the bare attribute rather than an enrichment.*
// dot path. Excluding it is how a caller asks for postings with no management
// marker — which is not the same as asking for individual-contributor postings.
func TestFilterFromValues_RoleType(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("role_type=people_manager")))
	want := [][]string{{`role_type = "people_manager"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_RoleTypeExclude(t *testing.T) {
	got := normalizeGroups(t, FilterFromValues(vals("role_type_exclude=people_manager")))
	want := [][]string{{`role_type != "people_manager"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterFromValues_ExperienceYearsMaxNegative(t *testing.T) {
	for _, raw := range []string{"-1", "-10"} {
		got := normalizeGroups(t, FilterFromValues(vals("experience_years_max="+raw)))
		if len(got) != 0 {
			t.Errorf("experience_years_max=%q: got %v, want no filter group", raw, got)
		}
	}
}

func TestFilterFromValues_ExperienceYearsMaxUnparseable(t *testing.T) {
	for _, raw := range []string{"", "abc", "3.5"} {
		got := normalizeGroups(t, FilterFromValues(vals("experience_years_max="+raw)))
		if len(got) != 0 {
			t.Errorf("experience_years_max=%q: got %v, want no filter group", raw, got)
		}
	}
}

func TestFilterFromValues_RegionUnspecifiedSentinel(t *testing.T) {
	// The reserved `regions=none` value selects jobs with no resolved geography
	// via Meili's IS EMPTY, not an equality against a literal "none" region.
	got := normalizeGroups(t, FilterFromValues(vals("regions=none")))
	want := [][]string{{`regions IS EMPTY`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("regions=none: got %v, want %v", got, want)
	}

	// It ORs with real region values inside the same facet group, so "Europe or
	// unspecified" is a single OR of an equality and IS EMPTY.
	got = normalizeGroups(t, FilterFromValues(vals("regions=none&regions=eu")))
	want = [][]string{{`regions = "eu"`, `regions IS EMPTY`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("regions=none&eu: got %v, want %v", got, want)
	}

	// Excluding the sentinel keeps only jobs that DO have a region (IS NOT EMPTY).
	got = normalizeGroups(t, FilterFromValues(vals("regions_exclude=none")))
	want = [][]string{{`regions IS NOT EMPTY`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("regions_exclude=none: got %v, want %v", got, want)
	}

	// The sentinel is scoped to the regions facet — "none" is a real value
	// everywhere else and stays an equality (never IS EMPTY).
	got = normalizeGroups(t, FilterFromValues(vals("relocation=none")))
	want = [][]string{{`enrichment.relocation = "none"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("relocation=none: got %v, want %v", got, want)
	}
}

func TestFilterFromValues_LocationFacetsORTogether(t *testing.T) {
	// regions, countries and cities describe one user concept ("where"), so their
	// included values OR into a single group instead of ANDing across facets:
	// selecting the "Global" region and "Brazil" must widen the results
	// (Global OR Brazil), not intersect them to zero.
	got := normalizeGroups(t, FilterFromValues(vals("regions=global&countries=BR")))
	want := [][]string{{`countries = "BR"`, `regions = "global"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("regions+countries: got %v, want %v", got, want)
	}

	// Cities join the same OR group.
	got = normalizeGroups(t, FilterFromValues(vals("regions=eu&countries=BR&cities=Berlin")))
	want = [][]string{{`cities = "Berlin"`, `countries = "BR"`, `regions = "eu"`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("regions+countries+cities: got %v, want %v", got, want)
	}

	// A non-location facet still ANDs with the location group as its own group:
	// "remote AND (Europe OR Brazil)".
	got = normalizeGroups(t, FilterFromValues(vals("regions=eu&countries=BR&work_mode=remote")))
	want = [][]string{
		{`countries = "BR"`, `regions = "eu"`},
		{`work_mode = "remote"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("location OR, work_mode AND: got %v, want %v", got, want)
	}

	// The regions-unspecified sentinel also joins the location OR group.
	got = normalizeGroups(t, FilterFromValues(vals("regions=none&countries=BR")))
	want = [][]string{{`countries = "BR"`, `regions IS EMPTY`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sentinel + country: got %v, want %v", got, want)
	}

	// Location excludes stay their own AND groups, independent of the include OR:
	// "(Europe OR Brazil) AND never Russia".
	got = normalizeGroups(t, FilterFromValues(vals("regions=eu&countries=BR&countries_exclude=RU")))
	want = [][]string{
		{`countries != "RU"`},
		{`countries = "BR"`, `regions = "eu"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("location OR with exclude: got %v, want %v", got, want)
	}
}

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

func TestFilterFromValues_NonNumericSalaryIgnored(t *testing.T) {
	// A non-numeric value must not emit a bogus `>= 0` fragment.
	if got := FilterFromValues(vals("salary_min=abc")); got != nil {
		t.Errorf("FilterFromValues(bad salary) = %v, want nil", got)
	}
}

func TestFilterFromValues_EmptyValueIgnored(t *testing.T) {
	if got := FilterFromValues(vals("seniority=")); got != nil {
		t.Errorf("FilterFromValues(empty facet value) = %v, want nil", got)
	}
}
