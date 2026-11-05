package handler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeSubindustries counts loads and holds each one until release is closed, so concurrent
// callers really are in flight together.
type fakeSubindustries struct {
	calls   atomic.Int32
	release chan struct{}
	err     error
}

func (f *fakeSubindustries) load(context.Context) ([]db.CompanySubindustriesRow, error) {
	f.calls.Add(1)
	<-f.release
	if f.err != nil {
		return nil, f.err
	}
	return []db.CompanySubindustriesRow{{Value: pgtype.Text{String: "Payments", Valid: true}, Count: 2}}, nil
}

// The incident this cache answers was a stampede: every concurrent miss ran its own full
// scan of companies. N concurrent callers inside the TTL must cost one query, and the
// next query must come only after the TTL.
func TestSubindustryCacheSharesOneLoadUntilExpiry(t *testing.T) {
	fake := &fakeSubindustries{release: make(chan struct{})}
	now := time.Date(2026, 11, 5, 11, 0, 0, 0, time.UTC)
	s := &subindustryCache{load: fake.load, now: func() time.Time { return now }}

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			facets, err := s.get(context.Background())
			if err == nil && (len(facets) != 1 || facets[0] != (subindustryFacet{Value: "Payments", Count: 2})) {
				err = errors.New("unexpected facets")
			}
			errs <- err
		}()
	}
	close(fake.release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("loads after %d concurrent calls = %d, want 1", n, got)
	}

	now = now.Add(subindustryCacheTTL - time.Second)
	if _, err := s.get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("loads inside the TTL = %d, want 1", got)
	}

	now = now.Add(time.Second)
	if _, err := s.get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fake.calls.Load(); got != 2 {
		t.Fatalf("loads after the TTL = %d, want 2", got)
	}
}

// A failed load must not be served from the cache: the next call queries again.
func TestSubindustryCacheDoesNotStoreFailure(t *testing.T) {
	fake := &fakeSubindustries{release: make(chan struct{}), err: errors.New("database down")}
	close(fake.release)
	s := &subindustryCache{load: fake.load, now: time.Now}

	if _, err := s.get(context.Background()); err == nil {
		t.Fatal("want the load error")
	}
	fake.err = nil
	facets, err := s.get(context.Background())
	if err != nil || len(facets) != 1 {
		t.Fatalf("after recovery: facets %v, err %v", facets, err)
	}
	if got := fake.calls.Load(); got != 2 {
		t.Fatalf("loads = %d, want 2", got)
	}
}
