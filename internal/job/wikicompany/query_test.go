package wikicompany

import (
	"strings"
	"testing"
)

func TestIsValidQID(t *testing.T) {
	cases := []struct {
		qid  string
		want bool
	}{
		{"Q123456", true},
		{"Q1", true},
		{"", false},
		{"123456", false},                // missing the Q prefix
		{"Q123 } VALUES { wd:Q1", false}, // a malformed/injected value must never reach the query builder
		{"q123", false},                  // lowercase q is not a valid Wikidata entity ID
	}
	for _, c := range cases {
		if got := isValidQID(c.qid); got != c.want {
			t.Errorf("isValidQID(%q) = %v, want %v", c.qid, got, c.want)
		}
	}
}

func TestBuildOrganizationCheckQuery_UsesTransitivePropertyPath(t *testing.T) {
	query := buildOrganizationCheckQuery("Q123456")

	if !strings.Contains(query, "wd:Q123456") {
		t.Fatalf("query does not reference the candidate QID: %s", query)
	}
	if !strings.Contains(query, "wdt:P31/wdt:P279*") {
		t.Fatalf("query does not walk the subclass hierarchy via wdt:P31/wdt:P279*: %s", query)
	}
	for _, anchor := range organizationAnchorQIDs {
		if !strings.Contains(query, "wd:"+anchor) {
			t.Fatalf("query missing anchor QID %s: %s", anchor, query)
		}
	}
}

// TestBuildOrganizationCheckQuery_ExcludesGeographicAndAdministrativeEntities
// guards against a production false positive: "Nissan" resolved to Q270195, a
// commune in Hérault, France — wdt:P31/wdt:P279* from a commune's class reaches
// Q56061 (administrative territorial entity), which Wikidata also treats as
// under Q43229 (organization) somewhere in its multi-parent class hierarchy, so
// the positive organization walk alone accepted it. The query must also assert
// the candidate is NOT reachable from a small set of geographic/administrative
// anchors, so a same-named place is rejected regardless of that shared ancestry.
func TestBuildOrganizationCheckQuery_ExcludesGeographicAndAdministrativeEntities(t *testing.T) {
	query := buildOrganizationCheckQuery("Q270195")

	if !strings.Contains(query, "FILTER NOT EXISTS") {
		t.Fatalf("query does not exclude geographic/administrative entities: %s", query)
	}
	for _, excluded := range nonOrganizationAnchorQIDs {
		if !strings.Contains(query, "wd:"+excluded) {
			t.Fatalf("query missing exclusion QID %s: %s", excluded, query)
		}
	}
}
