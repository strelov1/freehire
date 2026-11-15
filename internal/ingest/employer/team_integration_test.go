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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/ingest/moderation"
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

func TestTeam_DeletingTheAccountDetachesTheMembersVacancies(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	member := f.member(t, "dev.personal@example.test", "dev@acme.test")
	f.job(t, "member-vacancy", member)

	// The claimant deletes their freehire account: the company account and its team go.
	if _, err := f.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, f.holder); err != nil {
		t.Fatalf("delete claimant: %v", err)
	}
	var createdBy *int64
	if err := f.pool.QueryRow(ctx, `SELECT created_by FROM jobs WHERE public_slug = 'member-vacancy'`).Scan(&createdBy); err != nil || createdBy != nil {
		t.Fatalf("member's vacancy created_by = %v, %v; want NULL, detached with the account", createdBy, err)
	}

	// The former member claims another company: the old vacancy does not follow them.
	if _, err := f.repo.InsertPending(ctx, member, "globex", "Globex", "dev@globex.test"); err != nil {
		t.Fatalf("former member's claim: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE company_accounts SET status = 'active' WHERE user_id = $1`, member); err != nil {
		t.Fatalf("activate: %v", err)
	}
	jobs, _, err := f.repo.ListMine(ctx, member)
	if err != nil || len(jobs) != 0 {
		t.Errorf("the new company's vacancies = %d, %v; want none of the old company's", len(jobs), err)
	}
}

// A vacancy write in flight holds its author's membership and company rows, so deleting the
// account waits for it and then detaches the vacancy it wrote — the write is not missed by
// the trigger's snapshot.
func TestTeam_AnInFlightVacancyWriteIsDetachedWithTheAccount(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	member := f.member(t, "dev.personal@example.test", "dev@acme.test")

	// The write takes the account row first, as moderation's lockActingCompany does; the
	// deletion then starts BETWEEN the write's two locks — the interleaving that would
	// deadlock if the write took the membership first.
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	if _, err := q.LockCompanyAccountBySlug(ctx, "acme"); err != nil {
		t.Fatalf("LockCompanyAccountBySlug: %v", err)
	}

	deleted := make(chan error, 1)
	go func() {
		_, err := f.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, f.holder)
		deleted <- err
	}()
	select {
	case err := <-deleted:
		t.Fatalf("the account was deleted while a vacancy write held its row (err %v)", err)
	case <-time.After(300 * time.Millisecond):
	}
	if slug, err := q.LockActingMembership(ctx, member); err != nil || slug != "acme" {
		t.Fatalf("LockActingMembership with the deletion waiting = %q, %v", slug, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug, created_by)
		VALUES ('employer', 'freehire:inflight', '', 'Platform Engineer', 'Acme', 'acme', 'inflight', $1)`, member); err != nil {
		t.Fatalf("insert in flight: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	select {
	case err := <-deleted:
		if err != nil {
			t.Fatalf("delete claimant: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the account deletion never proceeded after the write committed")
	}
	var createdBy *int64
	if err := f.pool.QueryRow(ctx, `SELECT created_by FROM jobs WHERE public_slug = 'inflight'`).Scan(&createdBy); err != nil || createdBy != nil {
		t.Errorf("the in-flight vacancy's created_by = %v, %v; want NULL, detached with the account", createdBy, err)
	}
}

func TestTeam_AWriteByAUserWhoActsForNoCompanyIsRefused(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	nobody := seedUser(t, f.pool, "nobody@example.test")
	mod := moderation.New(moderation.NewQueriesRepository(db.New(f.pool), f.pool, enrich.Version))
	_, _, err := mod.Create(ctx, nobody, moderation.CreateInput{
		URL: "https://acme.test/jobs/9", Source: "employer", Title: "Go Engineer", Company: "Acme",
		Description: "Build things.", Employer: moderation.EmployerOwner{UserID: nobody, CompanySlug: "acme"},
	})
	if !errors.Is(err, moderation.ErrNoCompany) {
		t.Errorf("err = %v, want ErrNoCompany", err)
	}
}

func TestTeam_AWriteForACompanyTheAuthorLeftIsRefused(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	member := f.member(t, "dev.personal@example.test", "dev@acme.test")
	mod := moderation.New(moderation.NewQueriesRepository(db.New(f.pool), f.pool, enrich.Version))
	// Prepared for acme, but by the time it runs the author acts for globex.
	if _, err := f.pool.Exec(ctx, `DELETE FROM company_account_members WHERE user_id = $1`, member); err != nil {
		t.Fatalf("leave acme: %v", err)
	}
	if _, err := f.repo.InsertPending(ctx, member, "globex", "Globex", "dev@globex.test"); err != nil {
		t.Fatalf("claim globex: %v", err)
	}
	_, _, err := mod.Create(ctx, member, moderation.CreateInput{
		URL: "https://acme.test/jobs/10", Source: "employer", Title: "Go Engineer", Company: "Acme",
		Description: "Build things.", Employer: moderation.EmployerOwner{UserID: member, CompanySlug: "acme"},
	})
	if !errors.Is(err, moderation.ErrNoCompany) {
		t.Errorf("err = %v, want ErrNoCompany", err)
	}
}
