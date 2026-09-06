//go:build integration

// Integration tests for the classify-mail worker's two SQL guarantees: it may not overwrite
// a link a person or an agent made, and a failure it did not cause may not spend the
// message's attempt budget. Both are properties of the statements themselves — a CASE over
// the row's existing link_source, and a two-branch failed_at — so they can only be verified
// against a real Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// linkState is the four columns that say what a message is attached to and who attached it.
type linkState struct {
	jobID         pgtype.Int8
	applicationID pgtype.Int8
	linkSource    pgtype.Text
	confidence    pgtype.Float4
	signal        pgtype.Text
	classifiedAt  pgtype.Timestamptz
}

func readLinkState(t *testing.T, pool *pgxpool.Pool, id int64) linkState {
	t.Helper()
	var s linkState
	err := pool.QueryRow(context.Background(),
		`SELECT job_id, application_id, link_source, match_confidence, status_signal, classified_at
		 FROM emails WHERE id = $1`, id).
		Scan(&s.jobID, &s.applicationID, &s.linkSource, &s.confidence, &s.signal, &s.classifiedAt)
	if err != nil {
		t.Fatalf("read link state: %v", err)
	}
	return s
}

// linkEmail attaches a message to a job under a given provenance, the way the manual and
// agent paths do (LinkEmailToJob writes exactly this shape).
func linkEmail(t *testing.T, pool *pgxpool.Pool, emailID, jobID int64, source string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE emails
		    SET job_id = $2, link_source = $3, match_confidence = 1,
		        application_id = (SELECT a.id FROM applications a
		                           WHERE a.user_id = emails.user_id AND a.job_id = $2)
		  WHERE emails.id = $1`, emailID, jobID, source)
	if err != nil {
		t.Fatalf("seed %s link: %v", source, err)
	}
}

// TestSetEmailClassificationKeepsALinkItDidNotMake is the regression that matters most
// here. A hand-made link does not stamp classified_at, and EnqueuePendingEmailClassification
// selects on classified_at IS NULL — so every manually linked message comes back round on
// the next five-minute tick. The worker then writes its own answer, and for a closed posting
// or a message with no thread the deterministic cascade cannot reproduce the link at all:
// job_id NULL, application_id NULL, and (because ReconcileMailEvent runs in the same
// transaction) the employer_reply event retracted with it.
func TestSetEmailClassificationKeepsALinkItDidNotMake(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	for _, provenance := range []string{"manual", "agent"} {
		t.Run(provenance, func(t *testing.T) {
			user := insertUser(t, pool, provenance+"-keeps@example.test")
			job := insertJob(t, pool, provenance+"-kept-job")
			applyToJob(t, pool, user, job, "applied")
			email := insertEmailWithSource(t, pool, user, "hosted", provenance+"-kept-mail")
			linkEmail(t, pool, email, job, provenance)

			// What the worker writes for a message it could not match: every link
			// argument NULL, which int8OrNull(0) produces for an unmatched result.
			err := q.SetEmailClassification(ctx, SetEmailClassificationParams{
				StatusSignal: pgtype.Text{String: "rejection", Valid: true},
				Model:        pgtype.Text{String: "test-model", Valid: true},
				ID:           email,
				UserID:       user,
			})
			if err != nil {
				t.Fatalf("classify: %v", err)
			}

			s := readLinkState(t, pool, email)
			if s.jobID.Int64 != job {
				t.Errorf("job_id = %v, want %d — the worker detached a %s link", s.jobID, job, provenance)
			}
			if !s.applicationID.Valid {
				t.Error("application_id was cleared, so the ledger and the reply-rate rollup lose this reply")
			}
			if s.linkSource.String != provenance {
				t.Errorf("link_source = %q, want %q — provenance was demoted", s.linkSource.String, provenance)
			}
			if s.confidence.Float32 != 1 {
				t.Errorf("match_confidence = %v, want the 1 the link was made with", s.confidence)
			}
			// The classification itself is still the worker's to write.
			if s.signal.String != "rejection" || !s.classifiedAt.Valid {
				t.Errorf("classification not persisted: signal=%q classified=%v", s.signal.String, s.classifiedAt.Valid)
			}
		})
	}
}

// The guard must not freeze the worker out of its OWN links: an 'auto' link is a previous
// run's answer, and a later run correcting it is the queue working as designed.
func TestSetEmailClassificationRewritesItsOwnEarlierLink(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "auto-relink@example.test")
	first := insertJob(t, pool, "auto-relink-first")
	second := insertJob(t, pool, "auto-relink-second")
	applyToJob(t, pool, user, first, "applied")
	applyToJob(t, pool, user, second, "applied")
	email := insertEmailWithSource(t, pool, user, "gmail", "auto-relink-mail")
	linkEmail(t, pool, email, first, "auto")

	err := q.SetEmailClassification(ctx, SetEmailClassificationParams{
		JobID:           pgtype.Int8{Int64: second, Valid: true},
		LinkSource:      pgtype.Text{String: "auto", Valid: true},
		MatchConfidence: pgtype.Float4{Float32: 0.9, Valid: true},
		StatusSignal:    pgtype.Text{String: "screening", Valid: true},
		Model:           pgtype.Text{String: "test-model", Valid: true},
		ID:              email,
		UserID:          user,
	})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}

	s := readLinkState(t, pool, email)
	if s.jobID.Int64 != second {
		t.Errorf("job_id = %v, want %d — the worker may correct a link it made itself", s.jobID, second)
	}
	if !s.applicationID.Valid {
		t.Error("application_id was not re-derived for the new job")
	}
}

// queueEmail puts one message in the classification outbox and returns the entry id.
func queueEmail(t *testing.T, pool *pgxpool.Pool, emailID int64, attempts int, ageDays int) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO email_classification_outbox (email_id, attempts, created_at)
		 VALUES ($1, $2, now() - make_interval(days => $3)) RETURNING id`,
		emailID, attempts, ageDays).Scan(&id)
	if err != nil {
		t.Fatalf("queue email: %v", err)
	}
	return id
}

