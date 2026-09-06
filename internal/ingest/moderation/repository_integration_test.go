//go:build integration

// Integration tests for the moderator write path's queue side effects: a created or edited
// vacancy is queued for the live facet index atomically with its row, the same
// transactional-outbox property cmd/ingest has. Needs a real Postgres — the outbox and the
// slug minting live in SQL.
// Run with: go test -tags=integration ./internal/ingest/moderation/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package moderation_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// liveService returns the service on the real adapter, plus the moderator id the writes
// are stamped with — jobs.created_by is a foreign key, so the actor has to be a real row.
func liveService(t *testing.T) (*moderation.Service, *pgxpool.Pool, int64) {
	t.Helper()
	pool := testdb.Pool(t)
	var actorID int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ('moderator@example.test') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatalf("insert moderator: %v", err)
	}
	return moderation.New(moderation.NewQueriesRepository(db.New(pool), pool, 1)), pool, actorID
}

func queuedForSearch(t *testing.T, pool *pgxpool.Pool, jobID int64) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM search_outbox WHERE job_id = $1`, jobID).Scan(&n); err != nil {
		t.Fatalf("count search_outbox: %v", err)
	}
	return n > 0
}

// Every other write path queues the row it wrote: cmd/ingest, cmd/hydrate-adzuna-description,
// and linkimport (which pushes inline instead, for the same reason). The moderator path did
// not, so a hand-curated vacancy stayed out of /jobs/search, the facet counts, and the
// company's job count until the next full rebuild-and-swap — hours, on a 12h timer.
//
// The archived design justified the gap with parity ("manual jobs reach search via
// make reindex, as ingest"); that premise expired when ingest moved onto search_outbox.
func TestCreateQueuesTheJobForTheLiveIndex(t *testing.T) {
	svc, pool, actorID := liveService(t)

	j, _, err := svc.Create(context.Background(), actorID, moderation.CreateInput{
		URL:         "https://example.test/jobs/moderated-1",
		Title:       "Backend Engineer",
		Company:     "Acme",
		Location:    "Berlin, Germany",
		Description: "We are hiring a backend engineer to work on Go services.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !queuedForSearch(t, pool, j.Fields().ID) {
		t.Errorf("job %d is not in search_outbox after Create", j.Fields().ID)
	}
}

// An edit changes exactly what search shows — the title, the description, the derived
// facets — so it has to queue too. The create's own entry is cleared first: EnqueueSearchOutbox
// keeps one live entry per job (ON CONFLICT), so a leftover row would pass this test whether
// or not Update queues anything.
func TestUpdateQueuesTheEditedJobForTheLiveIndex(t *testing.T) {
	svc, pool, actorID := liveService(t)
	ctx := context.Background()

	created, _, err := svc.Create(ctx, actorID, moderation.CreateInput{
		URL:         "https://example.test/jobs/moderated-2",
		Title:       "Backend Engineer",
		Company:     "Acme",
		Description: "We are hiring a backend engineer to work on Go services.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM search_outbox WHERE job_id = $1`, created.Fields().ID); err != nil {
		t.Fatalf("clear the create's entry: %v", err)
	}

	title := "Senior Backend Engineer"
	edited, _, err := svc.Update(ctx, actorID, created.Fields().PublicSlug, moderation.UpdatePatch{Title: &title})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !queuedForSearch(t, pool, edited.Fields().ID) {
		t.Errorf("job %d is not in search_outbox after Update", edited.Fields().ID)
	}
}
