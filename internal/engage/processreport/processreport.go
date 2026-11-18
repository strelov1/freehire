// Package processreport owns candidate-reported facts about how a company hires. Today
// there is one: the employer screens with an AI interviewer.
//
// It is neither internal/engage/report (a complaint about a POSTING, whose only lever is
// closing it) nor internal/engage/companyfeedback (a 1-5 star REVIEW, unique per (user,
// company, category), so a user who reviewed compensation could never report the
// interviewer).
//
// It stores a countable claim: one row per (user, company, kind), no rating, no text. A
// claim is either true of an employer or not, so ONE reporter is enough to surface it; the
// count travels with the label so a reader can weigh one report against forty.
//
// The evidence is COMPANY-scoped although filed from a job page: an AI interviewer is a
// property of the employer's process, and per posting it would never reach useful coverage.
//
// A write and the company's counter recompute run in one pgx transaction, so a reader never
// sees the label without the count that qualifies it.
package processreport

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/db"
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
)

// Service files and withdraws process reports and keeps the company's counter in step.
type Service struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// New builds the service. The pool is needed as well as the queries because every
// write runs in a transaction with its recompute.
func New(q *db.Queries, pool *pgxpool.Pool) *Service {
	return &Service{q: q, pool: pool}
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

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// Lock the company row so a concurrent report on it serializes with the recompute below
	// (the same call companyfeedback makes).
	if err := q.LockCompanyForVote(ctx, slug); err != nil {
		return 0, err
	}

	if _, err := q.FileCompanyProcessReport(ctx, db.FileCompanyProcessReportParams{
		UserID: userID, CompanySlug: slug, Kind: kind,
	}); err != nil {
		// ON CONFLICT is guarded on retracted_at IS NOT NULL, so an already-live report returns
		// no row: that absence is the duplicate answer.
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrAlreadyReported
		}
		return 0, err
	}

	count, err := q.RecountCompanyProcessReports(ctx, slug)
	if err != nil {
		return 0, err
	}
	// Copy the count onto the company's postings in the same transaction, so a job
	// card never disagrees with the company page about a fact filed a second ago.
	if err := q.SyncJobsAIInterviewReports(ctx, db.SyncJobsAIInterviewReportsParams{
		CompanySlug: slug, AiInterviewReports: count,
	}); err != nil {
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
	// Copy the count onto the company's postings in the same transaction, so a job
	// card never disagrees with the company page about a fact filed a second ago.
	if err := q.SyncJobsAIInterviewReports(ctx, db.SyncJobsAIInterviewReportsParams{
		CompanySlug: slug, AiInterviewReports: count,
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

// Mine returns the kinds the caller currently has live against a company (possibly
// empty), so the write surface opens in the right state. ErrCompanyNotFound for a bad slug,
// checked first.
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

// requireCompany returns ErrCompanyNotFound for an unknown slug, for a clean 404 before
// the transaction opens (the FK would otherwise surface an opaque constraint violation).
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
