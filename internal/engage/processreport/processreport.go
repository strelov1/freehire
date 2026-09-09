// Package processreport owns candidate-reported facts about how a company hires.
// Today there is exactly one: that the employer screens with an AI interviewer.
//
// It differs from its two neighbours in what it stores, and the differences are the
// reason it is not either of them. internal/engage/report files a complaint about a
// POSTING and hands it to a moderator whose only lever is closing that posting —
// which answers "a bot interviewed me" with something nobody asked for.
// internal/engage/companyfeedback files a 1-5 star REVIEW with free text, one row per
// (user, company, category) — a star forces a judgement where this needs a fact, and
// its uniqueness bound would mean a user who already reviewed the compensation could
// never report the interviewer.
//
// What this stores is a countable claim: one row per (user, company, kind), no rating,
// no text, nothing to elaborate. Because a claim is either true of an employer or not,
// ONE reporter is enough to surface it — unlike the ghost signal's contributor gate,
// which exists because one person's silence may be bad luck rather than evidence. The
// count travels with the label everywhere it is shown, so a reader can weigh one
// report against forty.
//
// The evidence is COMPANY-scoped although it is filed from a job page. An AI
// interviewer is a property of the employer's process; filed per posting the signal
// would never reach useful coverage, since one employer in this catalogue has 203 open
// postings and each would need its own reporter before its own card showed anything.
//
// A write and the affected company's counter recompute run in one pgx transaction, so
// a reader never sees the label without the count that qualifies it.
package processreport

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/db"
)

// DefaultReportWindow/DefaultReportCap bound how many reports one user can file per
// window. Filing is cheap and honest reporting should never hit this — the cap exists
// so one account cannot label the catalogue. It is deliberately looser than
// companyfeedback's, because a report carries no text to moderate.
const (
	DefaultReportWindow = 24 * time.Hour
	DefaultReportCap    = 30
)

// Sentinel errors, mapped to HTTP statuses by the handler.
var (
	// ErrCompanyNotFound is a slug that names no existing company (404).
	ErrCompanyNotFound = errors.New("processreport: company not found")
	// ErrInvalidKind is a kind outside vocab.CompanyProcessReportKindValues (400).
	ErrInvalidKind = errors.New("processreport: invalid kind")
	// ErrAlreadyReported is a second live report of the same kind by the same user (409).
	ErrAlreadyReported = errors.New("processreport: already reported")
	// ErrNotFound is a retraction of a report the user never filed, or already
	// withdrew (404).
	ErrNotFound = errors.New("processreport: not found")
	// ErrRateLimited is a user over their window cap (429).
	ErrRateLimited = errors.New("processreport: rate limit exceeded")
)

// Config bounds one user's filing rate. A zero value takes the defaults.
type Config struct {
	Window time.Duration
	Cap    int
}

func (c Config) window() time.Duration {
	if c.Window <= 0 {
		return DefaultReportWindow
	}
	return c.Window
}

func (c Config) cap() int {
	if c.Cap <= 0 {
		return DefaultReportCap
	}
	return c.Cap
}

// Service files and withdraws process reports and keeps the company's counter in step.
type Service struct {
	q    *db.Queries
	pool *pgxpool.Pool
	cfg  Config
}

// New builds the service. The pool is needed as well as the queries because every
// write runs in a transaction with its recompute.
func New(q *db.Queries, pool *pgxpool.Pool, cfg Config) *Service {
	return &Service{q: q, pool: pool, cfg: cfg}
}

// ValidKind reports whether kind is in the controlled vocabulary. Exported because the
// handler rejects an unknown kind before it opens a transaction, and the search and
// badge layers ask the same question of a stored value.
func ValidKind(kind string) bool {
	return slices.Contains(vocab.CompanyProcessReportKindValues, kind)
}

