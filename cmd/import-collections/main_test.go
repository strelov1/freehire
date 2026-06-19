package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// plan matches yc names + the bigtech hand list against existing companies and
// emits a write only for companies whose managed-tag set actually changes,
// preserving any unmanaged tags.
func TestPlan(t *testing.T) {
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "stripe", Collections: []string{}},          // becomes yc + bigtech
		{Slug: "acme", Collections: []string{"custom"}},    // unmanaged tag preserved, no managed match
		{Slug: "google", Collections: []string{"bigtech"}}, // already correct → no write
		{Slug: "oldyc", Collections: []string{"yc"}},       // no longer matched → yc dropped
	}
	// "Stripe" matches stripe (yc). bigtech hand list contributes google + stripe.
	// "Unknown" matches nothing.
	ycNames := []string{"Stripe", "Unknown Co"}

	got := plan(rows, ycNames)

	writeBySlug := map[string][]string{}
	for _, w := range got.writes {
		writeBySlug[w.Slug] = w.Collections
	}

	if c := writeBySlug["stripe"]; !reflect.DeepEqual(c, []string{"bigtech", "yc"}) {
		t.Errorf("stripe write = %#v, want [bigtech yc]", c)
	}
	if c, ok := writeBySlug["oldyc"]; !ok || len(c) != 0 {
		t.Errorf("oldyc write = %#v (ok=%v), want [] (yc dropped)", c, ok)
	}
	if _, ok := writeBySlug["google"]; ok {
		t.Errorf("google should not be rewritten (already correct), got %v", writeBySlug["google"])
	}
	if _, ok := writeBySlug["acme"]; ok {
		t.Errorf("acme should not be rewritten (no managed change), got %v", writeBySlug["acme"])
	}

	if got.ycMatched != 1 || got.ycUnmatched != 1 {
		t.Errorf("yc matched/unmatched = %d/%d, want 1/1", got.ycMatched, got.ycUnmatched)
	}
	// bigtech hand list matched against existing: google + stripe present.
	wantBig := 2
	if got.bigMatched != wantBig {
		t.Errorf("bigtech matched = %d, want %d", got.bigMatched, wantBig)
	}
}

// A company keeps an unmanaged tag through reconciliation even when it gains a
// managed one.
func TestPlan_PreservesUnmanagedTag(t *testing.T) {
	rows := []db.ListCompanyCollectionsRow{
		{Slug: "stripe", Collections: []string{"custom"}},
	}
	got := plan(rows, []string{"Stripe"})
	if len(got.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(got.writes))
	}
	c := got.writes[0].Collections
	sort.Strings(c)
	// stripe is in the bigtech hand list too, so it gains both managed tags.
	if !reflect.DeepEqual(c, []string{"bigtech", "custom", "yc"}) {
		t.Errorf("collections = %#v, want [bigtech custom yc]", c)
	}
}
