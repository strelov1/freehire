package main

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/search"
)

// fakePusher records the batches it was asked to push, and can be made to fail.
type fakePusher struct {
	mu      sync.Mutex
	batches [][]search.JobDocument
	err     error
}

func (f *fakePusher) push(_ context.Context, docs []search.JobDocument) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, append([]search.JobDocument(nil), docs...))
	return f.err
}

func (f *fakePusher) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

func (f *fakePusher) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, b := range f.batches {
		n += len(b)
	}
	return n
}

func docs(n int) []search.JobDocument {
	out := make([]search.JobDocument, n)
	for i := range out {
		out[i] = search.JobDocument{ID: int64(i + 1)}
	}
	return out
}

func TestBatchIndexer_BuffersUntilFlush(t *testing.T) {
	f := &fakePusher{}
	idx := newBatchIndexer(f.push, 10)
	ctx := context.Background()

	for _, d := range docs(3) {
		idx.Add(ctx, d)
	}
	if f.calls() != 0 {
		t.Fatalf("expected no push before flush, got %d", f.calls())
	}

	idx.Flush(ctx)
	if f.calls() != 1 || f.total() != 3 {
		t.Fatalf("expected one flush of 3 docs, got calls=%d total=%d", f.calls(), f.total())
	}
}

func TestBatchIndexer_AutoFlushesAtChunkSize(t *testing.T) {
	f := &fakePusher{}
	idx := newBatchIndexer(f.push, 10)
	ctx := context.Background()

	for _, d := range docs(23) {
		idx.Add(ctx, d)
	}
	// Two full chunks pushed automatically; 3 remain buffered.
	if f.calls() != 2 || f.total() != 20 {
		t.Fatalf("expected 2 auto-flushes of 20 docs, got calls=%d total=%d", f.calls(), f.total())
	}
	idx.Flush(ctx)
	if f.total() != 23 {
		t.Fatalf("expected 23 docs total after final flush, got %d", f.total())
	}
}

func TestBatchIndexer_EmptyFlushDoesNotPush(t *testing.T) {
	f := &fakePusher{}
	idx := newBatchIndexer(f.push, 10)
	idx.Flush(context.Background())
	if f.calls() != 0 {
		t.Fatalf("expected no push on empty flush, got %d", f.calls())
	}
}

func TestBatchIndexer_PushErrorIsSwallowed(t *testing.T) {
	f := &fakePusher{err: errors.New("meili down")}
	idx := newBatchIndexer(f.push, 10)
	ctx := context.Background()

	// Neither Add (auto-flush) nor Flush may panic or surface the error.
	for _, d := range docs(10) {
		idx.Add(ctx, d)
	}
	idx.Add(ctx, search.JobDocument{ID: 99})
	idx.Flush(ctx)

	// Failed docs are tallied, not retried into success.
	if got := idx.Stats().Failed; got != 11 {
		t.Fatalf("expected 11 failed docs, got %d", got)
	}
	if got := idx.Stats().Indexed; got != 0 {
		t.Fatalf("expected 0 indexed docs on failure, got %d", got)
	}
}

func TestBatchIndexer_ConcurrentAddIsSafe(t *testing.T) {
	f := &fakePusher{}
	idx := newBatchIndexer(f.push, 7)
	ctx := context.Background()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				idx.Add(ctx, search.JobDocument{ID: int64(g*100 + i)})
			}
		}(g)
	}
	wg.Wait()
	idx.Flush(ctx)

	if f.total() != 400 {
		t.Fatalf("expected all 400 docs pushed, got %d", f.total())
	}
	if got := idx.Stats().Indexed; got != 400 {
		t.Fatalf("expected 400 indexed, got %d", got)
	}
}