// A gateway or database failure says nothing about the message, and the attempt counter does
// not measure how long an outage lasts: the lease makes a claimed entry re-claimable minutes
// later, so three attempts are spent well inside a short one. Nothing clears failed_at and
// the pending enqueue is ON CONFLICT DO NOTHING, so burying here is permanent.
func TestFailEmailClassificationDoesNotBuryAMessageForSomeoneElsesOutage(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "outage@example.test")
	email := insertEmailWithSource(t, pool, user, "gmail", "outage-mail")
	entry := queueEmail(t, pool, email, 2, 0) // one attempt short of the old ceiling

	row, err := q.FailEmailClassification(ctx, FailEmailClassificationParams{
		LastError:         "API returned unexpected status code: 502",
		MessageAtFault:    false,
		MaxAttempts:       3,
		UpstreamGraceDays: 14,
		ID:                entry,
	})
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if row.Attempts != 3 {
		t.Fatalf("attempts = %d, want 3 — the fixture no longer reaches the ceiling", row.Attempts)
	}
	if row.FailedAt.Valid {
		t.Error("a gateway failure dead-lettered the message on the attempt ceiling; " +
			"a fifteen-minute outage buries the whole queue permanently that way")
	}
}

// The age bound is not "never bury": an entry nothing can ever serve has to stop eventually.
func TestFailEmailClassificationBuriesAnEntryOlderThanTheGraceWindow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "stale-outage@example.test")
	email := insertEmailWithSource(t, pool, user, "gmail", "stale-outage-mail")
	entry := queueEmail(t, pool, email, 0, 30)

	row, err := q.FailEmailClassification(ctx, FailEmailClassificationParams{
		LastError:         "API returned unexpected status code: 502",
		MessageAtFault:    false,
		MaxAttempts:       3,
		UpstreamGraceDays: 14,
		ID:                entry,
	})
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if !row.FailedAt.Valid {
		t.Error("an entry 30 days into a 14-day grace window was not dead-lettered on its first attempt")
	}
}

// A window of zero must mean "never bury on age" rather than "bury everything": left as an
// arithmetic comparison, created_at < now() - 0 is true for every row, so a caller that
// forgot to set the window would dead-letter the queue on its first failure.
func TestFailEmailClassificationTreatsAMissingGraceWindowAsUnbounded(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "nowindow@example.test")
	email := insertEmailWithSource(t, pool, user, "gmail", "nowindow-mail")
	entry := queueEmail(t, pool, email, 0, 365)

	row, err := q.FailEmailClassification(ctx, FailEmailClassificationParams{
		LastError:         "connection refused",
		MessageAtFault:    false,
		MaxAttempts:       3,
		UpstreamGraceDays: 0,
		ID:                entry,
	})
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if row.FailedAt.Valid {
		t.Error("a misconfigured (zero) grace window buried the entry; it must cost retries, not mail")
	}
}

// The other branch: something about THIS message cannot be processed, so each try is a real
// try at something that may be impossible, and the attempt ceiling is what stops it.
func TestFailEmailClassificationBuriesAMessageAtFaultOnTheAttemptCeiling(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := insertUser(t, pool, "atfault@example.test")
	email := insertEmailWithSource(t, pool, user, "gmail", "atfault-mail")
	entry := queueEmail(t, pool, email, 2, 0)

	row, err := q.FailEmailClassification(ctx, FailEmailClassificationParams{
		LastError:         "mailclassify: unparseable response",
		MessageAtFault:    true,
		MaxAttempts:       3,
		UpstreamGraceDays: 14,
		ID:                entry,
	})
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if !row.FailedAt.Valid {
		t.Error("a message at fault was not dead-lettered on its third attempt, so nothing bounds it")
	}
}
