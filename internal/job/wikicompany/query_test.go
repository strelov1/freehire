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
