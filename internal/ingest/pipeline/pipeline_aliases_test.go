package pipeline

import (
	"context"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// fakeAliases answers CanonicalCompanySlugs from a canned folded_key -> canonical_slug map and
// records every batch it was asked about, so a test can prove the lookup happens once per board
// rather than once per posting.
type fakeAliases struct {
	mu    sync.Mutex
	canon map[string]string
	calls [][]string
	err   error
}

func (f *fakeAliases) CanonicalCompanySlugs(_ context.Context, foldedKeys []string) (map[string]string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), foldedKeys...))
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]string)
	for _, k := range foldedKeys {
		if c, ok := f.canon[k]; ok {
			out[k] = c
		}
	}
	return out, nil
}

// Company aliases are still resolved once per board before postings are stored.
func TestRunStoresTheCanonicalCompanySlug(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "DollarTree", URL: "u"},
		{ExternalID: "2", Title: "Data Engineer", Company: "DollarTree", URL: "u"},
	}}
	store := &fakeStore{}
	aliases := &fakeAliases{canon: map[string]string{"dollartree": "dollar-tree"}}
	r := Runner{Registry: registry(src), Store: store, Aliases: aliases}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.saved) != 2 {
		t.Fatalf("len(saved) = %d, want 2", len(store.saved))
	}
	for _, j := range store.saved {
		if got := j.Fields().CompanySlug; got != "dollar-tree" {
			t.Errorf("stored CompanySlug = %q, want %q (the canonical slug, not the derived one)",
				got, "dollar-tree")
		}
	}
	if len(aliases.calls) != 1 {
		t.Errorf("alias lookup ran %d times, want 1 per board run", len(aliases.calls))
	}
	if got := aliases.calls[0]; len(got) != 1 || got[0] != "dollartree" {
		t.Errorf("alias lookup asked for %v, want [dollartree] (the folded key)", got)
	}
}

// TestRunWithoutAliasesKeepsTheDerivedSlug pins the day-one behaviour: the registry is empty
// until the merge worker writes to it, and an unwired lookup must change nothing.
func TestRunWithoutAliasesKeepsTheDerivedSlug(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "DollarTree", URL: "u"},
	}}
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := store.saved[0].Fields().CompanySlug; got != "dollartree" {
		t.Errorf("CompanySlug = %q, want %q", got, "dollartree")
	}
}
