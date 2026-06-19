package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// plan matches yc names + the bigtech hand list against existing companies and
// emits a write only for companies whose managed-tag set actually changes,
// preserving any unmanaged tags. `google` is used as the known bigtech member and
// `acme-startup` as a yc-only match, so the test does not depend on which exact
// companies the hand list contains beyond google being present.
func TestPlan(t *testing.T) {
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "google", Collections: []string{}},               // bigtech (hand list)
		{Slug: "acme-startup", Collections: []string{"custom"}}, // yc match, unmanaged tag preserved
		{Slug: "nytimes", Collections: []string{}},              // matches nothing → no write
		{Slug: "oldyc", Collections: []string{"yc"}},            // no longer matched → yc dropped
	}
	// "Acme Startup" normalizes to acme-startup (yc). "Unknown Co" matches nothing.
	ycNames := []string{"Acme Startup", "Unknown Co"}

	got := plan(rows, ycNames)

	writeBySlug := map[string][]string{}
	for _, w := range got.writes {
		writeBySlug[w.Slug] = w.Collections
	}

	if c := writeBySlug["google"]; !reflect.DeepEqual(c, []string{"bigtech"}) {
		t.Errorf("google write = %#v, want [bigtech]", c)
	}
	if c := writeBySlug["acme-startup"]; !reflect.DeepEqual(c, []string{"custom", "yc"}) {
		t.Errorf("acme-startup write = %#v, want [custom yc]", c)
	}
	if c, ok := writeBySlug["oldyc"]; !ok || len(c) != 0 {
		t.Errorf("oldyc write = %#v (ok=%v), want [] (yc dropped)", c, ok)
	}
	if _, ok := writeBySlug["nytimes"]; ok {
		t.Errorf("nytimes should not be rewritten (no managed match), got %v", writeBySlug["nytimes"])
	}

	if got.ycMatched != 1 || got.ycUnmatched != 1 {
		t.Errorf("yc matched/unmatched = %d/%d, want 1/1", got.ycMatched, got.ycUnmatched)
	}
	// Only google (of the rows) is in the bigtech hand list.
	if got.bigMatched != 1 {
		t.Errorf("bigtech matched = %d, want 1", got.bigMatched)
	}
}

// A company keeps an unmanaged tag through reconciliation even when it gains a
// managed one.
func TestPlan_PreservesUnmanagedTag(t *testing.T) {
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "google", Collections: []string{"custom"}},
	}
	got := plan(rows, nil) // no yc names; google gains bigtech from the hand list
	if len(got.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(got.writes))
	}
	c := got.writes[0].Collections
	sort.Strings(c)
	if !reflect.DeepEqual(c, []string{"bigtech", "custom"}) {
		t.Errorf("collections = %#v, want [bigtech custom]", c)
	}
}
