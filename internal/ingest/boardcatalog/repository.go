package boardcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// ErrDuplicateBoard is a candidate colliding, on (provider, board, region), with an
// existing pending or active row — boards_identity_key. A rejected or retired row does
// not occupy the identity, so a corrected resubmission after a validation failure or a
// re-added board after retirement is never blocked by it.
var ErrDuplicateBoard = errors.New("boardcatalog: board already exists")

// Repository is the boards-table persistence contract, expressed in package domain
// types. It does not validate — see Insert, which validates and then calls InsertRow.
type Repository interface {
	// InsertRow persists a candidate at the given status (StatusRejected carries
	// rejectedReason; every other status ignores it). A collision with an existing
	// pending/active row of the same identity returns ErrDuplicateBoard.
	InsertRow(ctx context.Context, in InsertInput, status Status, rejectedReason string) (Board, error)
	// Activate flips a pending board to active on its first successful crawl. A board
	// that is already active, retired, rejected, or does not exist is left untouched
	// (found=false) — the caller need not check first.
	Activate(ctx context.Context, provider, board, region string) (found bool, err error)
	// Retire marks a live (pending or active) board retired without deleting its row.
	// found=false when no such live board exists.
	Retire(ctx context.Context, provider, board, region string) (found bool, err error)
	// Rename corrects a live board's company name — for a crowdsourced row seeded with
	// PlaceholderCompany, once a curator knows the real one. found=false when no such
	// live board exists.
	Rename(ctx context.Context, provider, board, region, company string) (found bool, err error)
	// ListActiveForProvider returns the boards cmd/ingest crawls for one provider:
	// pending and active.
	ListActiveForProvider(ctx context.Context, provider string) ([]Board, error)
	// ListBySubmitter returns one user's crowdsourced boards, newest first.
	ListBySubmitter(ctx context.Context, submittedBy int64) ([]Board, error)
}

// Insert validates the candidate against registry and persists it: StatusRejected with
// the validation error as reason when invalid, else wantStatus (StatusPending for a
// crowdsourced submission, StatusActive for a curator addition via cmd/add-board).
// Insert never surfaces a validation failure as an error — only ErrDuplicateBoard (a
// collision with an existing pending/active row) or a genuine persistence error.
func Insert(ctx context.Context, repo Repository, in InsertInput, wantStatus Status, registry map[string]sources.Source) (Board, error) {
	if err := Validate(in, registry); err != nil {
		return repo.InsertRow(ctx, in, StatusRejected, err.Error())
	}
	return repo.InsertRow(ctx, in, wantStatus, "")
}

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

func (r *QueriesRepository) InsertRow(ctx context.Context, in InsertInput, status Status, rejectedReason string) (Board, error) {
	tenants := []byte("{}")
	if in.Tenants != nil {
		var err error
		if tenants, err = json.Marshal(in.Tenants); err != nil {
			return Board{}, err
		}
	}
	activatedAt := pgtype.Timestamptz{}
	if status == StatusActive {
		activatedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	surface := in.Surface
	if surface == "" {
		surface = "curator"
	}
	row, err := r.q.InsertBoard(ctx, db.InsertBoardParams{
		Provider:       in.Provider,
		Board:          in.Board,
		Region:         in.Region,
		Company:        in.Company,
		Hub:            in.Hub,
		Tenants:        tenants,
		URL:            pgconv.Text(in.URL),
		Status:         string(status),
		SubmittedBy:    pgconv.Int8(in.SubmittedBy),
		Surface:        surface,
		RejectedReason: pgconv.Text(rejectedReason),
		ActivatedAt:    activatedAt,
	})
	if err != nil {
		if pgerr.IsUniqueViolation(err) {
			return Board{}, ErrDuplicateBoard
		}
		return Board{}, err
	}
	return fromRow(row)
}

func (r *QueriesRepository) Activate(ctx context.Context, provider, board, region string) (bool, error) {
	n, err := r.q.ActivateBoard(ctx, db.ActivateBoardParams{Provider: provider, Lower: board, Region: region})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *QueriesRepository) Retire(ctx context.Context, provider, board, region string) (bool, error) {
	n, err := r.q.RetireBoard(ctx, db.RetireBoardParams{Provider: provider, Lower: board, Region: region})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *QueriesRepository) Rename(ctx context.Context, provider, board, region, company string) (bool, error) {
	n, err := r.q.UpdateBoardCompany(ctx, db.UpdateBoardCompanyParams{
		Provider: provider, Board: board, Region: region, Company: company,
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *QueriesRepository) ListActiveForProvider(ctx context.Context, provider string) ([]Board, error) {
	rows, err := r.q.ListActiveBoardsForProvider(ctx, provider)
	if err != nil {
		return nil, err
	}
	return fromRows(rows)
}

func (r *QueriesRepository) ListBySubmitter(ctx context.Context, submittedBy int64) ([]Board, error) {
	rows, err := r.q.ListBoardsBySubmitter(ctx, pgconv.Int8(&submittedBy))
	if err != nil {
		return nil, err
	}
	return fromRows(rows)
}

func fromRows(rows []db.Board) ([]Board, error) {
	out := make([]Board, len(rows))
	for i, row := range rows {
		b, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		out[i] = b
	}
	return out, nil
}

func fromRow(row db.Board) (Board, error) {
	var tenants map[string]string
	if len(row.Tenants) > 0 {
		if err := json.Unmarshal(row.Tenants, &tenants); err != nil {
			return Board{}, err
		}
	}
	return Board{
		ID:             row.ID,
		Provider:       row.Provider,
		Board:          row.Board,
		Region:         row.Region,
		Company:        row.Company,
		Hub:            row.Hub,
		Tenants:        tenants,
		URL:            pgconv.TextString(row.URL),
		Status:         Status(row.Status),
		SubmittedBy:    pgconv.Int8Ptr(row.SubmittedBy),
		Surface:        row.Surface,
		RejectedReason: pgconv.TextString(row.RejectedReason),
		CreatedAt:      row.CreatedAt.Time,
		ActivatedAt:    pgconv.TimePtr(row.ActivatedAt),
	}, nil
}
