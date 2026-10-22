//go:build integration

// Integration tests for the real write path: internal/ingest/moderation.Service.Create as
// the Minter, plus QueriesRepository's Owner/BySlug/Update/Close — the URL-collision guard
// and the actor-scoped edit/close both rest on real SQL a fake repository cannot exercise.
// Run with: go test -tags=integration ./internal/ingest/employer/
package employer_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/applyform"
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

	if _, _, err := sA.CreateVacancy(ctx, userA, employer.VacancyInput{Description: "Build things.", URL: "https://shared.example/jobs/1", Title: "Go Engineer"}); err != nil {
		t.Fatalf("employer A CreateVacancy: %v", err)
	}

	_, _, err := sB.CreateVacancy(ctx, userB, employer.VacancyInput{Description: "Build things.", URL: "https://shared.example/jobs/1", Title: "Different Title"})
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

	slug, _, err := s.CreateVacancy(ctx, userID, employer.VacancyInput{Description: "Build things.", URL: "https://acme.test/jobs/1", Title: "Go Engineer"})
	if err != nil {
		t.Fatalf("CreateVacancy: %v", err)
	}
	if err := s.CloseVacancy(ctx, userID, slug.Fields().PublicSlug); err != nil {
		t.Fatalf("CloseVacancy: %v", err)
	}

	if _, _, err := s.CreateVacancy(ctx, userID, employer.VacancyInput{Description: "Build things.", URL: "https://acme.test/jobs/1", Title: "Go Engineer, take 2"}); err != nil {
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

	created, _, err := sA.CreateVacancy(ctx, userA, employer.VacancyInput{Description: "Build things.", URL: "https://acme.test/jobs/1", Title: "Go Engineer"})
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

// An on-site vacancy is a real row with no URL, a minted identity and a stored freehire
// form; only its own employer can replace the questions.
func TestOnSiteVacancy_StoredWithItsQuestionsAndScopedToItsEmployer(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('acme', 'Acme', '{"website":"acme.test"}'), ('bravo', 'Bravo', '{"website":"other.test"}')`); err != nil {
		t.Fatalf("seed companies: %v", err)
	}
	sA, userA := liveEmployer(t, pool, "Acme", "founder@acme.test")
	sB, userB := liveEmployer(t, pool, "Bravo", "founder@other.test")

	created, _, err := sA.CreateVacancy(ctx, userA, employer.VacancyInput{Description: "Build things.", ApplyMode: employer.ApplyFreehire,
		Title:     "Go Engineer",
		Questions: []applyform.EmployerQuestion{{Label: "EU work permit?", Type: applyform.TypeBoolean, Required: true}},
	})
	if err != nil {
		t.Fatalf("CreateVacancy: %v", err)
	}
	slug := created.Fields().PublicSlug

	var url, externalID, provider string
	var payload []byte
	if err := pool.QueryRow(ctx, `
		SELECT j.url, j.external_id, f.provider, f.payload
		FROM jobs j JOIN apply_forms f ON f.job_id = j.id
		WHERE j.public_slug = $1`, slug).Scan(&url, &externalID, &provider, &payload); err != nil {
		t.Fatalf("read vacancy and form: %v", err)
	}
	if url != "" || !strings.HasPrefix(externalID, "freehire:") || provider != applyform.ProviderFreehire {
		t.Fatalf("url/external_id/provider = %q/%q/%q", url, externalID, provider)
	}

	replaced := []applyform.EmployerQuestion{{Label: "Notice period?", Type: applyform.TypeText}}
	if _, _, err := sB.UpdateVacancy(ctx, userB, slug, employer.VacancyPatch{Questions: &replaced}); !errors.Is(err, employer.ErrJobNotFound) {
		t.Fatalf("cross-employer question edit err = %v, want ErrJobNotFound", err)
	}
	if _, _, err := sA.UpdateVacancy(ctx, userA, slug, employer.VacancyPatch{Questions: &replaced}); err != nil {
		t.Fatalf("owner question edit: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT f.payload FROM apply_forms f JOIN jobs j ON j.id = f.job_id WHERE j.public_slug = $1`, slug).Scan(&payload); err != nil {
		t.Fatalf("read form: %v", err)
	}
	var form applyform.Form
	if err := json.Unmarshal(payload, &form); err != nil {
		t.Fatalf("decode form: %v", err)
	}
	if len(form.Fields) != 1 || form.Fields[0].Label != "Notice period?" {
		t.Errorf("stored form = %+v, want the owner's replacement", form)
	}
}

func TestCreateVacancy_ExternalStillRequiresAURL(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('acme', 'Acme', '{"website":"acme.test"}')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	s, user := liveEmployer(t, pool, "Acme", "founder@acme.test")
	if _, _, err := s.CreateVacancy(ctx, user, employer.VacancyInput{Description: "Build things.", ApplyMode: employer.ApplyExternal, Title: "Go Engineer"}); !errors.Is(err, employer.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

// Reopening undoes only the employer's own close, puts the vacancy back in the search queue,
// and drops the pending removal its close queued — otherwise the drain would delete the
// reopened row from search after re-adding it.
func TestReopenVacancy_UndoesOnlyTheEmployersOwnClose(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name, company_info) VALUES ('acme', 'Acme', '{"website":"acme.test"}')`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	s, user := liveEmployer(t, pool, "Acme", "founder@acme.test")
	created, _, err := s.CreateVacancy(ctx, user, employer.VacancyInput{ApplyMode: employer.ApplyFreehire, Title: "Go Engineer", Description: "Build things."})
	if err != nil {
		t.Fatalf("CreateVacancy: %v", err)
	}
	slug := created.Fields().PublicSlug
	if err := s.CloseVacancy(ctx, user, slug); err != nil {
		t.Fatalf("CloseVacancy: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM search_outbox`); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}

	if err := s.ReopenVacancy(ctx, user, slug); err != nil {
		t.Fatalf("ReopenVacancy: %v", err)
	}
	var open bool
	var queued, deletes int
	if err := pool.QueryRow(ctx, `
		SELECT j.closed_at IS NULL,
		       (SELECT count(*) FROM search_outbox o WHERE o.job_id = j.id),
		       (SELECT count(*) FROM search_delete_outbox d WHERE d.job_id = j.id)
		FROM jobs j WHERE j.public_slug = $1`, slug).Scan(&open, &queued, &deletes); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !open || queued != 1 || deletes != 0 {
		t.Errorf("open/queued/pending deletes = %v/%d/%d, want true/1/0", open, queued, deletes)
	}

	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now(), closed_reason = 'moderated' WHERE public_slug = $1`, slug); err != nil {
		t.Fatalf("moderator close: %v", err)
	}
	if err := s.ReopenVacancy(ctx, user, slug); !errors.Is(err, employer.ErrInvalid) {
		t.Errorf("reopening a moderator's close: err = %v, want ErrInvalid", err)
	}
}
