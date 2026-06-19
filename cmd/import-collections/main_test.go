package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/strelov1/freehire/internal/collections"
	"github.com/strelov1/freehire/internal/db"
)

// plan matches each collection's candidates against existing companies and emits a
// write only for companies whose managed-tag set actually changes, preserving any
// unmanaged tags. `google` is the known bigtech member and `acme-startup` a yc-only
// match, so the test does not depend on the exact hand list beyond google being in it.
func TestPlan(t *testing.T) {
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "google", Collections: []string{}},               // bigtech (hand list)
		{Slug: "acme-startup", Collections: []string{"custom"}}, // yc match, unmanaged tag preserved
		{Slug: "nytimes", Collections: []string{}},              // matches nothing → no write
		{Slug: "oldyc", Collections: []string{"yc"}},            // no longer matched → yc dropped
	}
	resolved := map[string][]string{
		"yc":      {"Acme Startup", "Unknown Co"}, // "Acme Startup" → acme-startup; "Unknown Co" → none
		"bigtech": collections.BigTechSlugs,
		"unicorn": nil,
	}

	got := plan(rows, resolved)

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

	if s := got.stats["yc"]; s.matched != 1 || s.unmatched != 1 {
		t.Errorf("yc stats = %+v, want {matched:1 unmatched:1}", s)
	}
	if s := got.stats["bigtech"]; s.matched != 1 { // only google (of the rows) is in the hand list
		t.Errorf("bigtech matched = %d, want 1", s.matched)
	}
}

// A company keeps an unmanaged tag through reconciliation even when it gains a
// managed one.
func TestPlan_PreservesUnmanagedTag(t *testing.T) {
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "google", Collections: []string{"custom"}},
	}
	// google gains bigtech from the hand list; no yc/unicorn candidates.
	got := plan(rows, map[string][]string{"bigtech": collections.BigTechSlugs})
	if len(got.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(got.writes))
	}
	c := got.writes[0].Collections
	sort.Strings(c)
	if !reflect.DeepEqual(c, []string{"bigtech", "custom"}) {
		t.Errorf("collections = %#v, want [bigtech custom]", c)
	}
}
