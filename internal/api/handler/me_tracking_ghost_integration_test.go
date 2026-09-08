//go:build integration

// Integration coverage for attachGhostToTrackedCards: it needs a real Postgres
// (ListJobGhostStamps, ghostEvidenceFor) alongside a fake searcher for the reality-class
// half. Run with: go test -tags=integration -run TestAttachGhostToTrackedCards ./internal/api/handler/
package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

func seedGhostJob(t *testing.T, pool *pgxpool.Pool, ext string, atsAbsentAt *time.Time) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, ats_absent_at)
		 VALUES ('test', $1, 'http://example.test', 'Job '||$1, $1, $2)
		 RETURNING id`, ext, atsAbsentAt).Scan(&id); err != nil {
		t.Fatalf("seed job %q: %v", ext, err)
	}
	return id
}

func TestAttachGhostToTrackedCards(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	now := time.Now()

	// ats_absent alone is one criterion — not enough to converge (ghost.convergence is
	// 2) without a witnessed contributor, so it stays LevelNone by itself. Paired with an
	// evergreen reality class from the fake searcher, two criteria converge to
	// LevelPossible with no application/report evidence needed.
	ghostJobID := seedGhostJob(t, pool, "ghost", &now)
	// A job with no ats_absent stamp and no evergreen reality has nothing to say.
	quietJobID := seedGhostJob(t, pool, "quiet", nil)

	tracked := func() []jobtracking.TrackedJob {
		return []jobtracking.TrackedJob{
			{Interaction: jobtracking.Interaction{JobID: ghostJobID}, Job: &jobview.Card{}},
			{Interaction: jobtracking.Interaction{JobID: quietJobID}, Job: &jobview.Card{}},
			// An orphaned application: no posting left to attach anything to.
			{Interaction: jobtracking.Interaction{JobID: 999999999}, Job: nil},
		}
	}

	t.Run("converged criteria produce a ghost signal", func(t *testing.T) {
		items := tracked()
		fake := &fakeSearcher{res: search.SearchResult{Hits: []search.JobDocument{
			{ID: ghostJobID, Job: jobview.Job{Reality: &jobview.Reality{Class: "likely-evergreen"}}},
		}}}
		h := &trackingHandlers{search: fake, queries: queries}

		h.attachGhostToTrackedCards(ctx, items)

		if items[0].Job.Ghost == nil {
			t.Fatal("ghost job: Ghost = nil, want a signal from ats_absent + evergreen reality converging")
		}
		if items[1].Job.Ghost != nil {
			t.Errorf("quiet job: Ghost = %+v, want nil — nothing fired for it", items[1].Job.Ghost)
		}
		want1 := search.In("id", []int64{ghostJobID, quietJobID})
		want2 := search.In("id", []int64{quietJobID, ghostJobID})
		if got := fake.got.Filter; got != want1 && got != want2 {
			t.Errorf("Search filter = %v, want an id-in filter over exactly the two real jobs", got)
		}
	})

	t.Run("a search error degrades rather than failing", func(t *testing.T) {
		items := tracked()
		fake := &fakeSearcher{err: errors.New("meilisearch unavailable")}
		h := &trackingHandlers{search: fake, queries: queries}

		h.attachGhostToTrackedCards(ctx, items)

		if items[0].Job.Ghost != nil {
			t.Errorf("ghost job: Ghost = %+v, want nil — ats_absent alone never converges, and the "+
				"evergreen half is unreachable once Search fails", items[0].Job.Ghost)
		}
	})

	// Reality is not the only route to a ghost signal: two non-reality criteria (here
	// ats_absent + user_reports) converge on their own, so a Search failure that makes the
	// reality criterion unreachable does not have to suppress the signal — it just narrows
	// which criteria could have contributed. This is the "evidence-only convergence" case
	// the plain "a search error degrades rather than failing" test above does not cover
	// (there, only one non-reality criterion was seeded, so it correctly stayed nil).
	t.Run("evidence-only criteria still converge when Search fails", func(t *testing.T) {
		var reporterID int64
		if err := pool.QueryRow(context.Background(),
			`INSERT INTO users (email) VALUES ('ghost-reporter@example.test') RETURNING id`).Scan(&reporterID); err != nil {
			t.Fatalf("seed reporter: %v", err)
		}
		reportedJobID := seedGhostJob(t, pool, "ghost-reported", &now)
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO ghost_reports (user_id, job_id, applied_on) VALUES ($1, $2, $3)`,
			reporterID, reportedJobID, now.AddDate(0, 0, -30)); err != nil {
			t.Fatalf("seed ghost report: %v", err)
		}

		items := []jobtracking.TrackedJob{
			{Interaction: jobtracking.Interaction{JobID: reportedJobID}, Job: &jobview.Card{}},
		}
		fake := &fakeSearcher{err: errors.New("meilisearch unavailable")}
		h := &trackingHandlers{search: fake, queries: queries}

		h.attachGhostToTrackedCards(ctx, items)

		if items[0].Job.Ghost == nil {
			t.Fatal("Ghost = nil, want a signal from ats_absent + user_reports converging without reality")
		}
	})

	t.Run("nothing panics when every card is orphaned", func(t *testing.T) {
		items := []jobtracking.TrackedJob{{Interaction: jobtracking.Interaction{JobID: 123}, Job: nil}}
		fake := &fakeSearcher{}
		h := &trackingHandlers{search: fake, queries: queries}

		h.attachGhostToTrackedCards(ctx, items)

		if fake.got.Filter != nil {
			t.Errorf("Search called with %v, want no call — nothing to attach", fake.got.Filter)
		}
	})
}
