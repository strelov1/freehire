package main

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
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
