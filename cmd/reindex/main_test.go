package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

func TestPostedWithinFrom(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantDur time.Duration
		wantOK  bool
		wantErr bool
	}{
		{"absent", []string{"--semantic"}, 0, false, false},
		{"space form", []string{"--semantic", "--posted-within", "168h"}, 168 * time.Hour, true, false},
		{"equals form", []string{"--posted-within=720h"}, 720 * time.Hour, true, false},
		{"missing value", []string{"--posted-within"}, 0, false, true},
		{"unparseable", []string{"--posted-within", "7d"}, 0, false, true},
		{"non-positive", []string{"--posted-within", "0h"}, 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dur, ok, err := postedWithinFrom(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOK || dur != tt.wantDur {
				t.Errorf("got (%v, %v), want (%v, %v)", dur, ok, tt.wantDur, tt.wantOK)
			}
		})
	}
}

func TestSemanticRequested(t *testing.T) {
	if !semanticRequested([]string{"--since", "50h", "--semantic"}) {
		t.Error("--semantic should be detected among other args")
	}
	if semanticRequested([]string{"--since", "50h"}) {
		t.Error("facet pass must not be misread as semantic")
	}
}

func TestFromPGRequested(t *testing.T) {
	if !fromPGRequested([]string{"--semantic", "--from-pg"}) {
		t.Error("--from-pg should be detected among other args")
	}
	if fromPGRequested([]string{"--semantic"}) {
		t.Error("absent --from-pg must not be reported")
	}
}

