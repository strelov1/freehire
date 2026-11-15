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

// TestRunStoresTheCanonicalCompanySlugAndGatesOnIt is the structural guarantee behind duplicate
// class 1b, and behind the coverage gate not leaking a second time.
//
// "DollarTree" derives to `dollartree`, which a merge registered as an alias of `dollar-tree`.
// The posting must be STORED under the canonical slug, and the coverage lookup must be asked
// about that same canonical slug — because what it is really being asked is "does this employer
// already have ATS coverage", and `dollartree` is not the key the catalogue holds that under.
//
// The two agreeing is not a coincidence to be maintained by hand: both read one resolved map.
func TestRunStoresTheCanonicalCompanySlugAndGatesOnIt(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "DollarTree", URL: "u"},
		{ExternalID: "2", Title: "Data Engineer", Company: "DollarTree", URL: "u"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{}}
	aliases := &fakeAliases{canon: map[string]string{"dollartree": "dollar-tree"}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage, Aliases: aliases}

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
	if len(coverage.calls) != 1 {
		t.Fatalf("coverage was called %d times, want 1 (one batch per board)", len(coverage.calls))
	}
	if got := coverage.calls[0]; len(got) != 1 || got[0] != "dollar-tree" {
		t.Errorf("coverage asked about %v, want [dollar-tree] — the gate must ask about the "+
			"slug the upsert will write, or it silently stops matching", got)
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

// TestRunGatesOnTheCanonicalSlugAtDecisionTime is the other half of the invariant. The batch
// asks the lookup about the canonical slug; this proves the SKIP decision reads it too.
//
// `dollar-tree` has ATS coverage; the aggregator posting arrives spelled `DollarTree`. Before
// the registry resolved it, the gate compared `dollartree` against a catalogue holding
// `dollar-tree`, found nothing, and saved the duplicate it exists to suppress — the leak in
// its most literal form.
func TestRunGatesOnTheCanonicalSlugAtDecisionTime(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "DollarTree", URL: "u"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"dollar-tree": true}}
	aliases := &fakeAliases{canon: map[string]string{"dollartree": "dollar-tree"}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage, Aliases: aliases}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 0 {
		t.Errorf("saved %d postings, want 0 — the employer already has ATS coverage under its "+
			"canonical slug, and the gate must recognise that through the spelling",
			len(store.saved))
	}
	if got := stats.Total().ATSCovered; got != 1 {
		t.Errorf("ATSCovered = %d, want 1", got)
	}
}

// TestRunGatesOnTheStrippedSlugWithNoRegistry catches the leak in its plainest form, with no
// alias registry involved at all.
//
// The gate's question set and its decision must be the same derivation. When the batch was
// built with normalize.Slug while the posting keyed on normalize.CompanySlug, every company
// carrying a corporate form asked about "acme-inc" and then decided on "acme" — so the lookup
// answered about a company nobody would consult, and the gate skipped nothing. Silently:
// a gate that matches nothing looks exactly like a board with nothing to suppress.
func TestRunGatesOnTheStrippedSlugWithNoRegistry(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme Inc", URL: "u"},
	}}
	store := &fakeStore{}
	coverage := &fakeCoverage{covered: map[string]bool{"acme": true}}
	r := Runner{Registry: registry(src), Store: store, Coverage: coverage}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Himalayas", Provider: "himalayas"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := coverage.calls[0]; len(got) != 1 || got[0] != "acme" {
		t.Errorf("coverage asked about %v, want [acme] — the corporate form is not part of the "+
			"company key, so it must not be part of the question", got)
	}
	if len(store.saved) != 0 {
		t.Errorf("saved %d postings, want 0 (the employer has ATS coverage)", len(store.saved))
	}
	if got := stats.Total().ATSCovered; got != 1 {
		t.Errorf("ATSCovered = %d, want 1", got)
	}
}
