// Package companyfeedback owns a signed-in user's 1-5 star rating, closed
// category and free text about a company, shown under their site-wide
// pseudonymous persona (internal/engage/community's handle). One row per
// (user, company, category); resubmitting within a category edits in place.
//
// A write and the company's counter recompute run in one transaction. A
// moderator can Hide a review once reports come in; internal/engage/report has
// the fuller resolve/dismiss version of that pattern.
package companyfeedback

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// maxBodyRunes bounds a review's free text, in the same spirit as
// internal/engage/report's maxDetailsLen.
const maxBodyRunes = 5000

// Sentinel errors, mapped to HTTP statuses by the handler.
var (
	// ErrCompanyNotFound is a slug that names no existing company (404).
	ErrCompanyNotFound = errors.New("companyfeedback: company not found")
	// ErrInvalidRating is a rating outside 1-5 (400).
	ErrInvalidRating = errors.New("companyfeedback: rating must be between 1 and 5")
	// ErrInvalidFeedbackType is a feedback_type outside vocab.CompanyFeedbackTypeValues (400).
	ErrInvalidFeedbackType = errors.New("companyfeedback: invalid feedback type")
	// ErrEmptyBody is feedback submitted with no body text (422).
	ErrEmptyBody = errors.New("companyfeedback: body is required")
	// ErrBodyTooLong is feedback text over maxBodyRunes (422).
	ErrBodyTooLong = errors.New("companyfeedback: body is too long")
	// ErrNotFound is a review id naming no row (404).
	ErrNotFound = errors.New("companyfeedback: not found")
	// ErrInvalidReportReason is a report reason outside
	// vocab.CompanyFeedbackReportReasonValues (400).
	ErrInvalidReportReason = errors.New("companyfeedback: invalid report reason")
)

