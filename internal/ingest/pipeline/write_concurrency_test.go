package pipeline

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
)

// slowStore holds every Save open until saveConcurrency of them are in flight at once (or a
// deadline passes), recording the widest overlap it saw. Writes go to a database on another
// host since 2026-10-17, ~26 ms away, so a board written one posting at a time is bounded by
// that round trip — measured, energyjobline's ~54k refreshes needed ~2h against a 50-minute
// unit and the run stored nothing it fetched.
type slowStore struct {
	fakeStore
	mu       sync.Mutex
	inFlight int
	widest   int
	all      chan struct{}
	once     sync.Once
}

func (s *slowStore) Save(ctx context.Context, j job.Job) error {
	s.mu.Lock()
	s.inFlight++
	if s.inFlight > s.widest {
		s.widest = s.inFlight
	}
	if s.inFlight >= saveConcurrency {
		s.once.Do(func() { close(s.all) })
	}
	s.mu.Unlock()
	select {
	case <-s.all:
	case <-time.After(2 * time.Second):
	}
	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()
	return s.fakeStore.Save(ctx, j)
}

func TestIngestFetchedWritesABoardConcurrently(t *testing.T) {
	var jobs []sources.Job
	for i := 0; i < 3*saveConcurrency; i++ {
		jobs = append(jobs, sources.Job{ExternalID: fmt.Sprint(i), Title: "Backend Engineer", Company: "C", URL: "u"})
	}
	store := &slowStore{all: make(chan struct{})}
	r := Runner{Registry: registry(fakeSource{provider: "greenhouse", jobs: jobs}), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "C", Provider: "greenhouse", Board: "c"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := stats.Total().Ingested; got != len(jobs) {
		t.Fatalf("Ingested = %d, want %d — the merged per-worker tallies lost writes", got, len(jobs))
	}
	if store.widest < saveConcurrency {
		t.Errorf("at most %d saves were in flight at once, want %d", store.widest, saveConcurrency)
	}
}