// File records that the caller met this practice at this company, and returns the
// company's resulting count for that kind.
//
// A second live report by the same user is ErrAlreadyReported rather than a silent
// no-op: the caller asked to file and deserves to know their report already stands.
// A report the caller previously withdrew is REVIVED in place — the row is reused, so
// the uniqueness bound holds and a retraction can never become a way to file twice.
func (s *Service) File(ctx context.Context, userID int64, slug, kind string) (int32, error) {
	if !ValidKind(kind) {
		return 0, ErrInvalidKind
	}
	if err := s.requireCompany(ctx, slug); err != nil {
		return 0, err
	}
	if err := s.checkRate(ctx, userID); err != nil {
		return 0, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// Take the company row's lock so a concurrent report on the same company
	// serializes with the recompute below — LockCompanyForVote locks the companies
	// row and knows nothing about votes, so it is reused rather than duplicated, the
	// same call companyfeedback makes.
	if err := q.LockCompanyForVote(ctx, slug); err != nil {
		return 0, err
	}

	if _, err := q.FileCompanyProcessReport(ctx, db.FileCompanyProcessReportParams{
		UserID: userID, CompanySlug: slug, Kind: kind,
	}); err != nil {
		// The ON CONFLICT branch is guarded on retracted_at IS NOT NULL, so an
		// already-live report updates nothing and returns no row. That absence IS the
		// duplicate answer; it costs no second read.
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrAlreadyReported
		}
		return 0, err
	}

	count, err := q.RecountCompanyProcessReports(ctx, slug)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

// Retract withdraws the caller's own report and returns the company's resulting count.
// The row is kept and stamped, never deleted: withdrawing is how the signal self-heals
// when an employer changes practice, and the surviving row preserves the uniqueness
// bound.
func (s *Service) Retract(ctx context.Context, userID int64, slug, kind string) (int32, error) {
	if !ValidKind(kind) {
		return 0, ErrInvalidKind
	}
	if err := s.requireCompany(ctx, slug); err != nil {
		return 0, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	if err := q.LockCompanyForVote(ctx, slug); err != nil {
		return 0, err
	}

	if _, err := q.RetractCompanyProcessReport(ctx, db.RetractCompanyProcessReportParams{
		UserID: userID, CompanySlug: slug, Kind: kind,
	}); err != nil {
		// Guarded on retracted_at IS NULL, so withdrawing twice returns no row rather
		// than restamping — the caller is told there was nothing to withdraw.
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}

	count, err := q.RecountCompanyProcessReports(ctx, slug)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

// Mine returns the kinds the caller currently has live against a company (possibly
// empty). The write surface needs it to open in the right state — without it a
// returning reader cannot be shown that they already reported, and the only way to
// find out would be to file and be refused. ErrCompanyNotFound for a bad slug, checked
// first so it is never confused with "you have not reported anything".
func (s *Service) Mine(ctx context.Context, userID int64, slug string) ([]string, error) {
	if err := s.requireCompany(ctx, slug); err != nil {
		return nil, err
	}
	kinds, err := s.q.MyLiveCompanyProcessReportKinds(ctx, db.MyLiveCompanyProcessReportKindsParams{
		UserID: userID, CompanySlug: slug,
	})
	if err != nil {
		return nil, err
	}
	if kinds == nil {
		kinds = []string{}
	}
	return kinds, nil
}

// requireCompany reports ErrCompanyNotFound for a slug naming no company — the cheap
// existence check that returns a clean 404 before the transaction opens, since the FK
// would otherwise surface a bad slug as an opaque constraint violation (the same
// reasoning as companyfeedback.Service.requireCompany and vote's).
func (s *Service) requireCompany(ctx context.Context, slug string) error {
	ok, err := s.q.CompanySlugExists(ctx, slug)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCompanyNotFound
	}
	return nil
}

// checkRate refuses a user who has filed more than the cap allows inside the window.
// A revival takes the ON CONFLICT branch and leaves created_at untouched, so
// retract-and-refile does not spend the allowance twice.
func (s *Service) checkRate(ctx context.Context, userID int64) error {
	since := time.Now().Add(-s.cfg.window())
	n, err := s.q.CountRecentCompanyProcessReports(ctx, db.CountRecentCompanyProcessReportsParams{
		UserID:    userID,
		CreatedAt: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return err
	}
	if n >= int64(s.cfg.cap()) {
		return ErrRateLimited
	}
	return nil
}
