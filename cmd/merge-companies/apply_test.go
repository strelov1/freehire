package main

import (
	"context"
	"errors"
	"testing"
)

// fakeStore records the writes an apply run made, in order, so a test can prove both WHAT was
// written and that a dry run wrote nothing at all.
type fakeStore struct {
	jobs      map[string]int // company_slug -> open jobs still carrying it
	aliases   map[string]string
	rekeys    []string // canonical<-alias, one entry per chunk statement that moved rows
	failRekey error
}

func newFakeStore(jobs map[string]int) *fakeStore {
	return &fakeStore{jobs: jobs, aliases: map[string]string{}}
}

func (f *fakeStore) InsertAlias(_ context.Context, a alias, canonical, foldedKey string) error {
	if _, ok := f.aliases[a.Slug]; ok {
		return nil // ON CONFLICT DO NOTHING: the canon is frozen at first merge
	}
	f.aliases[a.Slug] = canonical
	return nil
}

func (f *fakeStore) RekeyChunk(_ context.Context, aliasSlug, canonical string, chunk int) (int64, error) {
	if f.failRekey != nil {
		return 0, f.failRekey
	}
	left := f.jobs[aliasSlug]
	if left == 0 {
		return 0, nil
	}
	moved := min(left, chunk)
	f.jobs[aliasSlug] -= moved
	f.jobs[canonical] += moved
	f.rekeys = append(f.rekeys, canonical+"<-"+aliasSlug)
	return int64(moved), nil
}

func plan() []merge {
	return []merge{{
		Canonical: "dollar-tree",
		FoldedKey: "dollartree",
		Jobs:      22966,
		Aliases:   []alias{{Slug: "dollartree", Reason: reasonSpelling, JobCount: 283}},
	}}
}

func TestApplyMerges_MovesEveryJobInChunks(t *testing.T) {
	store := newFakeStore(map[string]int{"dollartree": 283, "dollar-tree": 22683})

	moved, err := applyMerges(context.Background(), store, plan(), 100)
	if err != nil {
		t.Fatalf("applyMerges: %v", err)
	}
	if moved != 283 {
		t.Errorf("moved %d jobs, want 283", moved)
	}
	if store.jobs["dollartree"] != 0 {
		t.Errorf("%d jobs still on the retired slug, want 0", store.jobs["dollartree"])
	}
	if store.jobs["dollar-tree"] != 22966 {
		t.Errorf("canonical holds %d jobs, want 22966", store.jobs["dollar-tree"])
	}
	if len(store.rekeys) != 3 {
		t.Errorf("ran %d chunk statements, want 3 (283 jobs at 100 a chunk)", len(store.rekeys))
	}
	if store.aliases["dollartree"] != "dollar-tree" {
		t.Errorf("alias row = %q, want dollar-tree", store.aliases["dollartree"])
	}
}

// TestApplyMerges_IsIdempotent: a second run over the same plan must move nothing. The chunk
// statement selects rows still carrying the OLD slug, so an updated row leaves the set — which
// is also why stopping a wave mid-way is free.
func TestApplyMerges_IsIdempotent(t *testing.T) {
	store := newFakeStore(map[string]int{"dollartree": 283, "dollar-tree": 22683})
	if _, err := applyMerges(context.Background(), store, plan(), 100); err != nil {
		t.Fatalf("first run: %v", err)
	}
	before := len(store.rekeys)

	moved, err := applyMerges(context.Background(), store, plan(), 100)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if moved != 0 {
		t.Errorf("second run moved %d jobs, want 0", moved)
	}
	if len(store.rekeys) != before {
		t.Errorf("second run ran %d more chunk statements, want 0", len(store.rekeys)-before)
	}
}

// TestApplyMerges_RecordsTheAliasBeforeMovingJobs pins the write order. If the jobs moved first
// and the process died, the old slug would 404 with nothing recording where it went; with the
// alias first, the worst case is a redirect to a company that has not gained its jobs yet.
func TestApplyMerges_RecordsTheAliasBeforeMovingJobs(t *testing.T) {
	store := newFakeStore(map[string]int{"dollartree": 283})
	store.failRekey = errors.New("db down")

	if _, err := applyMerges(context.Background(), store, plan(), 100); err == nil {
		t.Fatal("applyMerges returned nil, want the re-key error")
	}
	if store.aliases["dollartree"] != "dollar-tree" {
		t.Error("the alias was not recorded before the re-key failed — the retired slug would " +
			"404 with nothing saying where it went")
	}
}
