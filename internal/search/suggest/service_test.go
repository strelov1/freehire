package suggest

import (
	"context"
	"slices"
	"testing"
)

type fakeIndex struct {
	hits       []Document
	gotQuery   string
	gotFilter  string
	gotLimit   int
	callCount  int
	allDocs    []Document
	failSearch error
}

func (f *fakeIndex) SearchSuggestions(_ context.Context, query, filter string, limit int) ([]Document, error) {
	f.callCount++
	f.gotQuery, f.gotFilter, f.gotLimit = query, filter, limit
	if f.failSearch != nil {
		return nil, f.failSearch
	}
	return f.hits, nil
}

func (f *fakeIndex) AllSuggestions(context.Context) ([]Document, error) { return f.allDocs, nil }

func doc(kind Kind, slug, text string, jobs int) Document {
	return Document{ID: string(kind) + ":" + slug, Kind: kind, Slug: slug, Text: text, Jobs: jobs}
}

func svc(idx *fakeIndex, known ...string) *Service {
	s := New(idx)
	s.phrases = phrases(known...)
	return s
}

// An empty box IS answered — with the filter modal's curated category order — but not
// from here. That order is presentation, it lives in the web, and it is checked there
// against the category vocabulary at compile time; a copy in Go would be a second list
// that agrees until one of them is edited. So this returns nothing and, more to the
// point, does not go asking the index for a ranking it must not use.
func TestService_EmptyQueryAsksTheIndexNothing(t *testing.T) {
	idx := &fakeIndex{}
	got, err := svc(idx).Suggest(context.Background(), "   ", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v", got)
	}
	if idx.callCount != 0 {
		t.Error("an empty query needs no index round trip")
	}
}

func TestService_CompletesTheFragment(t *testing.T) {
	idx := &fakeIndex{hits: []Document{doc(KindCompany, "google", "Google", 3187)}}
	s := svc(idx, "Senior Software Engineer")

	got, err := s.Suggest(context.Background(), "senior software engineer go", 10)
	if err != nil {
		t.Fatal(err)
	}
	if idx.gotQuery != "go" {
		t.Errorf("queried %q, want the fragment %q", idx.gotQuery, "go")
	}
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	// The row names the WHOLE phrase and applies every part of it: applying one of the
	// two would silently discard what the visitor typed.
	if len(got[0].Parts) != 2 {
		t.Fatalf("parts = %+v, want the role and the company", got[0].Parts)
	}
	if got[0].Text != "Senior Software Engineer Google" {
		t.Errorf("text = %q", got[0].Text)
	}
}

func TestService_ExcludesAKindTheQueryAlreadyNamed(t *testing.T) {
	idx := &fakeIndex{}
	s := svc(idx, "product owner")
	if _, err := s.Suggest(context.Background(), "product owner eng", 10); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains([]string{`kind != "category"`}, idx.gotFilter) {
		t.Errorf("filter = %q, want the named kind excluded", idx.gotFilter)
	}
}

func TestService_NoFilterWhenNothingRecognised(t *testing.T) {
	idx := &fakeIndex{}
	s := svc(idx)
	if _, err := s.Suggest(context.Background(), "backedn", 10); err != nil {
		t.Fatal(err)
	}
	if idx.gotFilter != "" {
		t.Errorf("filter = %q, want none", idx.gotFilter)
	}
}

// A suggestion whose count has fallen to zero between rebuilds leads to an empty page,
// which is worse than no suggestion.
func TestService_WithholdsADrainedSuggestion(t *testing.T) {
	idx := &fakeIndex{hits: []Document{
		doc(KindCategory, "backend", "Backend Engineer", 0),
		doc(KindCategory, "frontend", "Frontend Engineer", 10),
	}}
	got, err := svc(idx).Suggest(context.Background(), "eng", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Parts[0].Slug != "frontend" {
		t.Fatalf("got %+v", got)
	}
}

func TestService_HonoursTheLimit(t *testing.T) {
	idx := &fakeIndex{}
	if _, err := svc(idx).Suggest(context.Background(), "eng", 3); err != nil {
		t.Fatal(err)
	}
	if idx.gotLimit < 3 {
		t.Errorf("asked the index for %d, want at least the limit", idx.gotLimit)
	}
}

// The refresh is what keeps recognition current with the index the builder rewrites.
func TestService_RefreshLoadsThePhrases(t *testing.T) {
	idx := &fakeIndex{allDocs: []Document{doc(KindCategory, "backend", "Backend Engineer", 10)}}
	s := New(idx)
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.phrases.Len() != 1 {
		t.Errorf("phrases = %d, want 1", s.phrases.Len())
	}
}
