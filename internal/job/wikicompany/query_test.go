package wikicompany

import (
	"strings"
	"testing"
)

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
