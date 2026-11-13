//go:build integration

// Release A of the job_activity split: every observation READER and every closer decides on
// job_activity, never on the legacy jobs columns, and a closer re-checks freshness on the LATEST
// version of the activity row it locked. Writers still write the legacy columns in release A (a
// one-way bridge mirrors them), so these tests freshen or age the ACTIVITY row directly: that is
// the state a release-B writer leaves behind, and the only state in which "reads activity" and
// "reads the legacy column" give different answers.
//
// The legacy column is deliberately set to the OPPOSITE of the activity value in every test. The
// bridge takes GREATEST of the two timestamps, so `UPDATE jobs SET last_seen_at = <older>` does
// not lower the activity row; ageing must go through job_activity.
//
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/externalid"
)

// obsAge makes a job look stale to a reader of job_activity while its LEGACY jobs.last_seen_at
// still says "just seen", so a statement that still reads the old column answers wrongly.
func obsAge(t *testing.T, pool *pgxpool.Pool, id int64, ago string) {
	t.Helper()
	ctx := context.Background()
	// Legacy first: the bridge moves job_activity FORWARD to GREATEST(activity, legacy), so a
	// legacy write made after ageing the activity row would undo it.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET last_seen_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("freshen legacy: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - $2::interval WHERE job_id = $1`, id, ago); err != nil {
		t.Fatalf("age activity: %v", err)
	}
}

// obsFresh is the mirror image: activity fresh, legacy column stale.
func obsFresh(t *testing.T, pool *pgxpool.Pool, id int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() WHERE job_id = $1`, id); err != nil {
		t.Fatalf("freshen activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET last_seen_at = now() - interval '10 days' WHERE id = $1`, id); err != nil {
		t.Fatalf("age legacy: %v", err)
	}
}

// obsBothStale ages the activity row and the legacy column, the state in which a closer that
// still reads the legacy column closes immediately instead of waiting for a lock.
func obsBothStale(t *testing.T, pool *pgxpool.Pool, id int64) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - interval '10 days' WHERE job_id = $1`, id); err != nil {
		t.Fatalf("age activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET last_seen_at = now() - interval '10 days', created_at = now() - interval '60 days' WHERE id = $1`, id); err != nil {
		t.Fatalf("age legacy: %v", err)
	}
}

func obsClosedExternalIDs(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT external_id FROM jobs WHERE closed_at IS NOT NULL ORDER BY external_id`)
	if err != nil {
		t.Fatalf("list closed: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func obsReopenAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE jobs SET closed_at = NULL, closed_reason = ''`); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `TRUNCATE search_delete_outbox`); err != nil {
		t.Fatalf("clear delete outbox: %v", err)
	}
}

// obsWaitBlocked returns once the backend pid waits on a lock, and fails if the statement it
// runs finishes first: a closer that never waited did not lock the activity row at all.
func obsWaitBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid uint32, done <-chan error) {
	t.Helper()
	for {
		var blocked bool
		if err := pool.QueryRow(ctx,
			`SELECT COALESCE(wait_event_type = 'Lock', false) FROM pg_stat_activity WHERE pid = $1`, int32(pid)).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("statement finished (err=%v) without waiting for the freshening transaction: it does not lock job_activity", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// obsSeedPair stores one job that is stale and one that is fresh, from the point of view of
// job_activity, both under the same source, company and board so every closer's scope matches
// both and only the freshness decides.
func obsSeedPair(t *testing.T, pool *pgxpool.Pool) (stale, fresh int64) {
	t.Helper()
	ctx := context.Background()
	q := New(pool)
	truncate(t, pool)
	s, err := ingestUpsert(ctx, q, ingestParams("acme:stale", "Stale"))
	if err != nil {
		t.Fatalf("upsert stale: %v", err)
	}
	f, err := ingestUpsert(ctx, q, ingestParams("acme:fresh", "Fresh"))
	if err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}
	// Old enough for the age clock of CloseStaleUnseenUnprobeableJobs, which asks BOTH how old a
	// posting is and how long since it was seen.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET created_at = now() - interval '60 days'`); err != nil {
		t.Fatalf("age created_at: %v", err)
	}
	obsAge(t, pool, s.ID, "10 days")
	obsFresh(t, pool, f.ID)
	return s.ID, f.ID
}

func TestClosersDecideOnActivityNotTheLegacyColumn(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	obsSeedPair(t, pool)

	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}
	pattern := externalid.BoardPattern("acme")
	closers := []struct {
		name string
		run  func() error
	}{
		{"CloseUnseenJobs", func() error {
			_, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{Source: "greenhouse", Cutoff: cutoff, CompanySlugs: []string{"acme"}})
			return err
		}},
		{"CloseUnseenJobsForBoard", func() error {
			_, err := q.CloseUnseenJobsForBoard(ctx, CloseUnseenJobsForBoardParams{Source: "greenhouse", Cutoff: cutoff, BoardPattern: pattern})
			return err
		}},
		{"CloseUnseenJobsForBoards", func() error {
			_, err := q.CloseUnseenJobsForBoards(ctx, CloseUnseenJobsForBoardsParams{Source: "greenhouse", Cutoff: cutoff, BoardPatterns: []string{pattern}})
			return err
		}},
		{"CloseUnseenJobsBySource", func() error {
			_, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{Source: "greenhouse", Cutoff: cutoff})
			return err
		}},
		{"CloseStaleUnseenUnprobeableJobs", func() error {
			_, err := q.CloseStaleUnseenUnprobeableJobs(ctx, CloseStaleUnseenUnprobeableJobsParams{
				Sources: []string{"greenhouse"}, Cutoff: pgtype.Timestamptz{Time: time.Now().Add(-45 * 24 * time.Hour), Valid: true}, SeenCutoff: cutoff,
			})
			return err
		}},
	}
	for _, c := range closers {
		t.Run(c.name, func(t *testing.T) {
			obsReopenAll(t, pool)
			if err := c.run(); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			got := obsClosedExternalIDs(t, pool)
			if !slices.Equal(got, []string{"acme:stale"}) {
				t.Fatalf("closed = %v, want only acme:stale (stale in job_activity); the legacy column says the opposite", got)
			}
		})
	}
}

