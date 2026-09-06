//go:build integration

// Integration tests for the cmd/queue-metrics aggregates: the live/dead split of an
// outbox, the age of its oldest LIVE entry, the mutually exclusive board-fleet states,
// and catalogue freshness. All four are SQL behaviors — FILTER clauses, a partial index's
// ORDER BY ... LIMIT, and the empty-table cases — verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queueJob inserts a job and returns its id, so an outbox row has something to point at.
// search_outbox is UNIQUE (job_id), so every outbox row in these tests needs its own job.
func queueJob(t *testing.T, pool *pgxpool.Pool, externalID string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, description, public_slug)
		 VALUES ('test', $1, 'http://example.test', 'A job', 'Build things.', 'job-' || $1)
		 RETURNING id`, externalID).Scan(&id)
	if err != nil {
		t.Fatalf("insert job %s: %v", externalID, err)
	}
	return id
}

// queueSearchEntry adds a search_outbox row aged `age` old, dead-lettered or not.
// created_at is set explicitly because the column defaults to now() and these tests are
// entirely about how old an entry is.
//
// The age is subtracted from Postgres's own now(), never from Go's clock. The query under
// test measures age as now() - created_at inside the database, and the container's clock
// runs independently of this process's: seeding from time.Now() made the computed age
// land a few hundred milliseconds either side of the intended value, which is enough to
// fail a bound.
func queueSearchEntry(t *testing.T, pool *pgxpool.Pool, externalID string, age time.Duration, dead bool) {
	t.Helper()
	jobID := queueJob(t, pool, externalID)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO search_outbox (job_id, created_at, failed_at)
		 VALUES ($1, now() - make_interval(secs => $2::float8),
		         CASE WHEN $3::bool THEN now() ELSE NULL END)`,
		jobID, age.Seconds(), dead)
	if err != nil {
		t.Fatalf("insert search_outbox entry %s: %v", externalID, err)
	}
}

func TestSearchOutboxMetricsSplitsLiveFromDeadLettered(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	// The dead-lettered entry is deliberately the OLDEST row in the table. If the
	// min(created_at) lost its FILTER, the reported age would jump to 72h — so this
	// arrangement is what makes the assertion meaningful rather than incidental.
	queueSearchEntry(t, pool, "dead-oldest", 72*time.Hour, true)
	queueSearchEntry(t, pool, "dead-second", 48*time.Hour, true)
	queueSearchEntry(t, pool, "live-oldest", 6*time.Hour, false)
	queueSearchEntry(t, pool, "live-middle", 2*time.Hour, false)
	queueSearchEntry(t, pool, "live-newest", 5*time.Minute, false)

	got, err := q.SearchOutboxMetrics(context.Background())
	if err != nil {
		t.Fatalf("SearchOutboxMetrics: %v", err)
	}

	if got.Depth != 3 {
		t.Errorf("depth = %d, want 3 (live entries only)", got.Depth)
	}
	if got.DeadLetters != 2 {
		t.Errorf("dead_letters = %d, want 2", got.DeadLetters)
	}
	// The oldest LIVE entry is 6h old; allow a minute of slack for test runtime.
	if wantSec := (6 * time.Hour).Seconds(); got.OldestAgeSeconds < wantSec || got.OldestAgeSeconds > wantSec+60 {
		t.Errorf("oldest_age_seconds = %v, want ~%v (the oldest LIVE entry, not the 72h dead one)",
			got.OldestAgeSeconds, wantSec)
	}
}

func TestSearchOutboxMetricsReportsZeroesWhenDrained(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	got, err := q.SearchOutboxMetrics(context.Background())
	if err != nil {
		t.Fatalf("SearchOutboxMetrics: %v", err)
	}

	// A drained queue must publish explicit zeroes, not NULL and not an absent row:
	// the consuming alert rules read a missing series as a dead exporter, which is a
	// different incident from a healthy empty queue.
	if got.Depth != 0 || got.DeadLetters != 0 {
		t.Errorf("empty queue reported depth=%d dead=%d, want 0 and 0", got.Depth, got.DeadLetters)
	}
	if got.OldestAgeSeconds != 0 {
		t.Errorf("empty queue reported oldest_age_seconds = %v, want 0", got.OldestAgeSeconds)
	}
}

