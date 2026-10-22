//go:build integration

// Integration test for ListReferralCVs: which of a seeker's CVs fit a referral request. The
// fit is the query's own predicate, so only a real Postgres can say it holds.
// Run with: go test -tags=integration ./internal/engage/referral/
package referral_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/engage/referral"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestReferralCVs_FitTheVacancyOrTheCompany(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	repo := referral.NewQueriesRepository(db.New(pool))

	var seeker, other int64
	for email, id := range map[string]*int64{"s@example.test": &seeker, "o@example.test": &other} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(id); err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	job := func(slug, company string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug)
			VALUES ('greenhouse', $1, '', 'Engineer', $2, $2, $1) RETURNING id`, slug, company).Scan(&id); err != nil {
			t.Fatalf("seed job: %v", err)
		}
		return id
	}
	cvFor := func(user int64, title string, jobID *int64) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO cvs (user_id, title, data, job_id) VALUES ($1, $2, '{}', $3) RETURNING id`, user, title, jobID).Scan(&id); err != nil {
			t.Fatalf("seed cv: %v", err)
		}
		return id
	}
	acmeA, acmeB, globex := job("acme-a", "acme"), job("acme-b", "acme"), job("globex-a", "globex")
	forA := cvFor(seeker, "For acme A", &acmeA)
	forB := cvFor(seeker, "For acme B", &acmeB)
	cvFor(seeker, "For globex", &globex)
	cvFor(seeker, "Base", nil)
	cvFor(other, "Someone else's, for acme A", &acmeA)

	ids := func(opts []referral.CVOption) map[uuid.UUID]bool {
		m := map[uuid.UUID]bool{}
		for _, o := range opts {
			m[o.ID] = true
		}
		return m
	}

	fromVacancy, err := repo.ReferralCVs(ctx, seeker, "acme", &acmeA)
	if err != nil {
		t.Fatalf("ReferralCVs(vacancy): %v", err)
	}
	if got := ids(fromVacancy); len(got) != 1 || !got[forA] {
		t.Errorf("from the vacancy = %v, want only the CV tailored for it", fromVacancy)
	}

	fromCompany, err := repo.ReferralCVs(ctx, seeker, "acme", nil)
	if err != nil {
		t.Fatalf("ReferralCVs(company): %v", err)
	}
	if got := ids(fromCompany); len(got) != 2 || !got[forA] || !got[forB] {
		t.Errorf("from the company = %v, want the two CVs tailored for its vacancies", fromCompany)
	}
}
