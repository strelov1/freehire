package main

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"
)

func TestViewStepStarts(t *testing.T) {
	var starts []int64
	prev := -1
	for n := int64(0); n < 60; n++ {
		if s := viewStep(n); s != prev {
			starts = append(starts, n)
			prev = s
		}
	}
	if want := []int64{0, 5, 7, 8, 10, 13, 16, 20, 24, 30, 38, 47, 59}; !slices.Equal(starts, want) {
		t.Fatalf("steps start at %v, want %v", starts, want)
	}
}

type recordIndex struct {
	got  []map[int64]int64
	err  error
	tick func()
}

func (r *recordIndex) SetViewCounts(_ context.Context, counts map[int64]int64) error {
	r.got = append(r.got, maps.Clone(counts))
	if r.tick != nil {
		r.tick()
	}
	return r.err
}

func TestWriteViewCountsMostViewedFirst(t *testing.T) {
	counts := make([]countedJob, 0, viewCountChunk+1)
	for i := range int64(viewCountChunk + 1) {
		counts = append(counts, countedJob{id: i, n: 5 + i})
	}
	idx := &recordIndex{}
	pushed, skipped, err := writeViewCounts(context.Background(), idx, counts, time.Now().Add(time.Hour))
	if err != nil || pushed != viewCountChunk+1 || skipped != 0 {
		t.Fatalf("writeViewCounts = %d, %d, %v; want %d, 0, nil", pushed, skipped, err, viewCountChunk+1)
	}
	if len(idx.got) != 2 || len(idx.got[1]) != 1 || idx.got[1][0] != 5 {
		t.Fatalf("want the least viewed job alone in the last task, got %d tasks, last %v", len(idx.got), idx.got[len(idx.got)-1])
	}
}

func TestWriteViewCountsStopsAtDeadline(t *testing.T) {
	counts := make([]countedJob, 0, 2*viewCountChunk+1)
	for i := range int64(2*viewCountChunk + 1) {
		counts = append(counts, countedJob{id: i, n: 5 + i})
	}
	deadline := time.Now().Add(20 * time.Millisecond)
	// The first task runs past the deadline: the push stops before the next one.
	idx := &recordIndex{tick: func() { time.Sleep(time.Until(deadline)) }}
	pushed, skipped, err := writeViewCounts(context.Background(), idx, counts, deadline)
	if err != nil || pushed != viewCountChunk || skipped != viewCountChunk+1 {
		t.Fatalf("writeViewCounts = %d, %d, %v; want %d, %d, nil", pushed, skipped, err, viewCountChunk, viewCountChunk+1)
	}
	if top := int64(2*viewCountChunk + 5); idx.got[0][2*viewCountChunk] != top {
		t.Fatalf("first task misses the most viewed job (%d views)", top)
	}

	failing := &recordIndex{err: errors.New("meili down")}
	if n, _, err := writeViewCounts(context.Background(), failing, counts, time.Now().Add(time.Hour)); err == nil || n != 0 {
		t.Fatalf("failing index: writeViewCounts = %d, %v; want 0 and the error", n, err)
	}
}