func TestUnseenCandidateScansReadActivity(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	stale, _ := obsSeedPair(t, pool)
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}

	ids, err := q.UnseenJobIDs(ctx, UnseenJobIDsParams{Source: "greenhouse", Cutoff: cutoff, CompanySlugs: []string{"acme"}})
	if err != nil || !slices.Equal(ids, []int64{stale}) {
		t.Fatalf("UnseenJobIDs = %v, %v; want [%d]", ids, err, stale)
	}
	ids, err = q.UnseenJobIDsBySource(ctx, UnseenJobIDsBySourceParams{Source: "greenhouse", Cutoff: cutoff})
	if err != nil || !slices.Equal(ids, []int64{stale}) {
		t.Fatalf("UnseenJobIDsBySource = %v, %v; want [%d]", ids, err, stale)
	}
	rows, err := q.SelectStaleRegisteredCandidates(ctx, SelectStaleRegisteredCandidatesParams{Sources: []string{"greenhouse"}, Cutoff: cutoff})
	if err != nil || len(rows) != 1 || rows[0].ID != stale {
		t.Fatalf("SelectStaleRegisteredCandidates = %+v, %v; want only %d", rows, err, stale)
	}
	scrape, err := q.SelectScraplingLivenessCandidates(ctx, SelectScraplingLivenessCandidatesParams{Source: "greenhouse", Cutoff: cutoff, MaxRows: 10})
	if err != nil || len(scrape) != 1 || scrape[0].ID != stale {
		t.Fatalf("SelectScraplingLivenessCandidates = %+v, %v; want only %d", scrape, err, stale)
	}
}

// The strike count the probe worker carries into its verdict is the activity row's.
func TestLivenessCandidatesCarryActivityStrikes(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	stale, _ := obsSeedPair(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 3 WHERE job_id = $1`, stale); err != nil {
		t.Fatal(err)
	}
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}

	rows, err := q.SelectStaleRegisteredCandidates(ctx, SelectStaleRegisteredCandidatesParams{Sources: []string{"greenhouse"}, Cutoff: cutoff})
	if err != nil || len(rows) != 1 || rows[0].LivenessStrikes != 3 {
		t.Fatalf("stale candidates = %+v, %v; want one with 3 strikes", rows, err)
	}
	orphans, err := q.SelectOrphanLivenessCandidates(ctx, []string{"lever"})
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, o := range orphans {
		if o.ID == stale {
			seen = true
			if o.LivenessStrikes != 3 {
				t.Fatalf("orphan candidate strikes = %d, want 3", o.LivenessStrikes)
			}
		}
	}
	if !seen {
		t.Fatalf("orphan candidates %+v lack the job", orphans)
	}
}

// The probe's compare-and-set must test the activity row: a crawl (release B) that freshened
// ONLY job_activity has just proved the posting live.
func TestMarkLivenessExpiredCASReadsActivity(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	_, fresh := obsSeedPair(t, pool)
	seenBefore := pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}

	_, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: fresh, Threshold: 1, SeenBefore: seenBefore})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("probe on a posting fresh in job_activity: err = %v, want pgx.ErrNoRows", err)
	}
}

// Fresh observation wins, probe side: the crawl's activity write is uncommitted when the probe
// reaches its verdict. The probe must wait for it, re-check the latest activity version and
// write nothing.
func TestMarkLivenessExpiredWaitsForFreshObservationAndWritesNothing(t *testing.T) {
	pool := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stale, _ := obsSeedPair(t, pool)
	obsBothStale(t, pool, stale)

	crawl, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = crawl.Rollback(context.Background()) }()
	if _, err := crawl.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() WHERE job_id = $1`, stale); err != nil {
		t.Fatal(err)
	}

	probe, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Release()
	done := make(chan error, 1)
	go func() {
		_, err := New(probe).MarkLivenessExpired(ctx, MarkLivenessExpiredParams{
			ID: stale, Threshold: 1, SeenBefore: pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true},
		})
		done <- err
	}()
	obsWaitBlocked(t, ctx, pool, probe.Conn().PgConn().PID(), done)
	if err := crawl.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("probe after a committed fresh observation: err = %v, want pgx.ErrNoRows", err)
	}
	var strikes int32
	var closed bool
	var queued int
	if err := pool.QueryRow(ctx, `SELECT a.liveness_strikes, j.closed_at IS NOT NULL, (SELECT count(*) FROM search_delete_outbox)
		FROM jobs j JOIN job_activity a ON a.job_id = j.id WHERE j.id = $1`, stale).Scan(&strikes, &closed, &queued); err != nil {
		t.Fatal(err)
	}
	if strikes != 0 || closed || queued != 0 {
		t.Fatalf("probe wrote after losing the race: strikes=%d closed=%t queued=%d", strikes, closed, queued)
	}
}

