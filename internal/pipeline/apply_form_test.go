package pipeline

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/applyform"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/sources"
)

// formStore is a Store that also accepts a posting's application form — the shape the
// ingest store has and a test fake deliberately does not.
type formStore struct {
	fakeStore
	forms map[string]applyform.Form
}

func (s *formStore) SaveWithApplyForm(ctx context.Context, j job.Job, form applyform.Form) error {
	if err := s.Save(ctx, j); err != nil {
		return err
	}
	if s.forms == nil {
		s.forms = map[string]applyform.Form{}
	}
	s.forms[j.Fields().ExternalID] = form
	return nil
}

// formSource yields one posting, carrying a form or not.
type formSource struct{ form *applyform.Form }

func (formSource) Provider() string { return "recruitee" }

func (s formSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{
		ExternalID:  "1",
		Title:       "Backend Engineer",
		Company:     "Acme",
		URL:         "https://acme.recruitee.com/o/be",
		Description: "Build things.",
		ApplyForm:   s.form,
	}}, nil
}

func runOneBoard(t *testing.T, store Store, form *applyform.Form) Stats {
	t.Helper()
	r := Runner{
		Store:    store,
		Registry: map[string]sources.Source{"recruitee": formSource{form: form}},
	}
	all, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "recruitee", Board: "acme"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := all["recruitee"]
	if st.Ingested != 1 {
		t.Fatalf("ingested = %d, want 1 (stats: %+v)", st.Ingested, st)
	}
	return st
}

func TestSaveCarriesTheApplyFormWhenTheStoreAcceptsOne(t *testing.T) {
	store := &formStore{}
	form := applyform.Form{Provider: "recruitee", Fields: []applyform.Field{{ID: "cv", Label: "CV"}}}

	runOneBoard(t, store, &form)

	got, ok := store.forms["acme:1"]
	if !ok {
		t.Fatalf("no form stored, got %v", store.forms)
	}
	if len(got.Fields) != 1 || got.Fields[0].ID != "cv" {
		t.Errorf("stored form = %+v, want the one the adapter yielded", got)
	}
}

// A posting whose adapter yielded no form takes the ordinary path — nothing about the
// write changes for the platforms that describe no form in their listing.
func TestSaveTakesThePlainPathWithoutAForm(t *testing.T) {
	store := &formStore{}

	runOneBoard(t, store, nil)

	if len(store.forms) != 0 {
		t.Errorf("stored %v, want no form when the adapter yielded none", store.forms)
	}
	if len(store.saved) != 1 {
		t.Errorf("saved %d jobs, want the posting written as usual", len(store.saved))
	}
}

// A Store that cannot take a form must still ingest the posting. The capability is
// discovered by type assertion exactly like Closer and Toucher, so a fake — or any future
// Store — degrades to writing the job alone rather than failing the crawl.
func TestAStoreWithoutTheCapabilityStillIngests(t *testing.T) {
	store := &fakeStore{}
	form := applyform.Form{Provider: "recruitee"}

	runOneBoard(t, store, &form)

	if len(store.saved) != 1 {
		t.Errorf("saved %d jobs, want the posting written through the plain Save", len(store.saved))
	}
}
