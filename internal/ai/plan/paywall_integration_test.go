//go:build integration

package plan

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

func insertPaywallJob(t *testing.T, pool *pgxpool.Pool, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('greenhouse', $1, 'https://example.test/j/'||$1, 'Go Developer', 'Acme', $1)
		 RETURNING id`, slug).Scan(&id); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

func insertPaywallCV(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO cvs (user_id, title, template_id, data, job_id)
		 VALUES ($1, 'Tailored', 'classic-ats', '{}'::jsonb, $2) RETURNING id`, userID, jobID).Scan(&id); err != nil {
		t.Fatalf("seed cv: %v", err)
	}
	return id
}

func insertPaywallSession(t *testing.T, pool *pgxpool.Pool, userID, jobID int64) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO assistant_sessions (user_id, preset, job_id) VALUES ($1, 'chat', $2) RETURNING id`,
		userID, jobID).Scan(&id); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return id
}

// paywallJobSlug reads the job slug of the single paywall_hits row for (user, feature); ok is
// false when the row has no job.
func paywallJobSlug(t *testing.T, pool *pgxpool.Pool, userID int64, feature string) (slug string, ok bool) {
	t.Helper()
	var s *string
	if err := pool.QueryRow(context.Background(),
		`SELECT j.public_slug FROM paywall_hits h LEFT JOIN jobs j ON j.id = h.job_id
		 WHERE h.user_id = $1 AND h.feature = $2`, userID, feature).Scan(&s); err != nil {
		t.Fatalf("read paywall hit for %s: %v", feature, err)
	}
	if s == nil {
		return "", false
	}
	return *s, true
}

func countPaywallHits(t *testing.T, pool *pgxpool.Pool, userID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM paywall_hits WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count paywall hits: %v", err)
	}
	return n
}

func TestRecordPaywallHitResolvesTheJobFromEachIdentifier(t *testing.T) {
	s, pool := newStore(t, enforcing(), time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	ctx := context.Background()
	user := insertUser(t, pool, "paywall-resolve@example.test")
	job := insertPaywallJob(t, pool, "paywall-job")
	cv := insertPaywallCV(t, pool, user, job)
	session := insertPaywallSession(t, pool, user, job)

	for _, tc := range []struct {
		feature string
		job     PaywallJob
	}{
		{"match", PaywallJob{Slug: "paywall-job"}},
		{"tailor", PaywallJob{CVID: cv}},
		{"assistant", PaywallJob{SessionID: session}},
	} {
		if err := s.RecordPaywallHit(ctx, user, tc.feature, tc.job); err != nil {
			t.Fatalf("%s: %v", tc.feature, err)
		}
		if slug, ok := paywallJobSlug(t, pool, user, tc.feature); !ok || slug != "paywall-job" {
			t.Errorf("%s recorded job %q (set=%v), want paywall-job", tc.feature, slug, ok)
		}
	}
}

func TestRecordPaywallHitNeverResolvesAJobItCannotProve(t *testing.T) {
	s, pool := newStore(t, enforcing(), time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	ctx := context.Background()
	user := insertUser(t, pool, "paywall-free@example.test")
	other := insertUser(t, pool, "paywall-other@example.test")
	job := insertPaywallJob(t, pool, "someone-elses-job")
	foreignCV := insertPaywallCV(t, pool, other, job)
	foreignSession := insertPaywallSession(t, pool, other, job)

	for _, tc := range []struct {
		feature string
		job     PaywallJob
	}{
		{"unknown-slug", PaywallJob{Slug: "no-such-job"}},
		{"foreign-cv", PaywallJob{CVID: foreignCV}},
		{"foreign-session", PaywallJob{SessionID: foreignSession}},
		{"unknown-cv", PaywallJob{CVID: uuid.New()}},
		{"no-job", PaywallJob{}},
	} {
		if err := s.RecordPaywallHit(ctx, user, tc.feature, tc.job); err != nil {
			t.Fatalf("%s: %v", tc.feature, err)
		}
		if slug, ok := paywallJobSlug(t, pool, user, tc.feature); ok {
			t.Errorf("%s resolved to job %q, want none", tc.feature, slug)
		}
	}
}

func TestRecordPaywallHitIsOncePerUserFeatureJobDay(t *testing.T) {
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "paywall-dedupe@example.test")
	insertPaywallJob(t, pool, "job-a")
	insertPaywallJob(t, pool, "job-b")

	record := func(feature string, job PaywallJob) {
		t.Helper()
		if err := s.RecordPaywallHit(ctx, user, feature, job); err != nil {
			t.Fatalf("record %s: %v", feature, err)
		}
	}
	record("match", PaywallJob{Slug: "job-a"})
	record("match", PaywallJob{Slug: "job-a"}) // the same wall again
	record("match", PaywallJob{})
	record("match", PaywallJob{Slug: "gone"}) // folds into the no-job row
	record("match", PaywallJob{Slug: "job-b"})
	record("tailor", PaywallJob{Slug: "job-a"})
	if got := countPaywallHits(t, pool, user); got != 4 {
		t.Fatalf("rows = %d, want 4 (match/job-a, match/none, match/job-b, tailor/job-a)", got)
	}

	s.now = func() time.Time { return now.Add(24 * time.Hour) }
	record("match", PaywallJob{Slug: "job-a"})
	if got := countPaywallHits(t, pool, user); got != 5 {
		t.Fatalf("rows after the next day = %d, want 5", got)
	}
}

func TestDeletingTheUserErasesTheirPaywallHits(t *testing.T) {
	s, pool := newStore(t, enforcing(), time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	ctx := context.Background()
	user := insertUser(t, pool, "paywall-erase@example.test")
	insertPaywallJob(t, pool, "erase-job")
	if err := s.RecordPaywallHit(ctx, user, "match", PaywallJob{Slug: "erase-job"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if got := countPaywallHits(t, pool, user); got != 0 {
		t.Errorf("rows after deleting the account = %d, want 0", got)
	}
}

func TestDeletingTheJobKeepsTheHitWithoutIt(t *testing.T) {
	s, pool := newStore(t, enforcing(), time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC))
	ctx := context.Background()
	user := insertUser(t, pool, "paywall-jobgone@example.test")
	job := insertPaywallJob(t, pool, "vanishing-job")
	if err := s.RecordPaywallHit(ctx, user, "match", PaywallJob{Slug: "vanishing-job"}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job); err != nil {
		t.Fatalf("delete job: %v", err)
	}
	if got := countPaywallHits(t, pool, user); got != 1 {
		t.Fatalf("rows after the job went = %d, want 1", got)
	}
	if _, ok := paywallJobSlug(t, pool, user, "match"); ok {
		t.Error("the hit still names a job that no longer exists")
	}
}

func TestListPaywallHitsByFeatureReadsTheLastWindow(t *testing.T) {
	s, pool := newStore(t, enforcing(), time.Now().UTC())
	ctx := context.Background()
	user := insertUser(t, pool, "paywall-list@example.test")
	insertPaywallJob(t, pool, "list-job")
	for _, h := range []struct {
		feature string
		job     PaywallJob
	}{
		{"tailor", PaywallJob{Slug: "list-job"}},
		{"ats_review", PaywallJob{}},
		{"match", PaywallJob{Slug: "list-job"}},
	} {
		if err := s.RecordPaywallHit(ctx, user, h.feature, h.job); err != nil {
			t.Fatalf("record %s: %v", h.feature, err)
		}
	}
	// One older than the window, which the report must not show.
	if _, err := pool.Exec(ctx,
		`INSERT INTO paywall_hits (user_id, feature, day, created_at)
		 VALUES ($1, 'autofill', current_date - 30, now() - interval '30 days')`, user); err != nil {
		t.Fatalf("seed old hit: %v", err)
	}

	rows, err := db.New(pool).ListPaywallHitsByFeature(ctx, 7)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var features []string
	for _, r := range rows {
		features = append(features, r.Feature)
		if r.UserID != user {
			t.Errorf("row for user %d, want %d", r.UserID, user)
		}
		if r.Feature == "ats_review" && r.JobSlug.Valid {
			t.Errorf("ats_review carries job %q, want none", r.JobSlug.String)
		}
		if r.Feature == "match" && r.JobSlug.String != "list-job" {
			t.Errorf("match job = %q, want list-job", r.JobSlug.String)
		}
	}
	want := []string{"ats_review", "match", "tailor"}
	if len(features) != len(want) {
		t.Fatalf("features = %v, want %v ordered by feature, without the 30-day-old autofill", features, want)
	}
	for i := range want {
		if features[i] != want[i] {
			t.Fatalf("features = %v, want %v", features, want)
		}
	}
}