// Fresh observation wins, sweep side, for the closers that take a candidate list.
func TestClosersWaitForFreshObservationAndSkipTheRow(t *testing.T) {
	cutoff := func() pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}
	}
	pattern := externalid.BoardPattern("acme")
	cases := []struct {
		name string
		run  func(ctx context.Context, q *Queries) error
	}{
		{"CloseUnseenJobs", func(ctx context.Context, q *Queries) error {
			_, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{Source: "greenhouse", Cutoff: cutoff(), CompanySlugs: []string{"acme"}})
			return err
		}},
		{"CloseUnseenJobsForBoards", func(ctx context.Context, q *Queries) error {
			_, err := q.CloseUnseenJobsForBoards(ctx, CloseUnseenJobsForBoardsParams{Source: "greenhouse", Cutoff: cutoff(), BoardPatterns: []string{pattern}})
			return err
		}},
		{"CloseUnseenJobsBySource", func(ctx context.Context, q *Queries) error {
			_, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{Source: "greenhouse", Cutoff: cutoff()})
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := startPostgres(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			stale, _ := obsSeedPair(t, pool)
			obsBothStale(t, pool, stale)

			crawl, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = crawl.Rollback(context.Background()) }()
			if _, err := crawl.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() WHERE job_id = $1`, stale); err != nil {
				t.Fatal(err)
			}
			sweep, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer sweep.Release()
			done := make(chan error, 1)
			go func() { done <- c.run(ctx, New(sweep)) }()
			obsWaitBlocked(t, ctx, pool, sweep.Conn().PgConn().PID(), done)
			if err := crawl.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got := obsClosedExternalIDs(t, pool); len(got) != 0 {
				t.Fatalf("closed = %v after a fresh observation committed first, want nothing closed", got)
			}
			var queued int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox`).Scan(&queued); err != nil || queued != 0 {
				t.Fatalf("delete outbox = %d, %v; want 0", queued, err)
			}
		})
	}
}

// Lock order jobs, then job_activity, with NO KEY UPDATE on jobs. A cheap toucher holds the
// activity row, then inserts a child row whose foreign key takes KEY SHARE on the jobs tuple
// (EnqueueJobEnrichment, UpsertApplyForm, search_outbox). A closer that already holds the jobs
// tuple and waits for the activity row must not conflict with that KEY SHARE: FOR UPDATE would
// deadlock here (SQLSTATE 40P01), FOR NO KEY UPDATE does not.
func TestCloserWaitingForActivityDoesNotDeadlockAgainstAChildInsert(t *testing.T) {
	pool := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stale, _ := obsSeedPair(t, pool)
	obsBothStale(t, pool, stale)

	toucher, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = toucher.Rollback(context.Background()) }()
	if _, err := toucher.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() WHERE job_id = $1`, stale); err != nil {
		t.Fatal(err)
	}
	closer, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Release()
	done := make(chan error, 1)
	go func() {
		_, err := New(closer).CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{
			Source: "greenhouse", Cutoff: pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true},
		})
		done <- err
	}()
	obsWaitBlocked(t, ctx, pool, closer.Conn().PgConn().PID(), done)

	// The toucher now needs KEY SHARE on the jobs tuple the closer holds.
	if _, err := toucher.Exec(ctx, `INSERT INTO search_outbox (job_id, job_posted_at) VALUES ($1, now()) ON CONFLICT (job_id) DO NOTHING`, stale); err != nil {
		t.Fatalf("child insert while a closer holds the jobs tuple: %v", err)
	}
	if err := toucher.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("closer: %v", err)
	}
	if got := obsClosedExternalIDs(t, pool); len(got) != 0 {
		t.Fatalf("closed = %v, want nothing closed", got)
	}
}

// TouchJob used to lock the jobs tuple FOR UPDATE, which conflicts with the KEY SHARE every
// foreign-key child insert takes; with a toucher now holding other locks in the same
// transaction that is a deadlock waiting to happen. A plain UPDATE's lock (NO KEY UPDATE) is
// what it must take.
func TestTouchJobIsNotBlockedByAKeyShareHolder(t *testing.T) {
	pool := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, fresh := obsSeedPair(t, pool)

	child, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Rollback(context.Background()) }()
	if _, err := child.Exec(ctx, `SELECT 1 FROM jobs WHERE id = $1 FOR KEY SHARE`, fresh); err != nil {
		t.Fatal(err)
	}

	touch, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer touch.Release()
	done := make(chan error, 1)
	go func() {
		_, err := New(touch).TouchJob(ctx, TouchJobParams{
			Source: "greenhouse", ExternalID: "acme:fresh",
			WithheldIfSeenBefore: pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true},
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("TouchJob: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TouchJob is blocked by a KEY SHARE holder: it must lock the jobs tuple FOR NO KEY UPDATE, not FOR UPDATE")
	}
}

// The withheld-then-seen requeue compares the activity row's previous stamp, not the legacy
// column's.
func TestTouchJobRequeuesAPostingWhoseActivityWasWithheld(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	stale, _ := obsSeedPair(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - interval '20 days' WHERE job_id = $1`, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE search_outbox`); err != nil {
		t.Fatal(err)
	}
	if _, err := q.TouchJob(ctx, TouchJobParams{
		Source: "greenhouse", ExternalID: "acme:stale",
		WithheldIfSeenBefore: pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	var queued bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM search_outbox WHERE job_id = $1)`, stale).Scan(&queued); err != nil || !queued {
		t.Fatalf("queued = %t, %v; want a posting last seen 20 days ago (activity) requeued when seen again", queued, err)
	}
}

