//go:build integration

package onboarding

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func tailorUser(t *testing.T, pool *pgxpool.Pool, email string, parsedCV bool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if parsedCV {
		if _, err := pool.Exec(context.Background(),
			`UPDATE users SET resume_uploaded_at = now(), resume_extract_status = 'ok', resume_structured_uploaded_at = now() WHERE id = $1`, id); err != nil {
			t.Fatalf("parse CV: %v", err)
		}
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO onboarding_emails (user_id, step, sent_at) VALUES ($1, 'welcome', now() - interval '5 days')`, id); err != nil {
		t.Fatalf("welcome row: %v", err)
	}
	return id
}

func tailorIDs(t *testing.T, q *db.Queries) map[int64]bool {
	t.Helper()
	rows, err := q.ListTailorCandidates(context.Background(), db.ListTailorCandidatesParams{WindowDays: 30, AfterDays: 3, MaxRows: 100})
	if err != nil {
		t.Fatalf("ListTailorCandidates: %v", err)
	}
	got := map[int64]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}
	return got
}

func TestListTailorCandidates_LeavesAParsedCVToTheLifecycleLetters(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	without := tailorUser(t, pool, "tailor-nocv@example.test", false)
	with := tailorUser(t, pool, "tailor-cv@example.test", true)

	got := tailorIDs(t, q)
	if !got[without] {
		t.Errorf("an account without a parsed CV was not a candidate")
	}
	if got[with] {
		t.Errorf("an account with a parsed CV is a candidate; the lifecycle letters own it")
	}
}

func TestListTailorCandidates_ReUploadNotYetParsedIsStillACandidate(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	id := tailorUser(t, pool, "tailor-reupload@example.test", true)
	if _, err := pool.Exec(context.Background(), `UPDATE users SET resume_uploaded_at = now() + interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if !tailorIDs(t, q)[id] {
		t.Errorf("a CV that is uploaded but not parsed again should not count as parsed")
	}
}

func TestListTailorCandidates_SkipsAnAccountHoldingTodaysClaim(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	id := tailorUser(t, pool, "tailor-claimed@example.test", false)
	if _, err := pool.Exec(context.Background(), `INSERT INTO mail_day_claims (user_id, day, source) VALUES ($1, mail_today(), 'broadcast')`, id); err != nil {
		t.Fatal(err)
	}
	if tailorIDs(t, q)[id] {
		t.Errorf("an account that already got its letter today is a candidate")
	}
}
