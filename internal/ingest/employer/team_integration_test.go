//go:build integration

// Integration tests for company-team ownership against Postgres: what only the database
// decides — employer_team's reach over vacancies, one company per user across the two
// tables, and the hand-over when a member deletes their account. Members are seeded
// directly; joining by invitation is tested with the team service.
// Run with: go test -tags=integration ./internal/ingest/employer/
package employer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type teamFixture struct {
	pool   *pgxpool.Pool
	repo   *employer.QueriesRepository
	holder int64
}

func newTeamFixture(t *testing.T) teamFixture {
	t.Helper()
	pool := testdb.Pool(t)
	f := teamFixture{pool: pool, repo: employer.NewQueriesRepository(db.New(pool), pool), holder: seedUser(t, pool, "hr@acme.test")}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO company_accounts (user_id, company_slug, company_name, work_email, status, verified_at, email_confirmed_at)
		VALUES ($1, 'acme', 'Acme', 'hr@acme.test', 'active', now(), now())`, f.holder); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return f
}

// member adds a user to the acme team under workEmail.
func (f teamFixture) member(t *testing.T, email, workEmail string) int64 {
	t.Helper()
	id := seedUser(t, f.pool, email)
	if _, err := f.pool.Exec(context.Background(), `
		INSERT INTO company_account_members (user_id, company_slug, work_email, invited_by) VALUES ($1, 'acme', $2, $3)`,
		id, workEmail, f.holder); err != nil {
		t.Fatalf("seed member: %v", err)
	}
	return id
}

func (f teamFixture) job(t *testing.T, slug string, by int64) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `
		INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_by)
		VALUES ('employer', 'freehire:' || $1, '', 'Platform Engineer', 'Acme', 'acme', $1, $2)`, slug, by); err != nil {
		t.Fatalf("seed job %s: %v", slug, err)
	}
}

func TestTeam_AMemberActsForTheCompany(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	member := f.member(t, "someone@gmail.com", "dev@acme.test")

	acc, err := f.repo.GetByUserID(ctx, member)
	if err != nil {
		t.Fatalf("GetByUserID(member): %v", err)
	}
	if acc.UserID != f.holder || acc.CompanySlug != "acme" || acc.WorkEmail != "dev@acme.test" || !acc.Member || !acc.Active() {
		t.Errorf("member's account = %+v, want the company's, under the member's address", acc)
	}
	if holder, err := f.repo.GetByUserID(ctx, f.holder); err != nil || holder.Member || holder.WorkEmail != "hr@acme.test" {
		t.Errorf("holder's account = %+v, %v", holder, err)
	}

	// Vacancies are the team's: the holder sees and edits what the member published.
	f.job(t, "member-vacancy", member)
	jobs, _, err := f.repo.ListMine(ctx, f.holder)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("holder's ListMine = %d, %v; want the member's vacancy", len(jobs), err)
	}
	if _, _, err := f.repo.BySlug(ctx, f.holder, "member-vacancy"); err != nil {
		t.Errorf("holder's BySlug of the member's vacancy: %v", err)
	}
	stranger := seedUser(t, f.pool, "x@globex.test")
	if _, _, err := f.repo.BySlug(ctx, stranger, "member-vacancy"); !errors.Is(err, employer.ErrJobNotFound) {
		t.Errorf("a stranger's BySlug err = %v, want ErrJobNotFound", err)
	}
	for _, c := range []struct {
		a, b int64
		want bool
	}{{f.holder, member, true}, {member, f.holder, true}, {stranger, stranger, true}, {f.holder, stranger, false}} {
		if got, err := f.repo.Teammates(ctx, c.a, c.b); err != nil || got != c.want {
			t.Errorf("Teammates(%d, %d) = %v, %v; want %v", c.a, c.b, got, err, c.want)
		}
	}

	// One company per user: a member cannot claim another.
	if _, err := f.repo.InsertPending(ctx, member, "globex", "Globex", "dev@globex.test"); !errors.Is(err, employer.ErrAlreadyHasAccount) {
		t.Errorf("a member's claim err = %v, want ErrAlreadyHasAccount", err)
	}
}

func TestTeam_AMemberDeletingTheirAccountHandsTheirVacanciesOver(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	member := f.member(t, "dev.personal@example.test", "dev@acme.test")
	f.job(t, "member-vacancy", member)
	stranger := seedUser(t, f.pool, "x@globex.test")
	f.job(t, "stranger-vacancy", stranger)

	for _, id := range []int64{member, stranger} {
		if _, err := f.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
			t.Fatalf("delete user %d: %v", id, err)
		}
	}
	var createdBy *int64
	if err := f.pool.QueryRow(ctx, `SELECT created_by FROM jobs WHERE public_slug = 'member-vacancy'`).Scan(&createdBy); err != nil || createdBy == nil || *createdBy != f.holder {
		t.Errorf("member's vacancy created_by = %v, %v; want the holder", createdBy, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT created_by FROM jobs WHERE public_slug = 'stranger-vacancy'`).Scan(&createdBy); err != nil || createdBy != nil {
		t.Errorf("a non-member's vacancy created_by = %v, %v; want NULL as before", createdBy, err)
	}
}
