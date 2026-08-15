// Package companyfeedback owns a signed-in user's 1-5 star rating + a closed
// category + free text about a company — shown under their site-wide
// pseudonymous persona (internal/community's handle, not a real name), one row
// per (user, company, category), editable by resubmitting within that category
// (upsert-in-place, the same shape internal/vote's company thumbs use). A user
// may hold at most one review per category on a given company, but may leave a
// second review on the same company under a different category.
//
// A feedback write and the affected company's counter recompute run in one pgx
// transaction, so a reader never sees a rating without its average. A moderator
// can Hide a review once enough Reports come in — a lever, not a ticket queue:
// see internal/report for the fuller resolve/dismiss/notify version of that
// pattern, built for job postings' much higher report volume.
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

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/vocab"
)

// maxBodyRunes bounds a review's free text — the same order of magnitude as
// internal/report's maxDetailsLen, and for the same reason: long enough for a
// thorough account, short enough that one entry can't bloat a company page.
const maxBodyRunes = 5000

// DefaultFeedbackWindow/DefaultFeedbackCap bound how many NEW companies one user
// can review per window — the same shape and magnitude as
// community.DefaultThreadWindow/Cap, since both are "post original content"
// actions. An edit-by-resubmit never counts against this (see checkRate).
const (
	DefaultFeedbackWindow = 24 * time.Hour
	DefaultFeedbackCap    = 10
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
	// ErrBodyTooLong is feedback text over maxBodyRunes (422).
	ErrBodyTooLong = errors.New("companyfeedback: body is too long")
	// ErrNotFound is a review id naming no row (404).
	ErrNotFound = errors.New("companyfeedback: not found")
	// ErrRateLimited is a user over their new-review window cap (429).
	ErrRateLimited = errors.New("companyfeedback: rate limit exceeded")
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

// Summary is a company's recomputed materialized feedback counters, returned
// alongside a write so the caller can update its own view of the company
// without a second round trip — the same shape as internal/vote.Result.
type Summary struct {
	Count     int32
	RatingAvg *float32
}

// PersonaSource resolves a user's stable pseudonymous handle, minting one on
// first use — the same site-wide persona internal/community's discussion
// threads use, so a user's pseudonym stays consistent everywhere it's shown
// rather than a second identity being minted here.
type PersonaSource interface {
	PersonaFor(ctx context.Context, userID int64) (string, error)
}

// Config tunes the new-review rate limit. Zero values fall back to the
// Default* constants in New.
type Config struct {
	Window time.Duration
	Cap    int
}

// Service applies feedback writes against Postgres.
type Service struct {
	q        *db.Queries
	pool     *pgxpool.Pool
	personas PersonaSource
	cfg      Config
	now      func() time.Time
}

// New builds a Service over the shared queries, pool (drives the per-write
// transaction), and persona source, filling any zero Config field with its
// default.
func New(q *db.Queries, pool *pgxpool.Pool, personas PersonaSource, cfg Config) *Service {
	if cfg.Window == 0 {
		cfg.Window = DefaultFeedbackWindow
	}
	if cfg.Cap == 0 {
		cfg.Cap = DefaultFeedbackCap
	}
	return &Service{q: q, pool: pool, personas: personas, cfg: cfg, now: time.Now}
}

// checkRate rejects a new review once the caller is at or over their window
// cap. An edit-by-resubmit never inserts a company_feedback row (see
// CountRecentCompanyFeedback's doc comment), so the count this reads only
// grows on genuinely new rows — a first review of a company, or a review in a
// category the caller has not used on it yet. Callers only need to invoke it
// when Upsert is about to create, not update, a row.
func (s *Service) checkRate(ctx context.Context, userID int64) error {
	n, err := s.q.CountRecentCompanyFeedback(ctx, db.CountRecentCompanyFeedbackParams{
		UserID:    pgtype.Int8{Int64: userID, Valid: true},
		CreatedAt: pgtype.Timestamptz{Time: s.now().Add(-s.cfg.Window), Valid: true},
	})
	if err != nil {
		return err
	}
	if n >= int64(s.cfg.Cap) {
		return ErrRateLimited
	}
	return nil
}

// Upsert validates and writes the caller's feedback in one category on a
// company (inserting or overwriting their existing entry in that category in
// place), minting their persona if needed, and recomputes the company's
// materialized counters in the same transaction. A brand-new review (the
// caller has none yet in this category on this company) is subject to
// checkRate; editing an existing one never is — see checkRate's doc comment.
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

	_, err := s.q.GetMyCompanyFeedback(ctx, db.GetMyCompanyFeedbackParams{
		UserID: pgtype.Int8{Int64: userID, Valid: true}, CompanySlug: slug, FeedbackType: feedbackType,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// A genuinely new review: throttle it.
		if err := s.checkRate(ctx, userID); err != nil {
			return Feedback{}, Summary{}, err
		}
	} else if err != nil {
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

	// Take the company row's lock so a concurrent feedback write on the same
	// company serializes with the recompute below — same reasoning as
	// LockCompanyForVote, reused here rather than duplicated.
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
// category they've reviewed it under (possibly empty) — the write dialog's
// read for telling an edit from a new review: picking a category the caller
// has already used prefills that entry, any other starts a blank one.
// ErrCompanyNotFound for a bad slug (checked first, so it's never confused
// with "no feedback yet" the way an earlier single-feedback version of this
// method conflated the two).
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
	// Unlike List's author handles (resolved once per page via a join), these
	// rows are always the caller's own — worth the extra lookup to get a real
	// handle instead of leaving it blank, which the handler's shared
	// personaOrDeleted would otherwise render as "[deleted]" for the caller's
	// own, very much live, reviews.
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

// Report files a complaint against a specific review — evidence for a
// moderator to act on via Hide, not a per-report ticket with its own
// resolve/dismiss state (see internal/report for that fuller shape, sized for
// job postings' much higher report volume). A second report of the same
// review by the same user is a silent no-op.
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

// Hide is the moderator lever: mark a review hidden and recompute its
// company's materialized counters in the same transaction, so a hidden review
// stops counting toward the public average and the public list immediately.
// Idempotent; ErrNotFound for an unknown id.
func (s *Service) Hide(ctx context.Context, feedbackID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// Read the company slug WITHOUT locking the feedback row (GetCompanyFeedbackSlug
	// is a plain SELECT), so the company row can be locked BEFORE the feedback row is
	// touched — the same order Upsert/Delete already use. Locking the feedback row
	// first, the way HideCompanyFeedback's UPDATE would on its own, risks a deadlock
	// against those paths' opposite order.
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
			// Deleted between the read above and this update — same outcome as
			// never having existed.
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

// summaryFromCounts adapts RecountCompanyFeedback's return row to Summary.
func summaryFromCounts(row db.RecountCompanyFeedbackRow) Summary {
	return Summary{Count: row.FeedbackCount, RatingAvg: pgconv.Float4Ptr(row.FeedbackRatingAvg)}
}
