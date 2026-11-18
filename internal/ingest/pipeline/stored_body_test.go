package pipeline

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
)

// bodyStore is a fakeStore that also answers what body the catalogue holds per identity.
type bodyStore struct {
	fakeStore
	bodies  map[string]string // "source/external_id" -> stored description
	bodyErr error
	asked   []string
}

func (s *bodyStore) StoredDescription(_ context.Context, source, externalID string) (string, error) {
	s.asked = append(s.asked, source+"/"+externalID)
	return s.bodies[source+"/"+externalID], s.bodyErr
}

// A failed detail fetch yields an empty description and UpsertJob keeps the stored one, so the
// facets must be derived from that stored body — not from nothing over it.
func TestSaveDerivesAnEmptyBodyFromTheStoredOne(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme", URL: "u"},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Acme", URL: "u", Description: "<p>Ruby on Rails.</p>"},
	}}
	store := &bodyStore{bodies: map[string]string{"greenhouse/acme:1": "<p>We build services in Python on PostgreSQL.</p>"}}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "greenhouse", Board: "acme"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.saved) != 2 {
		t.Fatalf("saved %d postings, want 2", len(store.saved))
	}
	i := slices.IndexFunc(store.saved, func(j job.Job) bool { return j.Fields().ExternalID == "acme:1" })
	if i < 0 {
		t.Fatal("the empty-body posting was not saved")
	}
	got := store.saved[i].Fields()
	if got.Description != store.bodies["greenhouse/acme:1"] {
		t.Errorf("description = %q, want the stored body", got.Description)
	}
	if !slices.Contains(got.Skills, "python") || !slices.Contains(got.Skills, "postgresql") {
		t.Errorf("skills = %v, want those of the stored body", got.Skills)
	}
	// The body was not read, so the write must not count it as freshly hydrated.
	if !got.UpsertParams().BodyFromCatalogue {
		t.Error("a substituted body must reach the write as not fetched")
	}
	other := store.saved[1-i].Fields()
	if other.UpsertParams().BodyFromCatalogue {
		t.Error("a body the crawl brought must reach the write as fetched")
	}
	// A posting that brought its own body costs no lookup.
	if !slices.Equal(store.asked, []string{"greenhouse/acme:1"}) {
		t.Errorf("lookups = %v, want only the empty-body posting", store.asked)
	}
}

// A lookup that fails must not fall back to deriving from nothing: the posting is skipped
// and the next crawl retries it.
func TestSaveSkipsAnEmptyBodyWhenTheStoredOneCannotBeRead(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "Backend Engineer", Company: "Acme", URL: "u"}}}
	store := &bodyStore{bodyErr: errors.New("db down")}
	r := Runner{Registry: registry(src), Store: store}

	stats, _ := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "greenhouse", Board: "acme"}})
	if len(store.saved) != 0 || stats.Total().Skipped != 1 {
		t.Errorf("saved = %d, skipped = %d; want the posting skipped, nothing saved", len(store.saved), stats.Total().Skipped)
	}
	// Still listed, so its liveness is refreshed, or the sweep could close a live posting.
	if got := store.refreshed["greenhouse"]; !slices.Equal(got, []string{"acme:1"}) {
		t.Errorf("refreshed = %v, want the skipped posting's liveness refreshed", got)
	}
}