// hydrated_at in the per-board and per-provider seen-set is the activity row's.
func TestSeenSetReadsActivityHydratedAt(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)
	staleBody, err := ingestUpsert(ctx, q, ingestParams("acme:stale-body", "Stale body"))
	if err != nil {
		t.Fatal(err)
	}
	freshBody, err := ingestUpsert(ctx, q, ingestParams("acme:fresh-body", "Fresh body"))
	if err != nil {
		t.Fatal(err)
	}
	// The legacy column says the opposite of job_activity. Legacy first for the row that must
	// read stale: the bridge raises job_activity to GREATEST(activity, legacy), and never lowers it.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET hydrated_at = now() WHERE id = $1`, staleBody.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET hydrated_at = now() - interval '90 days' WHERE job_id = $1`, staleBody.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET hydrated_at = now() WHERE job_id = $1`, freshBody.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET hydrated_at = now() - interval '90 days' WHERE id = $1`, freshBody.ID); err != nil {
		t.Fatal(err)
	}

	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-24 * time.Hour), Valid: true}
	old := pgtype.Timestamptz{Time: time.Now().Add(-365 * 24 * time.Hour), Valid: true}
	deadlineFrom := pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, -7), Valid: true}
	all, err := q.ExistingExternalIDs(ctx, ExistingExternalIDsParams{
		Source: "greenhouse", HydrationCutoff: old, BodyRefreshCutoff: cutoff, BodyRefreshSlices: 1, BodyRefreshSlot: 0, DeadlineRecheckFrom: deadlineFrom,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ExternalID != "acme:fresh-body" {
		t.Fatalf("ExistingExternalIDs = %+v, want only acme:fresh-body (stale body is withheld so the crawl re-reads it)", all)
	}
	byBoard, err := q.ExistingExternalIDsByBoard(ctx, ExistingExternalIDsByBoardParams{
		Source: "greenhouse", Pattern: externalid.BoardPattern("acme"), HydrationCutoff: old, BodyRefreshCutoff: cutoff,
		BodyRefreshSlices: 1, BodyRefreshSlot: 0, DeadlineRecheckFrom: deadlineFrom,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(byBoard) != 1 || byBoard[0].ExternalID != "acme:fresh-body" {
		t.Fatalf("ExistingExternalIDsByBoard = %+v, want only acme:fresh-body", byBoard)
	}
}

// The predicate readers that count or gate on "seen recently". One posting: its legacy column
// always says "just seen", and only job_activity says otherwise.
func TestObservationPredicateReadersUseActivity(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)
	job, err := ingestUpsert(ctx, q, ingestParams("acme:1", "Engineer"))
	if err != nil {
		t.Fatal(err)
	}
	coverage := func() ([]string, error) {
		return q.CompaniesWithFreshNonAggregatorCoverage(ctx, CompaniesWithFreshNonAggregatorCoverageParams{
			FoldedCompanies: []string{"acme"},
			SeenAfter:       pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true},
			Aggregators:     []string{"adzuna"},
		})
	}
	if fresh, err := coverage(); err != nil || !slices.Equal(fresh, []string{"acme"}) {
		t.Fatalf("coverage with a fresh posting = %v, %v; want [acme]", fresh, err)
	}
	stats, err := q.AggregateOpenJobsBySource(ctx)
	if err != nil || len(stats) != 1 || stats[0].Unseen7dJobs != 0 {
		t.Fatalf("AggregateOpenJobsBySource = %+v, %v; want 0 unseen", stats, err)
	}

	obsAge(t, pool, job.ID, "10 days")

	if fresh, err := coverage(); err != nil || len(fresh) != 0 {
		t.Fatalf("coverage after job_activity went stale = %v, %v; want none (the legacy column still says fresh)", fresh, err)
	}
	stats, err = q.AggregateOpenJobsBySource(ctx)
	if err != nil || len(stats) != 1 || stats[0].Unseen7dJobs != 1 {
		t.Fatalf("AggregateOpenJobsBySource = %+v, %v; want unseen_7d_jobs = 1 (stale in job_activity)", stats, err)
	}
}

// Strike counts during the release A -> B overlap: job_activity holds the current count and the
// legacy column may be stale in either direction. The one-way bridge copies a legacy strike value
// only when it CHANGES, so every case below is one the bridge alone would get wrong.
func obsStrikes(t *testing.T, pool *pgxpool.Pool, id int64) (activity, legacy int32) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT a.liveness_strikes, j.liveness_strikes FROM jobs j JOIN job_activity a ON a.job_id = j.id WHERE j.id = $1`, id).Scan(&activity, &legacy); err != nil {
		t.Fatalf("read strikes: %v", err)
	}
	return activity, legacy
}

func obsSeedOne(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	truncate(t, pool)
	job, err := ingestUpsert(context.Background(), New(pool), ingestParams("acme:1", "Engineer"))
	if err != nil {
		t.Fatal(err)
	}
	return job.ID
}

func TestResetLivenessStrikesClearsActivityWhateverTheLegacyColumnSays(t *testing.T) {
	for _, tc := range []struct{ name, legacySQL string }{
		{"legacy already zero, activity four", `UPDATE job_activity SET liveness_strikes = 4 WHERE job_id = $1`},
		{"legacy four, activity four", `UPDATE jobs SET liveness_strikes = 4 WHERE id = $1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := startPostgres(t)
			id := obsSeedOne(t, pool)
			if _, err := pool.Exec(context.Background(), tc.legacySQL, id); err != nil {
				t.Fatal(err)
			}
			if a, _ := obsStrikes(t, pool, id); a != 4 {
				t.Fatalf("precondition: activity strikes = %d, want 4", a)
			}
			if err := New(pool).ResetLivenessStrikes(context.Background(), id); err != nil {
				t.Fatalf("reset: %v", err)
			}
			if a, l := obsStrikes(t, pool, id); a != 0 || l != 0 {
				t.Fatalf("after reset activity=%d legacy=%d, want 0/0", a, l)
			}
		})
	}
}

