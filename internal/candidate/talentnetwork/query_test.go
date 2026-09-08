package talentnetwork

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func values(raw string) url.Values {
	v, err := url.ParseQuery(raw)
	if err != nil {
		panic(err)
	}
	return v
}

func TestQueryFromValues_ReadsEveryFilter(t *testing.T) {
	q, unread := QueryFromValues(values(
		"categories=backend,devops&seniorities=senior&skills=go,postgresql&tz=Europe&cities=berlin&specializations=platform&min_years=5&limit=10&offset=20"))

	if len(unread) != 0 {
		t.Errorf("unread = %v, want none", unread)
	}
	want := Query{
		Categories:      []string{"backend", "devops"},
		Seniorities:     []string{"senior"},
		Skills:          []string{"go", "postgresql"},
		TimezoneRegions: []string{"Europe"},
		Cities:          []string{"berlin"},
		Specializations: []string{"platform"},
		MinYears:        5,
		Limit:           10,
		Offset:          20,
	}
	if !reflect.DeepEqual(q, want) {
		t.Errorf("query = %+v\nwant %+v", q, want)
	}
}

func TestQueryFromValues_AbsentAndEmptyAgree(t *testing.T) {
	absent, _ := QueryFromValues(values(""))
	empty, _ := QueryFromValues(values("categories=&skills=,,&cities=%20"))
	absent.Limit, empty.Limit = 0, 0
	if !reflect.DeepEqual(absent, empty) {
		t.Errorf("empty filters gave %+v, absent gave %+v", empty, absent)
	}
}

func TestQueryFromValues_AppliesADefaultLimit(t *testing.T) {
	q, _ := QueryFromValues(values(""))
	if q.Limit != defaultLimit {
		t.Errorf("limit = %d, want the default %d", q.Limit, defaultLimit)
	}
}

// An unusable value WIDENS the answer, so it is reported rather than swallowed. That is
// the same rule the rest of this API follows for a param it does not understand: staying
// silent is what let a mistyped filter pass for an unfiltered search.
func TestQueryFromValues_ReportsUnreadableValues(t *testing.T) {
	cases := map[string]string{
		"min_years=lots": "min_years",
		"limit=abc":      "limit",
		"limit=0":        "limit",
		"limit=1000":     "limit",
		"offset=-1":      "offset",
		"min_years=-3":   "min_years",
		// Above the ceiling. Reported rather than served: an empty catalogue would tell
		// the caller nothing, and their filter really was not read.
		"min_years=100": "min_years",
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			_, unread := QueryFromValues(values(raw))
			if len(unread) != 1 || unread[0] != want {
				t.Errorf("unread = %v, want [%s]", unread, want)
			}
		})
	}
}

func TestQueryFromValues_UnreadablePagingFallsBackRatherThanNarrowing(t *testing.T) {
	q, _ := QueryFromValues(values("limit=99999&offset=-5"))
	if q.Limit != defaultLimit {
		t.Errorf("limit = %d, want the default %d", q.Limit, defaultLimit)
	}
	if q.Offset != 0 {
		t.Errorf("offset = %d, want 0", q.Offset)
	}
}

// A filter list is truncated rather than refused: a URL is typed by hand and pasted
// between people, and the alternative is an error page. The FIRST terms are kept —
// a list reads left to right, so those are the ones somebody actually typed.
func TestQueryFromValues_CapsFilterTerms(t *testing.T) {
	many := make([]string, maxFilterTerms+10)
	for i := range many {
		many[i] = "skill" + string(rune('a'+i%26))
	}
	q, _ := QueryFromValues(values("skills=" + strings.Join(many, ",")))
	if len(q.Skills) != maxFilterTerms {
		t.Errorf("skills = %d terms, want capped at %d", len(q.Skills), maxFilterTerms)
	}
}

func TestQueryFromValues_TrimsAndDropsBlankTerms(t *testing.T) {
	q, _ := QueryFromValues(values("skills=%20go%20,,%20postgresql%20,"))
	if !reflect.DeepEqual(q.Skills, []string{"go", "postgresql"}) {
		t.Errorf("skills = %v, want [go postgresql]", q.Skills)
	}
}

// The vocabulary is what the ignored-params report is measured against, so it has to
// name every param QueryFromValues actually reads — a missing entry would report a
// working filter as ignored, which is worse than not reporting at all.
func TestKnownParams_CoversEveryFilterTheQueryReads(t *testing.T) {
	raw := "categories=a&seniorities=b&skills=c&tz=d&cities=e&specializations=f&min_years=1&limit=2&offset=3"
	known := KnownParams()
	for param := range values(raw) {
		if !contains(known, param) {
			t.Errorf("KnownParams is missing %q, which QueryFromValues reads", param)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
