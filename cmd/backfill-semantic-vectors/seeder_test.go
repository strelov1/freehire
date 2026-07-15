package main

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/search"
)

// fakeSource returns preloaded pages keyed by the offset the seeder requests, so a
// test asserts both the values moved and that the walk pages by pageSize.
type fakeSource struct {
	pages        map[int][]search.SemanticVector
	offsetsAsked []int
	err          error
}

func (f *fakeSource) Page(_ context.Context, offset, _ int) ([]search.SemanticVector, error) {
	f.offsetsAsked = append(f.offsetsAsked, offset)
	if f.err != nil {
		return nil, f.err
	}
	return f.pages[offset], nil
}

type fakeSink struct {
	saved []int64
	err   error
}

func (f *fakeSink) Save(_ context.Context, vecs []search.SemanticVector) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	for _, v := range vecs {
		f.saved = append(f.saved, v.ID)
	}
	return int64(len(vecs)), nil
}

func vecs(ids ...int64) []search.SemanticVector {
	out := make([]search.SemanticVector, len(ids))
	for i, id := range ids {
		out[i] = search.SemanticVector{ID: id, Vector: []float32{float32(id)}}
	}
	return out
}

func TestSeederWalksPagesUntilEmpty(t *testing.T) {
	src := &fakeSource{pages: map[int][]search.SemanticVector{
		0:  vecs(1, 2),
		10: vecs(3, 4),
		20: nil, // end
	}}
	sink := &fakeSink{}

	st, err := seeder{src: src, sink: sink, pageSize: 10}.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if st.Fetched != 4 || st.Saved != 4 || st.Pages != 2 {
		t.Fatalf("stats = %+v; want Fetched=4 Saved=4 Pages=2", st)
	}
	// Offset must advance by pageSize (0,10,20), independent of vectors returned.
	if len(src.offsetsAsked) != 3 || src.offsetsAsked[0] != 0 || src.offsetsAsked[1] != 10 || src.offsetsAsked[2] != 20 {
		t.Fatalf("offsets asked = %v; want [0 10 20]", src.offsetsAsked)
	}
	if len(sink.saved) != 4 {
		t.Fatalf("sink saved %d ids; want 4", len(sink.saved))
	}
}

func TestSeederPropagatesSourceError(t *testing.T) {
	boom := errors.New("meili down")
	_, err := seeder{src: &fakeSource{err: boom}, sink: &fakeSink{}, pageSize: 10}.run(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v; want %v", err, boom)
	}
}

func TestSeederPropagatesSinkError(t *testing.T) {
	boom := errors.New("pg down")
	src := &fakeSource{pages: map[int][]search.SemanticVector{0: vecs(1)}}
	_, err := seeder{src: src, sink: &fakeSink{err: boom}, pageSize: 10}.run(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v; want %v", err, boom)
	}
}