func TestMarkLivenessExpiredCountsFromActivityAndWritesBothPlaces(t *testing.T) {
	ctx := context.Background()
	t.Run("activity ahead of a frozen legacy column", func(t *testing.T) {
		pool := startPostgres(t)
		id := obsSeedOne(t, pool)
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 4 WHERE job_id = $1`, id); err != nil {
			t.Fatal(err)
		}
		res, err := New(pool).MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: id, Threshold: 6})
		if err != nil || res.LivenessStrikes != 5 || res.ClosedAt.Valid {
			t.Fatalf("mark = %+v, %v; want strike 5 and still open", res, err)
		}
		if a, l := obsStrikes(t, pool, id); a != 5 || l != 5 {
			t.Fatalf("activity=%d legacy=%d, want 5/5", a, l)
		}
		res, err = New(pool).MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: id, Threshold: 6})
		if err != nil || res.LivenessStrikes != 6 || !res.ClosedAt.Valid {
			t.Fatalf("mark = %+v, %v; want strike 6 and closed", res, err)
		}
		var queued int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox WHERE job_id = $1`, id).Scan(&queued); err != nil || queued != 1 {
			t.Fatalf("delete outbox = %d, %v; want 1", queued, err)
		}
	})
	t.Run("computed count equals the stale legacy value", func(t *testing.T) {
		pool := startPostgres(t)
		id := obsSeedOne(t, pool)
		// legacy 1, activity 0: the next strike is 1, which the bridge sees as no change.
		if _, err := pool.Exec(ctx, `UPDATE jobs SET liveness_strikes = 1 WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 0 WHERE job_id = $1`, id); err != nil {
			t.Fatal(err)
		}
		res, err := New(pool).MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: id, Threshold: 3})
		if err != nil || res.LivenessStrikes != 1 {
			t.Fatalf("mark = %+v, %v; want strike 1", res, err)
		}
		if a, l := obsStrikes(t, pool, id); a != 1 || l != 1 {
			t.Fatalf("activity=%d legacy=%d, want 1/1 (the bridge does not fire for 1 -> 1)", a, l)
		}
	})
	t.Run("computed count equals the stale legacy value and closes", func(t *testing.T) {
		// The activity write must land BEFORE the jobs write whose state trigger updates the same
		// activity row: legacy 1, activity 0, threshold 1 makes the bridge silent (1 -> 1) and the
		// close fires the state trigger in the same statement.
		pool := startPostgres(t)
		id := obsSeedOne(t, pool)
		if _, err := pool.Exec(ctx, `UPDATE jobs SET liveness_strikes = 1 WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 0 WHERE job_id = $1`, id); err != nil {
			t.Fatal(err)
		}
		res, err := New(pool).MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: id, Threshold: 1})
		if err != nil || res.LivenessStrikes != 1 || !res.ClosedAt.Valid {
			t.Fatalf("mark = %+v, %v; want strike 1 and closed", res, err)
		}
		if a, l := obsStrikes(t, pool, id); a != 1 || l != 1 {
			t.Fatalf("activity=%d legacy=%d, want 1/1", a, l)
		}
		var queued int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox WHERE job_id = $1`, id).Scan(&queued); err != nil || queued != 1 {
			t.Fatalf("delete outbox = %d, %v; want 1", queued, err)
		}
	})
	t.Run("closes on the activity count", func(t *testing.T) {
		pool := startPostgres(t)
		id := obsSeedOne(t, pool)
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 1 WHERE job_id = $1`, id); err != nil {
			t.Fatal(err)
		}
		res, err := New(pool).MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: id, Threshold: 2})
		if err != nil || res.LivenessStrikes != 2 || !res.ClosedAt.Valid {
			t.Fatalf("mark = %+v, %v; want strike 2 and closed (legacy column was 0)", res, err)
		}
	})
}