// Feedback is one user's rating + category + text about a company, carrying
// their persona handle — never their user id.
type Feedback struct {
	ID           int64
	CompanySlug  string
	AuthorHandle string
	Rating       int16
	FeedbackType string
	Body         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ReportedFeedback is a review with at least one report against it, plus the
// aggregated report info a moderator triages by — the moderator queue's shape.
type ReportedFeedback struct {
	Feedback
	ReportCount   int64
	ReportReasons []string
}

// Summary is a company's recomputed feedback counters, returned with a write
// so the caller can update its view without a second round trip.
type Summary struct {
	Count     int32
	RatingAvg *float32
}

// PersonaSource resolves a user's stable pseudonymous handle, minting one on
// first use. It is the same site-wide persona internal/engage/community uses.
type PersonaSource interface {
	PersonaFor(ctx context.Context, userID int64) (string, error)
}

// Service applies feedback writes against Postgres.
type Service struct {
	q        *db.Queries
	pool     *pgxpool.Pool
	personas PersonaSource
}

// New builds a Service over shared queries, transactions, and the persona source.
func New(q *db.Queries, pool *pgxpool.Pool, personas PersonaSource) *Service {
	return &Service{q: q, pool: pool, personas: personas}
}

// Upsert validates and writes the caller's feedback in one category on a
// company (insert or overwrite in place), minting their persona if needed, and
// recomputes the company's counters in the same transaction.
func (s *Service) Upsert(ctx context.Context, userID int64, slug string, rating int16, feedbackType, body string) (Feedback, Summary, error) {
	if rating < 1 || rating > 5 {
		return Feedback{}, Summary{}, ErrInvalidRating
	}
	if !slices.Contains(vocab.CompanyFeedbackTypeValues, feedbackType) {
		return Feedback{}, Summary{}, ErrInvalidFeedbackType
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Feedback{}, Summary{}, ErrEmptyBody
	}
	if utf8.RuneCountInString(body) > maxBodyRunes {
		return Feedback{}, Summary{}, ErrBodyTooLong
	}
	if err := s.requireCompany(ctx, slug); err != nil {
		return Feedback{}, Summary{}, err
	}

	handle, err := s.personas.PersonaFor(ctx, userID)
	if err != nil {
		return Feedback{}, Summary{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Feedback{}, Summary{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// Lock the company row so concurrent writes serialize with the recompute
	// below (same as LockCompanyForVote).
	if err := q.LockCompanyForVote(ctx, slug); err != nil {
		return Feedback{}, Summary{}, err
	}
	row, err := q.UpsertCompanyFeedback(ctx, db.UpsertCompanyFeedbackParams{
		UserID: pgtype.Int8{Int64: userID, Valid: true}, CompanySlug: slug,
		Rating: rating, FeedbackType: feedbackType, Body: body,
	})
	if err != nil {
		return Feedback{}, Summary{}, err
	}
	counts, err := q.RecountCompanyFeedback(ctx, slug)
	if err != nil {
		return Feedback{}, Summary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Feedback{}, Summary{}, err
	}
	return feedbackFromRow(row, handle), summaryFromCounts(counts), nil
}

// Delete removes the caller's own feedback in one category on a company
// (no-op when absent) and recomputes the company's materialized counters in
// the same transaction.
func (s *Service) Delete(ctx context.Context, userID int64, slug, feedbackType string) (Summary, error) {
	if !slices.Contains(vocab.CompanyFeedbackTypeValues, feedbackType) {
		return Summary{}, ErrInvalidFeedbackType
	}
	if err := s.requireCompany(ctx, slug); err != nil {
		return Summary{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	if err := q.LockCompanyForVote(ctx, slug); err != nil {
		return Summary{}, err
	}
	if err := q.DeleteCompanyFeedback(ctx, db.DeleteCompanyFeedbackParams{
		UserID: pgtype.Int8{Int64: userID, Valid: true}, CompanySlug: slug, FeedbackType: feedbackType,
	}); err != nil {
		return Summary{}, err
	}
	counts, err := q.RecountCompanyFeedback(ctx, slug)
	if err != nil {
		return Summary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Summary{}, err
	}
	return summaryFromCounts(counts), nil
}

// Mine returns all of the caller's own feedback on a company, across every
// category (possibly empty). The write dialog uses it to tell an edit from a
// new review. ErrCompanyNotFound for a bad slug, checked first.
func (s *Service) Mine(ctx context.Context, userID int64, slug string) ([]Feedback, error) {
	if err := s.requireCompany(ctx, slug); err != nil {
		return nil, err
	}
	rows, err := s.q.ListMyCompanyFeedback(ctx, db.ListMyCompanyFeedbackParams{
		UserID: pgtype.Int8{Int64: userID, Valid: true}, CompanySlug: slug,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Feedback{}, nil
	}
	// Resolve the real handle here: personaOrDeleted would render the caller's own
	// live reviews as "[deleted]".
	handle, err := s.personas.PersonaFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Feedback, len(rows))
	for i, row := range rows {
		out[i] = feedbackFromRow(row, handle)
	}
	return out, nil
}

// List returns a company's feedback, newest first, offset-paginated. Hidden
// reviews (Hide) are excluded.
func (s *Service) List(ctx context.Context, slug string, limit, offset int32) ([]Feedback, error) {
	if err := s.requireCompany(ctx, slug); err != nil {
		return nil, err
	}
	rows, err := s.q.ListCompanyFeedback(ctx, db.ListCompanyFeedbackParams{
		CompanySlug: slug, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Feedback, len(rows))
	for i, row := range rows {
		out[i] = Feedback{
			ID: row.ID, CompanySlug: row.CompanySlug, AuthorHandle: pgconv.TextString(row.AuthorHandle),
			Rating: row.Rating, FeedbackType: row.FeedbackType, Body: row.Body,
			CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		}
	}
	return out, nil
}

// Count returns how many (visible) feedback entries a company has.
func (s *Service) Count(ctx context.Context, slug string) (int64, error) {
	if err := s.requireCompany(ctx, slug); err != nil {
		return 0, err
	}
	return s.q.CountCompanyFeedback(ctx, slug)
}

// Report files a complaint against a review, as evidence for a moderator to
// act on via Hide (no per-report resolve/dismiss state; see
// internal/engage/report). A repeat report by the same user is a silent no-op.
func (s *Service) Report(ctx context.Context, userID, feedbackID int64, reason string) error {
	if !slices.Contains(vocab.CompanyFeedbackReportReasonValues, reason) {
		return ErrInvalidReportReason
	}
	ok, err := s.q.CompanyFeedbackExists(ctx, feedbackID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return s.q.InsertCompanyFeedbackReport(ctx, db.InsertCompanyFeedbackReportParams{
		FeedbackID: feedbackID, ReporterUserID: pgtype.Int8{Int64: userID, Valid: true}, Reason: reason,
	})
}

// Hide is the moderator lever: it marks a review hidden and recomputes the
// company's counters in the same transaction. Idempotent; ErrNotFound for an
// unknown id.
func (s *Service) Hide(ctx context.Context, feedbackID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// Read the slug WITHOUT locking the feedback row so the company row is locked
	// first, the order Upsert/Delete use. Locking the feedback row first (as
	// HideCompanyFeedback's UPDATE would) risks a deadlock against them.
	slug, err := q.GetCompanyFeedbackSlug(ctx, feedbackID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := q.LockCompanyForVote(ctx, slug); err != nil {
		return err
	}
	if _, err := q.HideCompanyFeedback(ctx, feedbackID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Deleted between the read above and this update.
			return ErrNotFound
		}
		return err
	}
	if _, err := q.RecountCompanyFeedback(ctx, slug); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListReported returns every review with at least one report, most-reported
// first — the moderator queue's read; Hide is the corresponding write.
func (s *Service) ListReported(ctx context.Context) ([]ReportedFeedback, error) {
	rows, err := s.q.ListReportedCompanyFeedback(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ReportedFeedback, len(rows))
	for i, row := range rows {
		out[i] = ReportedFeedback{
			Feedback: Feedback{
				ID: row.ID, CompanySlug: row.CompanySlug, AuthorHandle: pgconv.TextString(row.AuthorHandle),
				Rating: row.Rating, FeedbackType: row.FeedbackType, Body: row.Body,
				CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
			},
			ReportCount:   row.ReportCount,
			ReportReasons: row.ReportReasons,
		}
	}
	return out, nil
}

// requireCompany reports ErrCompanyNotFound for an unknown slug, giving a clean
// 404 instead of an opaque FK error from company_feedback.
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

// feedbackFromRow projects a company_feedback row onto Feedback; the handle is
// resolved separately.
func feedbackFromRow(row db.CompanyFeedback, handle string) Feedback {
	return Feedback{
		ID: row.ID, CompanySlug: row.CompanySlug, AuthorHandle: handle,
		Rating: row.Rating, FeedbackType: row.FeedbackType, Body: row.Body,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

// summaryFromCounts adapts RecountCompanyFeedback's return row to Summary.
func summaryFromCounts(row db.RecountCompanyFeedbackRow) Summary {
	return Summary{Count: row.FeedbackCount, RatingAvg: pgconv.Float4Ptr(row.FeedbackRatingAvg)}
}