func TestEnrichmentAndSemanticOutboxMetricsUseTheSameShape(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	enrichJob := queueJob(t, pool, "enrich-live")
	enrichDead := queueJob(t, pool, "enrich-dead")
	if _, err := pool.Exec(ctx,
		`INSERT INTO enrichment_outbox (job_id, target_version, created_at, failed_at)
		 VALUES ($1, 1, now() - interval '3 hours', NULL), ($2, 1, now() - interval '9 hours', now())`,
		enrichJob, enrichDead); err != nil {
		t.Fatalf("seed enrichment_outbox: %v", err)
	}

	semanticJob := queueJob(t, pool, "semantic-live")
	if _, err := pool.Exec(ctx,
		`INSERT INTO semantic_outbox (job_id, target_model, created_at) VALUES ($1, 'test-model', now() - interval '1 hour')`,
		semanticJob); err != nil {
		t.Fatalf("seed semantic_outbox: %v", err)
	}

	enrich, err := q.EnrichmentOutboxMetrics(ctx)
	if err != nil {
		t.Fatalf("EnrichmentOutboxMetrics: %v", err)
	}
	if enrich.Depth != 1 || enrich.DeadLetters != 1 {
		t.Errorf("enrichment depth=%d dead=%d, want 1 and 1", enrich.Depth, enrich.DeadLetters)
	}
	if wantSec := (3 * time.Hour).Seconds(); enrich.OldestAgeSeconds < wantSec || enrich.OldestAgeSeconds > wantSec+60 {
		t.Errorf("enrichment oldest_age_seconds = %v, want ~%v (the live 3h entry, not the dead 9h one)",
			enrich.OldestAgeSeconds, wantSec)
	}

	semantic, err := q.SemanticOutboxMetrics(ctx)
	if err != nil {
		t.Fatalf("SemanticOutboxMetrics: %v", err)
	}
	if semantic.Depth != 1 || semantic.DeadLetters != 0 {
		t.Errorf("semantic depth=%d dead=%d, want 1 and 0", semantic.Depth, semantic.DeadLetters)
	}
}

func TestBoardHealthMetricsStatesAreMutuallyExclusive(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	// Five boards spanning every combination that matters, including the one that
	// decides the precedence rule: failing AND cooled must be counted as cooled only.
	if _, err := pool.Exec(ctx,
		`INSERT INTO board_health (provider, board, consecutive_failures, cooldown_until) VALUES
		 ('greenhouse', 'healthy-board',      0, NULL),
		 ('greenhouse', 'recovered-board',    0, now() - interval '1 hour'),
		 ('ashby',      'failing-board',      2, NULL),
		 ('ashby',      'failing-expired',    2, now() - interval '5 minutes'),
		 ('lever',      'failing-and-cooled', 4, now() + interval '6 hours')`); err != nil {
		t.Fatalf("seed board_health: %v", err)
	}

	got, err := q.BoardHealthMetrics(ctx)
	if err != nil {
		t.Fatalf("BoardHealthMetrics: %v", err)
	}

	if got.Cooled != 1 {
		t.Errorf("cooled = %d, want 1 (only the board whose cooldown is still in the future)", got.Cooled)
	}
	if got.Failing != 2 {
		t.Errorf("failing = %d, want 2 (failures with no ACTIVE cooldown; the cooled one is not double-counted)", got.Failing)
	}
	if got.Healthy != 2 {
		t.Errorf("healthy = %d, want 2 (no failures, expired cooldown counts as healthy)", got.Healthy)
	}
	if total := got.Cooled + got.Failing + got.Healthy; total != 5 {
		t.Errorf("states sum to %d, want 5 — they must partition the fleet exactly", total)
	}
}

func TestNewestOpenJobCreatedAtIgnoresClosedJobs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	openID := queueJob(t, pool, "open-job")
	closedID := queueJob(t, pool, "closed-job")
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET created_at = now() - interval '2 hours' WHERE id = $1`, openID); err != nil {
		t.Fatalf("age the open job: %v", err)
	}
	// The closed job is NEWER than the open one, so a query that forgot the
	// closed_at filter would return this row's timestamp instead.
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET created_at = now(), closed_at = now() WHERE id = $1`, closedID); err != nil {
		t.Fatalf("close the newer job: %v", err)
	}

	got, err := q.NewestOpenJobCreatedAt(ctx)
	if err != nil {
		t.Fatalf("NewestOpenJobCreatedAt: %v", err)
	}
	assertRoughlyAgo(t, got, 2*time.Hour)
}

func TestNewestOpenJobCreatedAtReportsNoRowsForAnEmptyCatalogue(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	_, err := q.NewestOpenJobCreatedAt(context.Background())

	// No rows, NOT a zero timestamp: zero reads as 1970, i.e. an infinitely stale
	// catalogue, whereas an empty catalogue is a fresh-install state. The caller
	// distinguishes them on this error.
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("NewestOpenJobCreatedAt on an empty catalogue: err = %v, want pgx.ErrNoRows", err)
	}
}

