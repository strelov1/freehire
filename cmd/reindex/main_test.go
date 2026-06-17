package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

func TestSinceFrom(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantDur time.Duration
		wantOK  bool
		wantErr bool
	}{
		{"absent", []string{"--semantic"}, 0, false, false},
		{"space form", []string{"--semantic", "--since", "50h"}, 50 * time.Hour, true, false},
		{"equals form", []string{"--since=24h"}, 24 * time.Hour, true, false},
		{"missing value", []string{"--since"}, 0, false, true},
		{"unparseable", []string{"--since", "soon"}, 0, false, true},
		{"non-positive", []string{"--since", "0h"}, 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dur, ok, err := sinceFrom(tt.args)
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

// The reindex feed deliberately includes closed jobs: open ones are upserted as
// documents, closed ones become deletions so they leave the index (job-search
// spec: the index contains only open jobs).
func TestSplitJobs_OpenBecomeDocsClosedBecomeDeletions(t *testing.T) {
	open := db.Job{ID: 1, Title: "Open", PublicSlug: "open-x"}
	closed := db.Job{ID: 2, Title: "Closed", PublicSlug: "closed-x",
		ClosedAt: pgtype.Timestamptz{Time: open.CreatedAt.Time, Valid: true}}

	docs, deleteIDs, err := splitJobs([]db.Job{open, closed})
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

	fetch := func(_ context.Context, afterID int64) ([]db.Job, error) {
		if afterID == 0 {
			return page, nil
		}
		return nil, nil
	}

	f := &fakeRebuilder{}
	indexed, err := reindexFull(context.Background(), fetch, f)
	if err != nil {
		t.Fatalf("reindexFull: %v", err)
	}
	if indexed != 2 {
		t.Errorf("indexed = %d, want 2 (open jobs only)", indexed)
	}
	wantCalls := []string{"prepare", "push", "promote"}
	if !reflect.DeepEqual(f.calls, wantCalls) {
		t.Errorf("calls = %v, want %v", f.calls, wantCalls)
	}
	if len(f.pushed) != 1 || !reflect.DeepEqual(f.pushed[0], []int64{1, 3}) {
		t.Errorf("pushed = %v, want [[1 3]] (closed job 2 skipped)", f.pushed)
	}
}
