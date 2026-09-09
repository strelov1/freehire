package main

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/strelov1/freehire/internal/job/wikicompany"
)

// fakeStore is a tiny in-memory stand-in for the companies table, reimplementing
// just enough of ListCompaniesMissingWikipediaInfo's keyset-pagination semantics
// (slug > afterSlug, ordered, limited) to exercise the loop without a database.
// It also records every Fill/MarkChecked call so a test can prove both what was
// written and that a dry run writes nothing at all.
type fakeStore struct {
	candidates []candidate // remaining eligible companies, sorted by slug
	filled     map[string]struct {
		tagline     string
		companyInfo json.RawMessage
	}
	checked map[string]bool
}

func newFakeStore(candidates []candidate) *fakeStore {
	sorted := append([]candidate(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Slug < sorted[j].Slug })
	return &fakeStore{
		candidates: sorted,
		filled: map[string]struct {
			tagline     string
			companyInfo json.RawMessage
		}{},
		checked: map[string]bool{},
	}
}

func (f *fakeStore) ListMissing(_ context.Context, afterSlug string, limit int32) ([]candidate, error) {
	var page []candidate
	for _, c := range f.candidates {
		if c.Slug <= afterSlug {
			continue
		}
		page = append(page, c)
		if int32(len(page)) >= limit {
			break
		}
	}
	return page, nil
}

func (f *fakeStore) Fill(_ context.Context, slug, tagline string, companyInfo json.RawMessage) error {
	f.filled[slug] = struct {
		tagline     string
		companyInfo json.RawMessage
	}{tagline, companyInfo}
	return nil
}

func (f *fakeStore) MarkChecked(_ context.Context, slug string) error {
	f.checked[slug] = true
	return nil
}

// fakeMatcher looks up a fixed table of name -> match (nil meaning "no confident
// match") and records every name it was asked to resolve, in order.
type fakeMatcher struct {
	byName map[string]*wikicompany.Match
	calls  []string
}

func (f *fakeMatcher) Lookup(_ context.Context, name string) (*wikicompany.Match, error) {
	f.calls = append(f.calls, name)
	return f.byName[name], nil
}

func TestRunBackfill_DryRunWritesNothing(t *testing.T) {
	store := newFakeStore([]candidate{{Slug: "paladin-energy", Name: "Paladin Energy"}})
	matcher := &fakeMatcher{byName: map[string]*wikicompany.Match{
		"Paladin Energy": {Tagline: "Uranium company based in Western Australia"},
	}}

	res, err := runBackfill(context.Background(), store, matcher, false, 10)
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if res.Matched != 1 || res.Rejected != 0 {
		t.Errorf("result = %+v, want 1 matched, 0 rejected", res)
	}
	if len(store.filled) != 0 || len(store.checked) != 0 {
		t.Errorf("dry run wrote to the store: filled=%v checked=%v", store.filled, store.checked)
	}
}

func TestRunBackfill_ApplyFillsConfidentMatch(t *testing.T) {
	store := newFakeStore([]candidate{{Slug: "paladin-energy", Name: "Paladin Energy"}})
	matcher := &fakeMatcher{byName: map[string]*wikicompany.Match{
		"Paladin Energy": {
			Tagline: "Uranium company based in Western Australia",
			Summary: "Paladin Energy Ltd is a uranium producer.",
		},
	}}

	res, err := runBackfill(context.Background(), store, matcher, true, 10)
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if res.Matched != 1 {
		t.Errorf("Matched = %d, want 1", res.Matched)
	}
	got, ok := store.filled["paladin-energy"]
	if !ok {
		t.Fatal("expected a Fill write for paladin-energy")
	}
	if got.tagline != "Uranium company based in Western Australia" {
		t.Errorf("tagline written = %q", got.tagline)
	}
	var info map[string]string
	if err := json.Unmarshal(got.companyInfo, &info); err != nil {
		t.Fatalf("company_info not valid JSON: %v", err)
	}
	if info["summary"] != "Paladin Energy Ltd is a uranium producer." {
		t.Errorf("company_info.summary = %q", info["summary"])
	}
	if store.checked["paladin-energy"] {
		t.Error("a matched company should be recorded via Fill, not MarkChecked")
	}
}

func TestRunBackfill_ApplyMarksRejectedChecked(t *testing.T) {
	store := newFakeStore([]candidate{{Slug: "boardroom-appointments", Name: "Boardroom Appointments"}})
	matcher := &fakeMatcher{byName: map[string]*wikicompany.Match{}} // no confident match

	res, err := runBackfill(context.Background(), store, matcher, true, 10)
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if res.Rejected != 1 || res.Matched != 0 {
		t.Errorf("result = %+v, want 0 matched, 1 rejected", res)
	}
	if !store.checked["boardroom-appointments"] {
		t.Error("expected MarkChecked for a rejected company")
	}
	if len(store.filled) != 0 {
		t.Errorf("expected no Fill write for a rejected company, got %v", store.filled)
	}
}

func TestRunBackfill_RespectsMaxPerRun(t *testing.T) {
	store := newFakeStore([]candidate{
		{Slug: "a", Name: "A"},
		{Slug: "b", Name: "B"},
		{Slug: "c", Name: "C"},
	})
	matcher := &fakeMatcher{byName: map[string]*wikicompany.Match{}}

	res, err := runBackfill(context.Background(), store, matcher, true, 2)
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if total := res.Matched + res.Rejected; total != 2 {
		t.Errorf("processed %d companies, want exactly 2 (the max-per-run bound)", total)
	}
	if len(matcher.calls) != 2 {
		t.Errorf("Lookup called %d times, want 2", len(matcher.calls))
	}
}

func TestRunBackfill_PaginatesAcrossMultiplePages(t *testing.T) {
	candidates := make([]candidate, 5)
	for i := range candidates {
		slug := string(rune('a' + i))
		candidates[i] = candidate{Slug: slug, Name: slug}
	}
	store := newFakeStore(candidates)
	matcher := &fakeMatcher{byName: map[string]*wikicompany.Match{}}

	res, err := runBackfill(context.Background(), store, matcher, true, 10)
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if total := res.Matched + res.Rejected; total != 5 {
		t.Errorf("processed %d companies, want all 5 across pages", total)
	}
	if len(matcher.calls) != 5 {
		t.Errorf("Lookup called %d times, want 5", len(matcher.calls))
	}
}
