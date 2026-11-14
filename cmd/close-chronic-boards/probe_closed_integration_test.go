//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

func TestBoardSafetyNetsPreserveProbeClosedPostings(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()
	for _, kind := range []string{"chronic", "empty-feed", "board-gone"} {
		t.Run(kind, func(t *testing.T) {
			truncate(t, pool)
			seedProbeClosedBoard(t, pool, q, kind)
			params := ingestParams(externalid.Namespace("engineer", "123"), "Software Engineer")
			params.Source = "linkedin"
			row, err := q.UpsertJob(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			for _, apply := range []bool{false, true} {
				var report closeReport
				switch kind {
				case "chronic":
					report, err = closeChronicBoards(ctx, q, 60, 100, apply)
				case "empty-feed":
					report, err = closeEmptyFeedBoards(ctx, q, 30, 100, apply)
				case "board-gone":
					report, err = closeBoardGoneBoards(ctx, q, 7, 100, apply)
				}
				if err != nil {
					t.Fatal(err)
				}
				if report.boardsProcessed != 1 || report.jobsAffected != 0 {
					t.Fatalf("apply=%v: probe-only board reported closures: %+v", apply, report)
				}
			}
			job, err := q.GetJob(ctx, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			if job.ClosedAt.Valid {
				t.Fatalf("posting closed without posting evidence: %s", job.ClosedReason)
			}
		})
	}
}

func seedProbeClosedBoard(t *testing.T, pool *pgxpool.Pool, q *db.Queries, kind string) {
	t.Helper()
	switch kind {
	case "chronic":
		seedChronicBoard(t, pool, "linkedin", "engineer", 70)
	case "empty-feed":
		seedEmptyFeedBoard(t, pool, "linkedin", "engineer", 40)
	case "board-gone":
		seedBoardGoneBoard(t, pool, q, "linkedin", "engineer", "")
		seedBoardGoneSibling(t, q, "linkedin")
	}
}
