package pipeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
)

type batchBodyStore struct {
	fakeStore
	mu       sync.Mutex
	bodies   map[string]string
	batches  []int
	singles  int
	batchErr error
}

func (s *batchBodyStore) StoredDescriptions(_ context.Context, _ string, ids []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches = append(s.batches, len(ids))
	return s.bodies, s.batchErr
}
func (s *batchBodyStore) StoredDescription(_ context.Context, _ string, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.singles++
	return s.bodies[id], nil
}
func TestBufferedBodiesKeepStoredEvidenceAndProvenance(t *testing.T) {
	e := sources.CompanyEntry{Provider: "spainjobs", Company: "SpainJobs"}
	raw := []sources.Job{{ExternalID: "1", Title: "Engineering Manager", Company: "Acme", URL: "https://example.com/1"}, {ExternalID: "2", Title: "Software Engineer", Company: "Covered", URL: "https://example.com/2"}}
	s := &batchBodyStore{bodies: map[string]string{":1": "Build Python services on PostgreSQL."}}
	r := Runner{Store: s}
	got := r.writeBoard(context.Background(), e, raw, nil)
	if got.st.Ingested != 2 || got.st.Skipped != 0 {
		t.Fatalf("stats=%+v", got.st)
	}
	if s.singles != 0 || !slices.Equal(s.batches, []int{2}) {
		t.Fatalf("singles=%d batches=%v", s.singles, s.batches)
	}
	i := slices.IndexFunc(s.saved, func(j job.Job) bool { return j.Fields().ExternalID == ":1" })
	if i < 0 {
		t.Fatal("posting with stored body was not saved")
	}
	f := s.saved[i].Fields()
	if !f.UpsertParams().BodyFromCatalogue || !slices.Contains(f.Skills, "python") {
		t.Fatalf("stored evidence/provenance lost: %+v", f)
	}
}
func TestBufferedBodiesBoundQueriesAndFallBackOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			raw := make([]sources.Job, 1001)
			for i := range raw {
				raw[i] = sources.Job{ExternalID: fmt.Sprint(i), Title: "Software Engineer", Company: "Covered", URL: "https://example.com/job"}
			}
			s := &batchBodyStore{}
			if fail {
				s.batchErr = errors.New("batch unavailable")
			}
			got := (Runner{Store: s}).writeBoard(context.Background(), sources.CompanyEntry{Provider: "spainjobs"}, raw, nil)
			if got.st.Ingested != 1001 || got.st.Skipped != 0 {
				t.Fatalf("stats=%+v", got.st)
			}
			if !slices.Equal(s.batches, []int{500, 500, 1}) {
				t.Fatalf("batches=%v", s.batches)
			}
			want := 0
			if fail {
				want = 1001
			}
			if s.singles != want {
				t.Fatalf("single lookups=%d want %d", s.singles, want)
			}
		})
	}
}

func TestBufferedDuplicateIdentitiesReadTheCurrentBody(t *testing.T) {
	s := &batchBodyStore{bodies: map[string]string{":repeat": "Build Python services."}}
	raw := []sources.Job{
		{ExternalID: "repeat", Title: "Software Engineer", Company: "Acme", Description: "Build Python services.", URL: "https://example.com/job"},
		{ExternalID: "repeat", Title: "Software Engineer", Company: "Acme", URL: "https://example.com/job"},
	}
	got := (Runner{Store: s}).writeBoard(context.Background(), sources.CompanyEntry{Provider: "spainjobs"}, raw, nil)
	if got.st.Ingested != 2 || s.singles != 1 || len(s.batches) != 0 {
		t.Fatalf("stats=%+v singles=%d batches=%v", got.st, s.singles, s.batches)
	}
}