// The reindex feed deliberately includes closed jobs: open ones are upserted as
// documents, closed ones become deletions so they leave the index (job-search
// spec: the index contains only open jobs).
func TestSplitJobs_OpenBecomeDocsClosedBecomeDeletions(t *testing.T) {
	open := db.Job{ID: 1, Title: "Open", PublicSlug: "open-x"}
	closed := db.Job{ID: 2, Title: "Closed", PublicSlug: "closed-x",
		ClosedAt: pgtype.Timestamptz{Time: open.CreatedAt.Time, Valid: true}}

	docs, deleteIDs, err := splitJobs([]db.Job{open, closed}, nil, time.Now())
	if err != nil {
		t.Fatalf("splitJobs: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != 1 {
		t.Fatalf("docs = %+v, want only the open job", docs)
	}
	if len(deleteIDs) != 1 || deleteIDs[0] != 2 {
		t.Fatalf("deleteIDs = %v, want [2]", deleteIDs)
	}
}

// A non-canonical repost (duplicate_of set) must not be indexed, and is deleted from
// the index so a previously-canonical row that got demoted leaves search.
func TestSplitJobs_RepostsDeletedNotIndexed(t *testing.T) {
	canon := db.Job{ID: 1, Title: "Canon", PublicSlug: "canon-x"}
	repost := db.Job{ID: 2, Title: "Repost", PublicSlug: "repost-x",
		DuplicateOf: pgtype.Int8{Int64: 1, Valid: true}}

	docs, deleteIDs, err := splitJobs([]db.Job{canon, repost}, nil, time.Now())
	if err != nil {
		t.Fatalf("splitJobs: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != 1 {
		t.Fatalf("docs = %+v, want only the canon", docs)
	}
	if len(deleteIDs) != 1 || deleteIDs[0] != 2 {
		t.Fatalf("deleteIDs = %v, want [2] (repost removed)", deleteIDs)
	}
}

// fakeRebuilder records the sequence of rebuilder calls and the ids of every
// pushed batch, so a unit test can pin the full-rebuild orchestration without a
// real Meilisearch.
type fakeRebuilder struct {
	calls  []string
	pushed [][]int64
}

func (f *fakeRebuilder) Prepare(context.Context) error {
	f.calls = append(f.calls, "prepare")
	return nil
}

func (f *fakeRebuilder) Push(_ context.Context, docs []search.JobDocument) error {
	f.calls = append(f.calls, "push")
	ids := make([]int64, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	f.pushed = append(f.pushed, ids)
	return nil
}

func (f *fakeRebuilder) Promote(context.Context) error {
	f.calls = append(f.calls, "promote")
	return nil
}

// A full rebuild streams ONLY open jobs into a fresh index (closed jobs are
// simply absent — the fresh index never held them, so there is nothing to
// delete), then promotes exactly once at the end.
func TestReindexFull_PushesOpenDocsThenPromotes(t *testing.T) {
	open1 := db.Job{ID: 1, Title: "A", PublicSlug: "a"}
	closed := db.Job{ID: 2, Title: "B", PublicSlug: "b",
		ClosedAt: pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true}}
	open3 := db.Job{ID: 3, Title: "C", PublicSlug: "c"}
	page := []db.Job{open1, closed, open3}

	reader := &fakePageReader{pages: map[int64][]db.Job{0: page}}

	f := &fakeRebuilder{}
	indexed, skipped, err := reindexFull(context.Background(), reader, f, nil, time.Now())
	if err != nil {
		t.Fatalf("reindexFull: %v", err)
	}
	if indexed != 2 {
		t.Errorf("indexed = %d, want 2 (open jobs only)", indexed)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	wantCalls := []string{"prepare", "push", "promote"}
	if !reflect.DeepEqual(f.calls, wantCalls) {
		t.Errorf("calls = %v, want %v", f.calls, wantCalls)
	}
	if len(f.pushed) != 1 || !reflect.DeepEqual(f.pushed[0], []int64{1, 3}) {
		t.Errorf("pushed = %v, want [[1 3]] (closed job 2 skipped)", f.pushed)
	}
}

// A corrupted row (its Batch read faults with XX001) must not abort the rebuild:
// ResilientPage degrades to per-row reads, the corrupted row is skipped, the rest
// are indexed, and the fresh index still promotes.
func TestReindexFull_SkipsCorruptedRowAndPromotes(t *testing.T) {
	reader := &fakePageReader{
		corruptAfter: map[int64]bool{0: true},
		idPages:      map[int64][]int64{0: {1, 2, 3}},
		rows: map[int64]db.Job{
			1: {ID: 1, Title: "A", PublicSlug: "a"},
			3: {ID: 3, Title: "C", PublicSlug: "c"},
		},
		rowCorrupt: map[int64]bool{2: true},
	}

	f := &fakeRebuilder{}
	indexed, skipped, err := reindexFull(context.Background(), reader, f, nil, time.Now())
	if err != nil {
		t.Fatalf("reindexFull: %v", err)
	}
	if indexed != 2 || skipped != 1 {
		t.Errorf("indexed=%d skipped=%d, want indexed=2 skipped=1", indexed, skipped)
	}
	if len(f.pushed) == 0 || !reflect.DeepEqual(f.pushed[0], []int64{1, 3}) {
		t.Errorf("pushed = %v, want [[1 3]] (corrupted row 2 skipped)", f.pushed)
	}
	if f.calls[len(f.calls)-1] != "promote" {
		t.Errorf("expected promote at the end, calls = %v", f.calls)
	}
}

// fakePageReader implements worker.PageReader. Batch returns pages[afterID], or
// faults with XX001 when corruptAfter[afterID] is set (driving the degrade path,
// which then reads idPages[afterID] and rows[id], faulting on rowCorrupt[id]).
type fakePageReader struct {
	pages        map[int64][]db.Job
	corruptAfter map[int64]bool
	idPages      map[int64][]int64
	rows         map[int64]db.Job
	rowCorrupt   map[int64]bool
}

func xx001() error { return &pgconn.PgError{Code: "XX001"} }

func (f *fakePageReader) Batch(_ context.Context, afterID int64, _ int32) ([]db.Job, error) {
	if f.corruptAfter[afterID] {
		return nil, xx001()
	}
	return f.pages[afterID], nil
}

func (f *fakePageReader) IDs(_ context.Context, afterID int64, _ int32) ([]int64, error) {
	return f.idPages[afterID], nil
}

func (f *fakePageReader) Row(_ context.Context, id int64) (db.Job, error) {
	if f.rowCorrupt[id] {
		return db.Job{}, xx001()
	}
	return f.rows[id], nil
}
