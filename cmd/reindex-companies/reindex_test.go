package main

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

var errTest = errors.New("push boom")

// fakeReader serves companies in keyset pages: each Page returns rows with slug >
// afterSlug, capped at limit, over the pre-sorted input.
type fakeReader struct{ rows []db.Company }

func (f *fakeReader) Page(_ context.Context, afterSlug string, limit int32) ([]db.Company, error) {
	var out []db.Company
	for _, r := range f.rows {
		if r.Slug > afterSlug {
			out = append(out, r)
			if int32(len(out)) == limit {
				break
			}
		}
	}
	return out, nil
}

// fakeRebuild records the documents pushed and the lifecycle calls.
type fakeRebuild struct {
	prepared, promoted, cleaned int
	pushErr                     error
	pushed                      []search.CompanyDocument
}

func (f *fakeRebuild) Prepare(context.Context) error { f.prepared++; return nil }
func (f *fakeRebuild) Push(_ context.Context, docs []search.CompanyDocument) error {
	if f.pushErr != nil {
		return f.pushErr
	}
	f.pushed = append(f.pushed, docs...)
	return nil
}
func (f *fakeRebuild) Promote(context.Context) error { f.promoted++; return nil }
func (f *fakeRebuild) Cleanup(context.Context) error { f.cleaned++; return nil }

func TestReindexCompanies_StreamsAllRowsAcrossPages(t *testing.T) {
	rows := []db.Company{
		{Slug: "acme", Name: "Acme", JobCount: 3},
		{Slug: "bravo", Name: "Bravo", JobCount: 1},
		{Slug: "cirrus", Name: "Cirrus", JobCount: 9},
	}
	r := &fakeReader{rows: rows}
	b := &fakeRebuild{}

	// batchSize 2 forces multiple keyset pages (2 + 1).
	indexed, err := reindexCompanies(context.Background(), r, b, 2)
	if err != nil {
		t.Fatalf("reindexCompanies: %v", err)
	}
	if indexed != 3 {
		t.Errorf("indexed = %d, want 3", indexed)
	}
	if b.prepared != 1 || b.promoted != 1 {
		t.Errorf("lifecycle: prepared=%d promoted=%d, want 1/1", b.prepared, b.promoted)
	}
	if len(b.pushed) != 3 {
		t.Fatalf("pushed %d docs, want 3", len(b.pushed))
	}
	for i, want := range []string{"acme", "bravo", "cirrus"} {
		if b.pushed[i].Slug != want {
			t.Errorf("pushed[%d].Slug = %q, want %q (rows mapped via FromCompany in keyset order)", i, b.pushed[i].Slug, want)
		}
	}
}

func TestReindexCompanies_EmptyPromotesEmptyIndex(t *testing.T) {
	b := &fakeRebuild{}
	indexed, err := reindexCompanies(context.Background(), &fakeReader{}, b, 2)
	if err != nil {
		t.Fatalf("reindexCompanies: %v", err)
	}
	if indexed != 0 {
		t.Errorf("indexed = %d, want 0", indexed)
	}
	// Even with no companies the fresh (empty) index is prepared and promoted, so an
	// emptied catalogue swaps in cleanly rather than leaving a stale index live.
	if b.prepared != 1 || b.promoted != 1 {
		t.Errorf("lifecycle: prepared=%d promoted=%d, want 1/1", b.prepared, b.promoted)
	}
}

func TestReindexCompanies_CleansUpOnAbort(t *testing.T) {
	rows := []db.Company{{Slug: "acme", Name: "Acme", JobCount: 3}}
	b := &fakeRebuild{pushErr: errTest}

	if _, err := reindexCompanies(context.Background(), &fakeReader{rows: rows}, b, 2); err == nil {
		t.Fatal("reindexCompanies: want the push error propagated")
	}
	if b.promoted != 0 {
		t.Errorf("promoted = %d, want 0 (aborted before Promote)", b.promoted)
	}
	if b.cleaned != 1 {
		t.Errorf("cleaned = %d, want 1 (an aborted run drops the half-built rebuild index)", b.cleaned)
	}
}

func TestReindexCompanies_NoCleanupOnSuccess(t *testing.T) {
	rows := []db.Company{{Slug: "acme", Name: "Acme", JobCount: 3}}
	b := &fakeRebuild{}

	if _, err := reindexCompanies(context.Background(), &fakeReader{rows: rows}, b, 2); err != nil {
		t.Fatalf("reindexCompanies: %v", err)
	}
	if b.cleaned != 0 {
		t.Errorf("cleaned = %d, want 0 (Promote's swap-and-drop is the happy-path teardown)", b.cleaned)
	}
}
