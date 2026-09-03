package contribution

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository is the production Repository backed by sqlc-generated *db.Queries.
// A recognized board is a row in `boards` (see internal/ingest/boardcatalog); an
// unrecognized link awaiting triage is a row in `board_submissions`.
type QueriesRepository struct {
	q      *db.Queries
	boards boardcatalog.Repository
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q, boards: boardcatalog.NewQueriesRepository(q)}
}

// BoardTracked reports whether the catalogue already crawls this board (any job whose
// external_id is "<board>:…"). It matches with a LIKE-prefix served by the
// (source, external_id text_pattern_ops) index; the board's LIKE metacharacters are escaped
// so a slug with % or _ cannot widen the match. Reads `jobs`, unaffected by the
// board-catalog migration.
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

// BoardByAshbyJobID returns the ashby board already carrying a job with the given Ashby job id,
// or ok=false when none is tracked.
func (r *QueriesRepository) BoardByAshbyJobID(ctx context.Context, jobID string) (board string, ok bool, err error) {
	board, err = r.q.BoardByAshbyJobID(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return board, true, nil
}

// CompanyForBoard returns the company name + slug already tracked on the board, or ok=false
// when the board has no job with a resolved company. Reads `jobs`/`companies`, unaffected
// by the board-catalog migration.
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

// Record inserts the board at status='pending' — immediately eligible for the provider's
// next scheduled crawl, with no separate manual onboarding step. It carries no real
// company name (recognition from a URL is network-free), so it is seeded with a
// placeholder derived from the board id (boardcatalog.PlaceholderCompany); a curator
// corrects it later via cmd/add-board --rename. A collision with an existing
// pending/active board maps to ErrBoardAlreadyContributed, which also makes the
// concurrent-duplicate race safe: exactly one insert wins, so exactly one submission is
// ever rewarded. The reward itself is granted by the caller, keyed by the returned id.
func (r *QueriesRepository) Record(ctx context.Context, in RecordInput) (Contribution, error) {
	submittedBy := in.SubmittedBy
	b, err := boardcatalog.Insert(ctx, r.boards, boardcatalog.InsertInput{
		Provider:    in.Source,
		Board:       in.Board,
		Company:     boardcatalog.PlaceholderCompany(in.Board),
		URL:         in.URL,
		SubmittedBy: &submittedBy,
		Surface:     NormalizeSurface(in.Surface),
	}, boardcatalog.StatusPending, sources.Taxonomy())
	if err != nil {
		if errors.Is(err, boardcatalog.ErrDuplicateBoard) {
			return Contribution{}, ErrBoardAlreadyContributed
		}
		return Contribution{}, err
	}
	if b.Status == boardcatalog.StatusRejected {
		// A recognized (source, board) that still fails catalog validation is a drift
		// between atsboard's recognizer and the registered adapters — treat it as any
		// other failed intake: no contribution worth showing, no credit.
		return Contribution{}, errors.New("contribution: board failed catalog validation: " + b.RejectedReason)
	}
	return contributionFromBoard(b), nil
}

// RecordReview inserts an unrecognized-but-valid link for manual review: no source/board,
// status 'review', no AI credit. The unique index on board_submissions.url rejects a
// duplicate submission of the same url, mapped to ErrBoardAlreadyContributed — an
// unrecognised link still belongs in the triage queue at most once, unlike a recognised
// board.
func (r *QueriesRepository) RecordReview(ctx context.Context, submittedBy int64, url, surface string) (Contribution, error) {
	row, err := r.q.InsertBoardSubmission(ctx, db.InsertBoardSubmissionParams{
		URL: url, SubmittedBy: submittedBy, Surface: NormalizeSurface(surface),
	})
	if err != nil {
		if pgerr.IsUniqueViolation(err) {
			return Contribution{}, ErrBoardAlreadyContributed
		}
		return Contribution{}, err
	}
	return contributionFromSubmission(row), nil
}

// ListByUser returns one user's contributions, newest first — the union of their
// recognized boards and their still-unclassified submissions.
func (r *QueriesRepository) ListByUser(ctx context.Context, userID int64) ([]Contribution, error) {
	boards, err := r.boards.ListBySubmitter(ctx, userID)
	if err != nil {
		return nil, err
	}
	submissions, err := r.q.ListBoardSubmissionsBySubmitter(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Contribution, 0, len(boards)+len(submissions))
	for _, b := range boards {
		out = append(out, contributionFromBoard(b))
	}
	for _, s := range submissions {
		out = append(out, contributionFromSubmission(s))
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := out[i].CreatedAt, out[j].CreatedAt
		if ti == nil || tj == nil {
			return false
		}
		return ti.After(*tj)
	})
	return out, nil
}

// contributionFromBoard maps a recognized board to the package domain type.
func contributionFromBoard(b boardcatalog.Board) Contribution {
	var submittedBy int64
	if b.SubmittedBy != nil {
		submittedBy = *b.SubmittedBy
	}
	createdAt := b.CreatedAt
	return Contribution{
		ID:          b.ID,
		SubmittedBy: submittedBy,
		URL:         b.URL,
		Source:      b.Provider,
		Board:       b.Board,
		Status:      string(b.Status),
		Surface:     b.Surface,
		CreatedAt:   &createdAt,
	}
}

// contributionFromSubmission maps an unclassified triage row to the package domain type:
// status review, no source/board.
func contributionFromSubmission(s db.BoardSubmission) Contribution {
	return Contribution{
		ID:          s.ID,
		SubmittedBy: s.SubmittedBy,
		URL:         s.URL,
		Status:      StatusReview,
		Surface:     s.Surface,
		CreatedAt:   pgconv.TimePtr(s.CreatedAt),
	}
}
