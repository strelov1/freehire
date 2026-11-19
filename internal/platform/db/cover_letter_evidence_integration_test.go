//go:build integration

package db

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCoverLetterEvidenceOwnershipAndConcurrentRevisions(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)
	user := seedAPIKeyUser(t, pool, "evidence@example.test")
	other := seedAPIKeyUser(t, pool, "other-evidence@example.test")
	job := insertJob(t, pool, "evidence")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := q.SaveCoverLetterEvidence(ctx, SaveCoverLetterEvidenceParams{UserID: user, JobID: job, ExpectedRevision: 0, AtomIds: []pgtype.UUID{}})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success := 0
	conflicts := 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, pgx.ErrNoRows):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("first saves: %d successes, %d conflicts", success, conflicts)
	}
	row, err := q.GetCoverLetterEvidence(ctx, GetCoverLetterEvidenceParams{UserID: user, JobID: job})
	if err != nil || row.Revision != 1 || row.AtomIds == nil || len(row.AtomIds) != 0 {
		t.Fatalf("empty selection: %v %v", row, err)
	}
	if _, err = q.GetCoverLetterEvidence(ctx, GetCoverLetterEvidenceParams{UserID: other, JobID: job}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign read: %v", err)
	}
	args := UpdateCoverLetterEvidenceParams{UserID: user, JobID: job, ExpectedRevision: 1, AtomIds: []pgtype.UUID{}}
	if _, err = q.UpdateCoverLetterEvidence(ctx, args); err != nil {
		t.Fatal(err)
	}
	if _, err = q.UpdateCoverLetterEvidence(ctx, args); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale update: %v", err)
	}
}