// assertRoughlyAgo checks a returned timestamp is about `want` old. The tolerance is
// symmetric because this comparison straddles two clocks — the row was aged by Postgres's
// now(), and time.Since reads this process's — so the difference can land on either side
// of the intended age regardless of how fast the test runs.
func assertRoughlyAgo(t *testing.T, ts pgtype.Timestamptz, want time.Duration) {
	t.Helper()
	if !ts.Valid {
		t.Fatal("timestamp is not valid, want a real value")
	}
	if got := time.Since(ts.Time); got < want-time.Minute || got > want+time.Minute {
		t.Errorf("timestamp is %v old, want ~%v", got, want)
	}
}

// queueAutoApplyEntry adds one auto_apply_queue row in a named state. Everything here is a
// column combination rather than a call to the real writers, because the point is what the
// AGGREGATE makes of each state, not how a row reaches it.
func queueAutoApplyEntry(t *testing.T, pool *pgxpool.Pool, externalID string, set string) {
	t.Helper()
	var userID int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, externalID+"@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user for %s: %v", externalID, err)
	}
	jobID := queueJob(t, pool, externalID)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO auto_apply_queue (user_id, job_id) VALUES ($1, $2)`, userID, jobID); err != nil {
		t.Fatalf("insert auto_apply_queue entry %s: %v", externalID, err)
	}
	if set == "" {
		return
	}
	// tailored_cv_id carries a foreign key, so the states that name one need a real CV
	// rather than a generated uuid.
	if strings.Contains(set, "tailored_cv_id") {
		var cvID uuid.UUID
		if err := pool.QueryRow(context.Background(),
			`INSERT INTO cvs (user_id, title, data) VALUES ($1, 'CV', '{}') RETURNING id`, userID).Scan(&cvID); err != nil {
			t.Fatalf("insert cv for %s: %v", externalID, err)
		}
		set = strings.ReplaceAll(set, "gen_random_uuid()", "'"+cvID.String()+"'::uuid")
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE auto_apply_queue SET `+set+` WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("shape auto_apply_queue entry %s (%s): %v", externalID, set, err)
	}
}

// TestAutoApplyQueueMetricsCountsWhatAWorkerWillActuallyPickUp is the guard for the two
// readings this aggregate is easiest to get wrong, both of which it shipped with.
//
// A DECLINE is not parked work. DeclineAutoApplyReview reuses the park vocabulary and
// stamps blocked_at, and nothing ever deletes the row — so a `blocked` count that reads
// blocked_at alone grows with ordinary candidate behaviour until the sidecar population it
// exists to publish is a rounding error inside it, and the alert it was created for can
// never be set.
//
// A dead letter has TWO markers. preview_failed_at retires the preview pass exactly as
// failed_at retires the submit pass, and a row carrying only the first is claimed by
// neither batch — the definition of a population that must not be left out.
//
// The three counts must also stay mutually exclusive: cmd/queue-metrics publishes them as
// a stacked reading, so an entry counted twice is a graph that does not add up.
func TestAutoApplyQueueMetricsCountsWhatAWorkerWillActuallyPickUp(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	queueAutoApplyEntry(t, pool, "aa-owed", `tailored_cv_id = gen_random_uuid(), review_decision = 'approved'`)
	queueAutoApplyEntry(t, pool, "aa-submit-dead", `failed_at = now()`)
	queueAutoApplyEntry(t, pool, "aa-preview-dead", `preview_failed_at = now()`)
	queueAutoApplyEntry(t, pool, "aa-parked", `blocked_at = now()`)
	queueAutoApplyEntry(t, pool, "aa-declined", `blocked_at = now(), review_decision = 'declined'`)
	// Awaiting the candidate: no decision yet, nothing wrong with it. In none of the three.
	queueAutoApplyEntry(t, pool, "aa-awaiting", `tailored_cv_id = gen_random_uuid()`)

	got, err := q.AutoApplyQueueMetrics(context.Background())
	if err != nil {
		t.Fatalf("AutoApplyQueueMetrics: %v", err)
	}
	if got.Depth != 1 {
		t.Errorf("depth = %d, want 1 — only the approved entry is work a run will pick up", got.Depth)
	}
	if got.DeadLetters != 2 {
		t.Errorf("dead letters = %d, want 2 — a preview that gave up is as unclaimable as a submit that did",
			got.DeadLetters)
	}
	if got.Blocked != 1 {
		t.Errorf("blocked = %d, want 1 — a candidate's own decline is not an attempt parked for want of data",
			got.Blocked)
	}
	if sum := got.Depth + got.DeadLetters + got.Blocked; sum != 4 {
		t.Errorf("the three counts total %d over 6 rows, want 4 — they are published stacked, so an "+
			"entry counted twice is a graph that does not add up", sum)
	}
}