// A private posting is one user's pasted JD: no sweep closes it (the guard walks the closers too).
func TestClosersLeavePrivatePostingsAlone(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	stale, _ := obsSeedPair(t, pool)
	// The production shape of a private posting: its own source (privatejob.SourcePasted), never a
	// board provider's.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET is_private = true, source = 'pasted' WHERE id = $1`, stale); err != nil {
		t.Fatal(err)
	}
	n, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{
		Source: "pasted", Cutoff: pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true},
	})
	if err != nil || n != 0 {
		t.Fatalf("closed %d, %v; want 0 (the stale posting is private)", n, err)
	}
}

// ---- Release A through the REAL writers --------------------------------------------------------
// In release A the crawl writes the legacy jobs columns (RefreshUnchangedJob, UpsertJob, TouchJob);
// the one-way bridge mirrors them into job_activity at statement end. A closer therefore waits on
// the JOBS tuple first. These tests drive the real statements, not a hand UPDATE of job_activity.

func obsHashedParams() UpsertJobParams {
	p := ingestParams("acme:1", "Engineer")
	p.ContentHash = pgtype.Text{String: "h1", Valid: true}
	return p
}

func obsRefreshParams() RefreshUnchangedJobParams {
	return RefreshUnchangedJobParams{
		Source: "greenhouse", ExternalID: "acme:1", ContentHash: pgtype.Text{String: "h1", Valid: true},
		WithheldIfSeenBefore: pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true},
	}
}

// obsSeedStaleHashed stores one open posting that a sweep or probe would act on.
func obsSeedStaleHashed(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	truncate(t, pool)
	job, err := ingestUpsert(context.Background(), New(pool), obsHashedParams())
	if err != nil {
		t.Fatal(err)
	}
	obsBothStale(t, pool, job.ID)
	return job.ID
}

func obsCloseVerdicts() []struct {
	name string
	run  func(ctx context.Context, q *Queries, id int64) error
} {
	cutoff := func() pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}
	}
	return []struct {
		name string
		run  func(ctx context.Context, q *Queries, id int64) error
	}{
		{"CloseUnseenJobs", func(ctx context.Context, q *Queries, _ int64) error {
			_, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{Source: "greenhouse", Cutoff: cutoff(), CompanySlugs: []string{"acme"}})
			return err
		}},
		{"CloseUnseenJobsBySource", func(ctx context.Context, q *Queries, _ int64) error {
			_, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{Source: "greenhouse", Cutoff: cutoff()})
			return err
		}},
		{"MarkLivenessExpired", func(ctx context.Context, q *Queries, id int64) error {
			_, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: id, Threshold: 1, SeenBefore: cutoff()})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // the compare-and-set found a fresh row: nothing written
			}
			return err
		}},
	}
}

