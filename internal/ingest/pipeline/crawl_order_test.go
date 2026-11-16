package pipeline

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

type orderedBlockingSource struct {
	mu      sync.Mutex
	started []int
}

func (*orderedBlockingSource) Provider() string { return "join" }
func (s *orderedBlockingSource) Fetch(ctx context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	n, _ := strconv.Atoi(e.Board)
	s.mu.Lock()
	s.started = append(s.started, n)
	s.mu.Unlock()
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestTruncatedRunAdmitsOldestBoardsFirst(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	src := &orderedBlockingSource{}
	entries := make([]sources.CompanyEntry, 64)
	for i := range entries {
		entries[i] = sources.CompanyEntry{Provider: "join", Company: "Acme", Board: fmt.Sprint(i)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r := Runner{Registry: registry(src), Store: &fakeStore{}}
	_, err := r.Run(ctx, entries)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want deadline", err)
	}
	if len(src.started) != defaultConcurrency {
		t.Fatalf("started %v, want %d boards", src.started, defaultConcurrency)
	}
	for _, n := range src.started {
		if n >= defaultConcurrency {
			t.Errorf("board %d reached before the oldest waiting boards; started %v", n, src.started)
		}
	}
}
