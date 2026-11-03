package pipeline

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

type websiteStore struct {
	fakeStore
	websites map[string]string
}

func (s *websiteStore) RecordCompanyWebsite(_ context.Context, slug, website string) error {
	if s.websites == nil {
		s.websites = map[string]string{}
	}
	s.websites[slug] = website
	return nil
}

type websiteSource struct{ website string }

func (websiteSource) Provider() string { return "eures" }

func (s websiteSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{
		ExternalID:     "1",
		Title:          "Backend Engineer",
		Company:        "Acme GmbH",
		URL:            "https://europa.eu/eures/portal/jv-se/jv-details/1",
		Description:    "Build things.",
		CompanyWebsite: s.website,
	}}, nil
}

func runWebsite(t *testing.T, website string) *websiteStore {
	t.Helper()
	store := &websiteStore{}
	r := Runner{Store: store, Registry: map[string]sources.Source{"eures": websiteSource{website: website}}}
	all, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "EURES", Provider: "eures", Board: "de"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if all["eures"].Ingested != 1 {
		t.Fatalf("ingested = %d, want 1", all["eures"].Ingested)
	}
	return store
}

// The website lands on the company the posting was filed under, by its company slug.
func TestSavedPostingRecordsItsCompanyWebsite(t *testing.T) {
	store := runWebsite(t, " https://www.acme.de/jobs ")
	if got := store.websites["acme"]; got != "https://www.acme.de/jobs" {
		t.Errorf("websites = %v, want acme -> https://www.acme.de/jobs", store.websites)
	}
}

func TestCompanyWebsiteRefusesWhatIsNotAURL(t *testing.T) {
	for _, raw := range []string{"", "n/a", "info@acme.de", "/about", "http://localhost"} {
		if got := companyWebsite(raw); got != "" {
			t.Errorf("companyWebsite(%q) = %q, want empty", raw, got)
		}
		if store := runWebsite(t, raw); len(store.websites) != 0 {
			t.Errorf("%q recorded %v, want nothing", raw, store.websites)
		}
	}
	for _, ok := range []string{"http://www.forumjobs.be", "www.acme.de"} {
		if got := companyWebsite(ok); got != ok {
			t.Errorf("companyWebsite(%q) = %q, want it kept", ok, got)
		}
	}
}

type refreshSource struct{}

func (refreshSource) Provider() string { return "eures" }

func (refreshSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{
		ExternalID:     "1",
		Title:          "Backend Engineer",
		Company:        "Acme GmbH",
		URL:            "https://europa.eu/eures/portal/jv-se/jv-details/1",
		SeenRefresh:    true,
		CompanyWebsite: "https://acme.de",
	}}, nil
}

// An employer whose postings were all stored before websites were collected is only ever
// re-listed, so the refresh path records the website too.
func TestRefreshedPostingRecordsItsCompanyWebsite(t *testing.T) {
	store := &websiteStore{}
	r := Runner{Store: store, Registry: map[string]sources.Source{"eures": refreshSource{}}}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "EURES", Provider: "eures", Board: "de"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.touched) != 1 {
		t.Fatalf("touched = %v, want the posting refreshed", store.touched)
	}
	if got := store.websites["acme"]; got != "https://acme.de" {
		t.Errorf("websites = %v, want acme -> https://acme.de", store.websites)
	}
}
