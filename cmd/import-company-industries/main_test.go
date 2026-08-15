package main

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

// fakeStore is the companies table reduced to what the two walks touch: a slug to
// industries map, paged by slug the way the real keyset query does.
type fakeStore struct {
	rows   map[string][]string
	writes int
}

func (f *fakeStore) ListCompanyIndustriesPage(_ context.Context, arg db.ListCompanyIndustriesPageParams) ([]db.ListCompanyIndustriesPageRow, error) {
	slugs := make([]string, 0, len(f.rows))
	for slug := range f.rows {
		if slug > arg.AfterSlug {
			slugs = append(slugs, slug)
		}
	}
	slices.Sort(slugs)
	if len(slugs) > int(arg.PageLimit) {
		slugs = slugs[:arg.PageLimit]
	}
	out := make([]db.ListCompanyIndustriesPageRow, 0, len(slugs))
	for _, slug := range slugs {
		out = append(out, db.ListCompanyIndustriesPageRow{Slug: slug, Industries: f.rows[slug]})
	}
	return out, nil
}

func (f *fakeStore) SetCompanyIndustries(_ context.Context, arg db.SetCompanyIndustriesParams) (int64, error) {
	// Mirrors the query's IS DISTINCT FROM guard, so "rewrote N rows" means the
	// same thing in a test as it does against Postgres.
	if slices.Equal(f.rows[arg.Slug], arg.Industries) {
		return 0, nil
	}
	f.rows[arg.Slug] = arg.Industries
	f.writes++
	return 1, nil
}

func TestNormalizeStored(t *testing.T) {
	s := &fakeStore{rows: map[string][]string{
		"already-clean": {"ai", "fintech"},
		"needs-folding": {"Artificial Intelligence", "FinTech"},
		"has-unknowns":  {"Healthcare", "Engineering, Product and Design", "Tech, Software & IT Services"},
		"empty":         {},
	}}

	changed, dropped, err := normalizeStored(context.Background(), s)
	if err != nil {
		t.Fatalf("normalizeStored: %v", err)
	}

	if want := []string{"ai", "fintech"}; !reflect.DeepEqual(s.rows["needs-folding"], want) {
		t.Errorf("needs-folding = %q, want %q", s.rows["needs-folding"], want)
	}
	if want := []string{"healthcare"}; !reflect.DeepEqual(s.rows["has-unknowns"], want) {
		t.Errorf("has-unknowns = %q, want %q — values outside the dictionary are dropped",
			s.rows["has-unknowns"], want)
	}
	if changed != 2 {
		t.Errorf("changed = %d, want 2: the clean and the empty row must not be rewritten", changed)
	}
	if dropped["Engineering, Product and Design"] != 1 || dropped["Tech, Software & IT Services"] != 1 {
		t.Errorf("dropped = %v, want both unrecognized labels tallied", dropped)
	}
	if _, ok := dropped["Healthcare"]; ok {
		t.Errorf("a label that resolved was reported as dropped: %v", dropped)
	}
}

func TestNormalizeStoredIsIdempotent(t *testing.T) {
	s := &fakeStore{rows: map[string][]string{"c": {"Artificial Intelligence"}}}

	if _, _, err := normalizeStored(context.Background(), s); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	before := s.writes

	changed, _, err := normalizeStored(context.Background(), s)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if changed != 0 || s.writes != before {
		t.Errorf("second pass rewrote %d rows (%d writes), want none", changed, s.writes-before)
	}
}

func TestMergeSourceUnionsIntoStoredValues(t *testing.T) {
	s := &fakeStore{rows: map[string][]string{
		"kept":      {"logistics"},
		"untouched": {"retail"},
	}}
	byKey := map[string][]string{
		"kept":    {"fintech"},
		"missing": {"ai"}, // matches no company; must not create one
	}

	merged, err := mergeSource(context.Background(), s, byKey)
	if err != nil {
		t.Fatalf("mergeSource: %v", err)
	}

	if want := []string{"fintech", "logistics"}; !reflect.DeepEqual(s.rows["kept"], want) {
		t.Errorf("kept = %q, want the sorted union %q", s.rows["kept"], want)
	}
	if want := []string{"retail"}; !reflect.DeepEqual(s.rows["untouched"], want) {
		t.Errorf("untouched = %q, want %q", s.rows["untouched"], want)
	}
	if _, ok := s.rows["missing"]; ok {
		t.Error("mergeSource created a company that was not in the table")
	}
	if merged != 1 {
		t.Errorf("merged = %d, want 1", merged)
	}
}

func TestParseSource(t *testing.T) {
	in := strings.Join([]string{
		`{"slug":"circle-com","name":"Circle","markets":["Fintech","Blockchain"]}`,
		`{"slug":"acme","name":"Acme","markets":["Nonsense-Tag-Nobody-Knows"]}`,
		``,
		`{"slug":"dup","name":"Dup","markets":["Retail","retail","RETAIL"]}`,
	}, "\n")

	got, err := parseSource(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}

	// Indexed under both keys: dump slugs are domain-derived ("circle-com") while
	// ours come from the company name ("circle"), and neither alone matches enough
	// of our rows to be worth walking the table for.
	want := []string{"crypto", "fintech"}
	if !reflect.DeepEqual(got["circle-com"], want) {
		t.Errorf("got[%q] = %q, want %q", "circle-com", got["circle-com"], want)
	}
	if !reflect.DeepEqual(got["circle"], want) {
		t.Errorf("name-derived key missing: got[%q] = %q, want %q", "circle", got["circle"], want)
	}

	// A record whose every label is unknown must not be stored at all, or it would
	// drive a pointless UPDATE that rewrites a company's industries to nothing.
	if _, ok := got["acme"]; ok {
		t.Errorf("a record with no resolvable label should be dropped, got %q", got["acme"])
	}

	if w := []string{"retail"}; !reflect.DeepEqual(got["dup"], w) {
		t.Errorf("got[%q] = %q, want %q", "dup", got["dup"], w)
	}
}

func TestParseSourceRejectsMalformedJSON(t *testing.T) {
	if _, err := parseSource(strings.NewReader(`{"slug":"broken"`)); err == nil {
		t.Error("parseSource accepted malformed JSON; a truncated dump must fail loudly")
	}
}