// (a) The crawl's REAL cheap write is in flight and uncommitted; the closer waits on the jobs
// tuple, then sees the row fresh and writes nothing.
func TestClosersWaitForARealLegacyRefreshAndWriteNothing(t *testing.T) {
	for _, c := range obsCloseVerdicts() {
		t.Run(c.name, func(t *testing.T) {
			pool := startPostgres(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			id := obsSeedStaleHashed(t, pool)

			crawl, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = crawl.Rollback(context.Background()) }()
			if _, err := New(crawl).RefreshUnchangedJob(ctx, obsRefreshParams()); err != nil {
				t.Fatalf("real cheap refresh: %v", err)
			}
			closer, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer closer.Release()
			done := make(chan error, 1)
			go func() { done <- c.run(ctx, New(closer), id) }()
			obsWaitBlocked(t, ctx, pool, closer.Conn().PgConn().PID(), done)
			if err := crawl.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			var closed bool
			var strikes int32
			var queued int
			if err := pool.QueryRow(ctx, `SELECT j.closed_at IS NOT NULL, a.liveness_strikes, (SELECT count(*) FROM search_delete_outbox)
				FROM jobs j JOIN job_activity a ON a.job_id = j.id WHERE j.id = $1`, id).Scan(&closed, &strikes, &queued); err != nil {
				t.Fatal(err)
			}
			if closed || strikes != 0 || queued != 0 {
				t.Fatalf("after a committed refresh: closed=%t strikes=%d queued=%d, want an untouched open row", closed, strikes, queued)
			}
			if age := obsSeenAge(t, pool, id); age > time.Minute {
				t.Fatalf("activity last_seen_at is %s old: the refresh did not reach job_activity", age)
			}
		})
	}
}

func obsSeenAge(t *testing.T, pool *pgxpool.Pool, id int64) time.Duration {
	t.Helper()
	var seen time.Time
	if err := pool.QueryRow(context.Background(), `SELECT last_seen_at FROM job_activity WHERE job_id = $1`, id).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	return time.Since(seen)
}

// (b) The closer commits first. The cheap write then matches no row (it never refreshes a closed
// row), and the full upsert reopens it and clears the strikes in both places.
func TestRealWritersReopenWhatACloserClosedFirst(t *testing.T) {
	for _, c := range obsCloseVerdicts() {
		t.Run(c.name, func(t *testing.T) {
			pool := startPostgres(t)
			ctx := context.Background()
			q := New(pool)
			id := obsSeedStaleHashed(t, pool)
			if err := c.run(ctx, q, id); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got := obsClosedExternalIDs(t, pool); !slices.Equal(got, []string{"acme:1"}) {
				t.Fatalf("closed = %v, want the stale posting closed", got)
			}
			if _, err := q.RefreshUnchangedJob(ctx, obsRefreshParams()); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("cheap refresh of a closed row: err = %v, want pgx.ErrNoRows", err)
			}
			if _, err := q.UpsertJob(ctx, obsHashedParams()); err != nil {
				t.Fatalf("full upsert: %v", err)
			}
			if got := obsClosedExternalIDs(t, pool); len(got) != 0 {
				t.Fatalf("closed = %v after the upsert, want it reopened", got)
			}
			if a, l := obsStrikes(t, pool, id); a != 0 || l != 0 {
				t.Fatalf("strikes after reopen: activity=%d legacy=%d, want 0/0", a, l)
			}
			if age := obsSeenAge(t, pool, id); age > time.Minute {
				t.Fatalf("activity last_seen_at is %s old after the reopening upsert", age)
			}
		})
	}
}

// A TouchJob that reopens a posting a closer is closing right now waits for the close, then reopens.
func TestTouchJobReopensWhatAConcurrentCloserClosed(t *testing.T) {
	pool := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	id := obsSeedStaleHashed(t, pool)

	closing, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closing.Rollback(context.Background()) }()
	if n, err := New(closing).CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{
		Source: "greenhouse", Cutoff: pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true},
	}); err != nil || n != 1 {
		t.Fatalf("close in transaction: %d, %v", n, err)
	}
	touch, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer touch.Release()
	done := make(chan error, 1)
	go func() {
		_, err := New(touch).TouchJob(ctx, TouchJobParams{
			Source: "greenhouse", ExternalID: "acme:1", AllowReopen: true,
			WithheldIfSeenBefore: pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true},
		})
		done <- err
	}()
	obsWaitBlocked(t, ctx, pool, touch.Conn().PgConn().PID(), done)
	if err := closing.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("touch: %v", err)
	}
	if got := obsClosedExternalIDs(t, pool); len(got) != 0 {
		t.Fatalf("closed = %v, want the posting reopened by the touch", got)
	}
	if a, l := obsStrikes(t, pool, id); a != 0 || l != 0 {
		t.Fatalf("strikes after reopen: activity=%d legacy=%d, want 0/0", a, l)
	}
}

