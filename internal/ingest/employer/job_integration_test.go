//go:build integration

// Integration tests for the real write path: internal/ingest/moderation.Service.Create as
// the Minter, plus QueriesRepository's Owner/BySlug/Update/Close — the URL-collision guard
// and the actor-scoped edit/close both rest on real SQL a fake repository cannot exercise.
// Run with: go test -tags=integration ./internal/ingest/employer/
package employer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// liveEmployer builds a real Service (moderation.Service as Minter, the real
// QueriesRepository for both accounts and jobs) and an already-active account for a fresh
// user claiming companyName, so tests can go straight to CreateVacancy/UpdateVacancy/
// CloseVacancy. companyName's website is seeded first so the claim auto-activates.
func liveEmployer(t *testing.T, pool *pgxpool.Pool, companyName, workEmail string) (*employer.Service, int64) {
	t.Helper()
	ctx := context.Background()
	q := db.New(pool)

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, workEmail).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	repo := employer.NewQueriesRepository(q, pool)
	minter := moderation.New(moderation.NewQueriesRepository(q, pool, 1))
	s := employer.New(repo, liveCodes{pool: pool}, liveMailer{}, repo, minter)

	if _, err := s.Claim(ctx, userID, companyName, workEmail); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(ctx, userID, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}
	return s, userID
}

// liveCodes is a trivial code issuer for these tests: it always "mails" a fixed code and
// accepts exactly that code back, so the tests exercise the real DB paths without pulling in
// accounts.Service's own storage (already covered by internal/identity/accounts' own tests).
type liveCodes struct{ pool *pgxpool.Pool }

func (liveCodes) IssueCode(ctx context.Context, _ int64, _, email string, send func(ctx context.Context, email, code string) error) error {
	return send(ctx, email, "654321")
}
func (liveCodes) ConfirmCode(_ context.Context, _ int64, _, code string) error {
	if code != "654321" {
		return errors.New("invalid code")
	}
	return nil
}

type liveMailer struct{}

func (liveMailer) SendClaimVerificationCode(context.Context, string, string) error { return nil }

func TestCreateVacancy_URLCollisionAcrossTwoRealEmployerAccounts(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('acme', 'Acme', '{"website":"acme.test"}')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	sA, userA := liveEmployer(t, pool, "Acme", "founder@acme.test")

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('bravo', 'Bravo', '{"website":"other.test"}')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	sB, userB := liveEmployer(t, pool, "Bravo", "founder@other.test")

	if _, _, err := sA.CreateVacancy(ctx, userA, employer.VacancyInput{
		URL: "https://shared.example/jobs/1", Title: "Go Engineer",
	}); err != nil {
		t.Fatalf("employer A CreateVacancy: %v", err)
	}

	_, _, err := sB.CreateVacancy(ctx, userB, employer.VacancyInput{
		URL: "https://shared.example/jobs/1", Title: "Different Title",
	})
	if !errors.Is(err, employer.ErrURLTaken) {
		t.Fatalf("err = %v, want ErrURLTaken", err)
	}

	var title string
	if err := pool.QueryRow(ctx, `SELECT title FROM jobs WHERE source = 'employer' AND external_id = $1`,
		"https://shared.example/jobs/1").Scan(&title); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if title != "Go Engineer" {
		t.Errorf("title = %q, want employer A's original — B's attempt must not have touched it", title)
	}
}

func TestCreateVacancy_SameOwnerReCreateReopensAClosedVacancy(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('acme', 'Acme', '{"website":"acme.test"}')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	s, userID := liveEmployer(t, pool, "Acme", "founder@acme.test")

	slug, _, err := s.CreateVacancy(ctx, userID, employer.VacancyInput{URL: "https://acme.test/jobs/1", Title: "Go Engineer"})
	if err != nil {
		t.Fatalf("CreateVacancy: %v", err)
	}
	if err := s.CloseVacancy(ctx, userID, slug.Fields().PublicSlug); err != nil {
		t.Fatalf("CloseVacancy: %v", err)
	}

	if _, _, err := s.CreateVacancy(ctx, userID, employer.VacancyInput{URL: "https://acme.test/jobs/1", Title: "Go Engineer, take 2"}); err != nil {
		t.Fatalf("re-Create: %v", err)
	}

	var closedAt *string
	if err := pool.QueryRow(ctx, `SELECT closed_at::text FROM jobs WHERE public_slug = $1`, slug.Fields().PublicSlug).Scan(&closedAt); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if closedAt != nil {
		t.Error("re-creating under the same URL must reopen the vacancy (closed_at cleared)")
	}
}

func TestUpdateAndCloseVacancy_ScopedToTheOwningEmployer(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('acme', 'Acme', '{"website":"acme.test"}')`); err != nil {
		t.Fatalf("seed company acme: %v", err)
	}
	sA, userA := liveEmployer(t, pool, "Acme", "founder@acme.test")
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('bravo', 'Bravo', '{"website":"other.test"}')`); err != nil {
		t.Fatalf("seed company bravo: %v", err)
	}
	sB, userB := liveEmployer(t, pool, "Bravo", "founder@other.test")

	created, _, err := sA.CreateVacancy(ctx, userA, employer.VacancyInput{URL: "https://acme.test/jobs/1", Title: "Go Engineer"})
	if err != nil {
		t.Fatalf("CreateVacancy: %v", err)
	}
	slug := created.Fields().PublicSlug

	newTitle := "Hijacked"
	if _, _, err := sB.UpdateVacancy(ctx, userB, slug, employer.VacancyPatch{Title: &newTitle}); !errors.Is(err, employer.ErrJobNotFound) {
		t.Fatalf("cross-employer Update err = %v, want ErrJobNotFound", err)
	}
	if err := sB.CloseVacancy(ctx, userB, slug); !errors.Is(err, employer.ErrJobNotFound) {
		t.Fatalf("cross-employer Close err = %v, want ErrJobNotFound", err)
	}

	realTitle := "Senior Go Engineer"
	if _, _, err := sA.UpdateVacancy(ctx, userA, slug, employer.VacancyPatch{Title: &realTitle}); err != nil {
		t.Fatalf("owner Update: %v", err)
	}
	if err := sA.CloseVacancy(ctx, userA, slug); err != nil {
		t.Fatalf("owner Close: %v", err)
	}

	var title, closedReason string
	if err := pool.QueryRow(ctx, `SELECT title, closed_reason FROM jobs WHERE public_slug = $1`, slug).Scan(&title, &closedReason); err != nil {
		t.Fatalf("read job: %v", err)
	}
	if title != realTitle {
		t.Errorf("title = %q, want %q", title, realTitle)
	}
	if closedReason != "employer_closed" {
		t.Errorf("closed_reason = %q, want employer_closed", closedReason)
	}
}
