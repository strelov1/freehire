// Package companyfeedback owns a signed-in user's 1-5 star rating + a closed
// category + free text about a company — shown under their site-wide
// pseudonymous persona (internal/community's handle, not a real name), one row
// per (user, company), editable by resubmitting (upsert-in-place, the same
// shape internal/vote's company thumbs use).
//
// A feedback write and the affected company's counter recompute run in one pgx
// transaction, so a reader never sees a rating without its average.
package companyfeedback

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/vocab"
)

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
	// ErrNotFound is a caller with no feedback on the given company (404).
	ErrNotFound = errors.New("companyfeedback: not found")
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

// PersonaSource resolves a user's stable pseudonymous handle, minting one on
// first use — the same site-wide persona internal/community's discussion
// threads use, so a user's pseudonym stays consistent everywhere it's shown
// rather than a second identity being minted here.
type PersonaSource interface {
	PersonaFor(ctx context.Context, userID int64) (string, error)
}

// Service applies feedback writes against Postgres.
type Service struct {
	q        *db.Queries
	pool     *pgxpool.Pool
	personas PersonaSource
}

// New builds a Service over the shared queries, pool (drives the per-write
// transaction), and persona source.
func New(q *db.Queries, pool *pgxpool.Pool, personas PersonaSource) *Service {
	return &Service{q: q, pool: pool, personas: personas}
}

// validFeedbackType reports whether t is one of the closed category values.
func validFeedbackType(t string) bool {
	for _, v := range vocab.CompanyFeedbackTypeValues {
		if v == t {
			return true
		}
	}
	return false
}

// Upsert validates and writes the caller's feedback on a company (inserting or
// overwriting their existing one in place), minting their persona if needed,
// and recomputes the company's materialized counters in the same transaction.
func (s *Service) Upsert(ctx context.Context, userID int64, slug string, rating int16, feedbackType, body string) (Feedback, error) {
	if rating < 1 || rating > 5 {
		return Feedback{}, ErrInvalidRating
	}
	if !validFeedbackType(feedbackType) {
		return Feedback{}, ErrInvalidFeedbackType
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Feedback{}, ErrEmptyBody
	}
	if err := s.requireCompany(ctx, slug); err != nil {
		return Feedback{}, err
	}

	handle, err := s.personas.PersonaFor(ctx, userID)
	if err != nil {
		return Feedback{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Feedback{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// Take the company row's lock so a concurrent feedback write on the same
	// company serializes with the recompute below — same reasoning as
	// LockCompanyForVote, reused here rather than duplicated.
	if err := q.LockCompanyForVote(ctx, slug); err != nil {
		return Feedback{}, err
	}
	row, err := q.UpsertCompanyFeedback(ctx, db.UpsertCompanyFeedbackParams{
		UserID: pgtype.Int8{Int64: userID, Valid: true}, CompanySlug: slug,
		Rating: rating, FeedbackType: feedbackType, Body: body,
	})
	if err != nil {
		return Feedback{}, err
	}
	if _, err := q.RecountCompanyFeedback(ctx, slug); err != nil {
		return Feedback{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Feedback{}, err
	}
	return feedbackFromRow(row, handle), nil
}

// Delete removes the caller's own feedback (no-op when absent) and recomputes
// the company's materialized counters in the same transaction.
func (s *Service) Delete(ctx context.Context, userID int64, slug string) error {
	if err := s.requireCompany(ctx, slug); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	if err := q.LockCompanyForVote(ctx, slug); err != nil {
		return err
	}
	if err := q.DeleteCompanyFeedback(ctx, db.DeleteCompanyFeedbackParams{
		UserID: pgtype.Int8{Int64: userID, Valid: true}, CompanySlug: slug,
	}); err != nil {
		return err
	}
	if _, err := q.RecountCompanyFeedback(ctx, slug); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Mine returns the caller's own feedback on a company (for the edit form's
// prefill), or ErrNotFound when they have not left one yet. AuthorHandle is
// left blank — the caller already knows who they are, so it is not worth a
// second round trip through PersonaSource just to fill in their own handle.
func (s *Service) Mine(ctx context.Context, userID int64, slug string) (Feedback, error) {
	row, err := s.q.GetMyCompanyFeedback(ctx, db.GetMyCompanyFeedbackParams{
		UserID: pgtype.Int8{Int64: userID, Valid: true}, CompanySlug: slug,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Feedback{}, ErrNotFound
		}
		return Feedback{}, err
	}
	return feedbackFromRow(row, ""), nil
}

// List returns a company's feedback, newest first, offset-paginated.
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

// Count returns how many feedback entries a company has.
func (s *Service) Count(ctx context.Context, slug string) (int64, error) {
	if err := s.requireCompany(ctx, slug); err != nil {
		return 0, err
	}
	return s.q.CountCompanyFeedback(ctx, slug)
}

// requireCompany reports ErrCompanyNotFound for a slug naming no company — the
// cheap existence check that returns a clean 404 before touching
// company_feedback, whose FK would otherwise surface a bad slug as an opaque
// error (same reasoning as vote.Service.requireCompany).
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

// feedbackFromRow projects a plain company_feedback row onto Feedback, given
// the author handle resolved separately (the row itself never carries it).
func feedbackFromRow(row db.CompanyFeedback, handle string) Feedback {
	return Feedback{
		ID: row.ID, CompanySlug: row.CompanySlug, AuthorHandle: handle,
		Rating: row.Rating, FeedbackType: row.FeedbackType, Body: row.Body,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}
