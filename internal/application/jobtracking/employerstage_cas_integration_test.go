//go:build integration

// The compare-and-set behind EmployerStages.Mirror, against Postgres: a write expecting
// the wrong author is refused even when the stage matches — the case of a candidate who
// changed the stage away and back between the employer's read and write.
// Run with: go test -tags=integration ./internal/application/jobtracking/
package jobtracking

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestMirrorEmployerStage_ComparesTheAuthorToo(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	repo := employerStageQueries{q: q}

	var userID, jobID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('c@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
		VALUES ('employer', 'freehire:cas', '', 'Platform Engineer', 'Acme', 'acme', 'acme-cas') RETURNING id`).Scan(&jobID); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	svc := New(NewQueriesRepository(q, pool))
	if _, err := svc.MarkApplied(ctx, userID, "acme-cas", appevent.SourceUser); err != nil {
		t.Fatalf("MarkApplied: %v", err)
	}
	// The candidate records a rejection themselves.
	rejected := "rejected"
	if _, err := svc.Track(ctx, userID, "acme-cas", &rejected, nil, appevent.SourceUser); err != nil {
		t.Fatalf("Track: %v", err)
	}
	stage, source, found, err := repo.CandidateStage(ctx, userID, jobID)
	if err != nil || !found || stage != "rejected" || source != appevent.SourceUser {
		t.Fatalf("CandidateStage = %q, %q, %v, %v", stage, source, found, err)
	}
	// An employer write that read the stage as its own is refused: the author differs.
	if moved, err := repo.SetIfUnchanged(ctx, userID, jobID, "rejected", appevent.SourceEmployer, "interview"); err != nil || moved {
		t.Fatalf("SetIfUnchanged with a stale author = %v, %v; want refused", moved, err)
	}
	// With the author it actually has, the same write goes through.
	if moved, err := repo.SetIfUnchanged(ctx, userID, jobID, "rejected", appevent.SourceUser, "interview"); err != nil || !moved {
		t.Fatalf("SetIfUnchanged with the current author = %v, %v; want moved", moved, err)
	}
}