// Strike rules through the real writers while job_activity is ahead of a frozen legacy column
// (the release A -> B overlap): an OPEN row keeps its strikes whichever writer re-sees it; a
// REOPENED row has them cleared whatever the legacy column says.
func TestRealWritersKeepActivityStrikesOnOpenRowsAndClearThemOnReopen(t *testing.T) {
	ctx := context.Background()
	reseen := []struct {
		name string
		run  func(q *Queries) error
	}{
		{"UpsertJob", func(q *Queries) error { _, err := q.UpsertJob(ctx, obsHashedParams()); return err }},
		{"RefreshUnchangedJob", func(q *Queries) error { _, err := q.RefreshUnchangedJob(ctx, obsRefreshParams()); return err }},
		{"TouchJob", func(q *Queries) error {
			_, err := q.TouchJob(ctx, TouchJobParams{
				Source: "greenhouse", ExternalID: "acme:1", AllowReopen: true,
				WithheldIfSeenBefore: pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true},
			})
			return err
		}},
	}
	for _, w := range reseen {
		t.Run(w.name+" keeps strikes on an open row", func(t *testing.T) {
			pool := startPostgres(t)
			id := obsSeedStaleHashed(t, pool)
			if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 4 WHERE job_id = $1`, id); err != nil {
				t.Fatal(err)
			}
			if err := w.run(New(pool)); err != nil {
				t.Fatalf("%s: %v", w.name, err)
			}
			if a, l := obsStrikes(t, pool, id); a != 4 || l != 0 {
				t.Fatalf("after %s on an open row: activity=%d legacy=%d, want 4/0 (the bridge copies only a CHANGED legacy value)", w.name, a, l)
			}
		})
		if w.name == "RefreshUnchangedJob" {
			continue // never refreshes a closed row; the reopen belongs to UpsertJob / TouchJob
		}
		t.Run(w.name+" clears strikes on reopen", func(t *testing.T) {
			pool := startPostgres(t)
			id := obsSeedStaleHashed(t, pool)
			if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 4 WHERE job_id = $1`, id); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now(), closed_reason = 'unseen' WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
			if err := w.run(New(pool)); err != nil {
				t.Fatalf("%s: %v", w.name, err)
			}
			if got := obsClosedExternalIDs(t, pool); len(got) != 0 {
				t.Fatalf("closed = %v, want reopened", got)
			}
			if a, l := obsStrikes(t, pool, id); a != 0 || l != 0 {
				t.Fatalf("after %s reopened the row: activity=%d legacy=%d, want 0/0", w.name, a, l)
			}
		})
	}
}

// The row-by-row sweep fallback closes one id at a time after listing its candidates. The close
// takes the crawl cutoff and re-checks the activity row's freshness under its locks: a posting a
// crawl listed again since the listing is not closed and queues no removal, whether the refresh
// committed before the close started or is still in flight when it does.
func TestCloseUnseenJobByIDRechecksFreshnessUnderItsLocks(t *testing.T) {
	cutoff := func() pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}
	}
	t.Run("stale row closes", func(t *testing.T) {
		pool := startPostgres(t)
		id := obsSeedStaleHashed(t, pool)
		n, err := New(pool).CloseUnseenJobByID(context.Background(), CloseUnseenJobByIDParams{ID: id, Cutoff: cutoff()})
		if err != nil || n != 1 {
			t.Fatalf("closed %d, %v; want 1", n, err)
		}
	})
	t.Run("refresh committed since the listing", func(t *testing.T) {
		pool := startPostgres(t)
		id := obsSeedStaleHashed(t, pool)
		if _, err := New(pool).RefreshUnchangedJob(context.Background(), obsRefreshParams()); err != nil {
			t.Fatal(err)
		}
		n, err := New(pool).CloseUnseenJobByID(context.Background(), CloseUnseenJobByIDParams{ID: id, Cutoff: cutoff()})
		if err != nil || n != 0 {
			t.Fatalf("closed %d, %v; want 0 for a fresh row", n, err)
		}
		if got := obsClosedExternalIDs(t, pool); len(got) != 0 {
			t.Fatalf("closed = %v, want nothing closed", got)
		}
		var queued int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM search_delete_outbox`).Scan(&queued); err != nil || queued != 0 {
			t.Fatalf("removals queued = %d, %v; want 0", queued, err)
		}
	})
	t.Run("refresh still in flight waits then writes nothing", func(t *testing.T) {
		pool := startPostgres(t)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		id := obsSeedStaleHashed(t, pool)
		crawl, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = crawl.Rollback(context.Background()) }()
		if _, err := New(crawl).RefreshUnchangedJob(ctx, obsRefreshParams()); err != nil {
			t.Fatal(err)
		}
		closer, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer closer.Release()
		done := make(chan error, 1)
		var n int64
		go func() {
			var err error
			n, err = New(closer).CloseUnseenJobByID(ctx, CloseUnseenJobByIDParams{ID: id, Cutoff: cutoff()})
			done <- err
		}()
		obsWaitBlocked(t, ctx, pool, closer.Conn().PgConn().PID(), done)
		if err := crawl.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil || n != 0 {
			t.Fatalf("close = %d, %v; want 0 after the fresh observation committed", n, err)
		}
		if got := obsClosedExternalIDs(t, pool); len(got) != 0 {
			t.Fatalf("closed = %v, want nothing closed", got)
		}
	})
}
