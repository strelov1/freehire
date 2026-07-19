package contribution

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository is the production Repository backed by sqlc-generated *db.Queries.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// BoardTracked reports whether the catalogue already crawls this board (any job whose
// external_id is "<board>:…"). It matches with a LIKE-prefix served by the
// (source, external_id text_pattern_ops) index; the board's LIKE metacharacters are escaped
// so a slug with % or _ cannot widen the match.
func (r *QueriesRepository) BoardTracked(ctx context.Context, source, board string) (bool, error) {
	return r.q.JobsExistForBoard(ctx, db.JobsExistForBoardParams{Source: source, BoardPattern: likePrefix(board)})
}

// BoardByGreenhouseJobID returns the greenhouse board already carrying a job with the given
// Greenhouse job id, or ok=false when none is tracked.
func (r *QueriesRepository) BoardByGreenhouseJobID(ctx context.Context, jobID string) (board string, ok bool, err error) {
	board, err = r.q.BoardByGreenhouseJobID(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return board, true, nil
}

// CompanyForBoard returns the company name + slug already tracked on the board, or ok=false
// when the board has no job with a resolved company.
func (r *QueriesRepository) CompanyForBoard(ctx context.Context, source, board string) (name, slug string, ok bool, err error) {
	row, err := r.q.CompanyForBoard(ctx, db.CompanyForBoardParams{Source: source, BoardPattern: likePrefix(board)})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return row.Company, row.CompanySlug, true, nil
}

// likePrefix builds a LIKE pattern matching external_ids on board ("<board>:…"), escaping the
// LIKE metacharacters \ % _ in the (URL-derived) board with the default backslash escape.
func likePrefix(board string) string {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(board)
	return esc + ":%"
}

// Record inserts the contribution. The UNIQUE (source, board) constraint rejects a
// duplicate board (another vacancy or the listing), surfaced as ErrBoardAlreadyContributed;
// the AI-credits reward is granted separately by the handler, keyed by the contribution id.
func (r *QueriesRepository) Record(ctx context.Context, in RecordInput) (Contribution, error) {
	row, err := r.q.CreateContribution(ctx, db.CreateContributionParams{
		SubmittedBy: in.SubmittedBy,
		URL:         in.URL,
		Source:      in.Source,
		Board:       in.Board,
	})
	if err != nil {
		if pgerr.IsUniqueViolation(err) {
			return Contribution{}, ErrBoardAlreadyContributed
		}
		return Contribution{}, err
	}
	return fromRow(row), nil
}

// ListByUser returns one user's contributions, newest first.
func (r *QueriesRepository) ListByUser(ctx context.Context, userID int64) ([]Contribution, error) {
	rows, err := r.q.ListContributionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Contribution, len(rows))
	for i, row := range rows {
		out[i] = fromRow(row)
	}
	return out, nil
}

// fromRow maps the generated db row to the package domain type.
func fromRow(row db.LinkContribution) Contribution {
	return Contribution{
		ID:          row.ID,
		SubmittedBy: row.SubmittedBy,
		URL:         row.URL,
		Source:      row.Source,
		Board:       row.Board,
		Status:      row.Status,
		CreatedAt:   pgconv.TimePtr(row.CreatedAt),
	}
}
