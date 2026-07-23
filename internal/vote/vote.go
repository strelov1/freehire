// Package vote owns thumbs up/down voting for jobs and companies. A signed-in
// user holds at most one vote per target; the aggregate up/down counts are a
// public signal materialized on the job and company rows.
//
// A vote write and the affected target's counter recompute run in one pgx
// transaction, so a reader never sees a vote without its counter. The recompute
// is a separate statement run after the write — a same-statement CTE would not
// see the write, since Postgres hides a data-modifying CTE's effect from sibling
// sub-statements.
package vote

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrNotFound is returned when the slug resolves to no job or company.
	ErrNotFound = errors.New("vote: target not found")
	// ErrInvalidDirection is returned when a request carries neither up nor down.
	ErrInvalidDirection = errors.New("vote: invalid direction")
)

// Direction is a cast vote: Up (+1) or Down (-1).
type Direction int16

const (
	Down Direction = -1
	Up   Direction = 1
)

// ParseDirection maps the request body's direction token onto a Direction,
// rejecting anything else before any database touch.
func ParseDirection(s string) (Direction, error) {
	switch s {
	case "up":
		return Up, nil
	case "down":
		return Down, nil
	default:
		return 0, ErrInvalidDirection
	}
}

// Result is the outcome of a vote write: the target's resulting public counters
// and the caller's own vote (-1, 0, or 1).
type Result struct {
	UpvoteCount   int32 `json:"upvote_count"`
	DownvoteCount int32 `json:"downvote_count"`
	MyVote        int16 `json:"my_vote"`
}

// Service applies vote writes against Postgres.
type Service struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// New builds a Service over the shared queries and pool (the pool drives the
// per-write transaction).
func New(q *db.Queries, pool *pgxpool.Pool) *Service {
	return &Service{q: q, pool: pool}
}

// VoteJob casts dir on a job with toggle/flip semantics (re-casting the same
// direction clears it; the opposite replaces it) and returns the resulting
// counters and caller vote.
func (s *Service) VoteJob(ctx context.Context, userID int64, slug string, dir Direction) (Result, error) {
	jobID, err := s.jobIDBySlug(ctx, slug)
	if err != nil {
		return Result{}, err
	}
	return s.jobTx(ctx, jobID, func(q *db.Queries) (int16, error) {
		if err := q.LockJobForVote(ctx, jobID); err != nil {
			return 0, err
		}
		return q.UpsertJobVote(ctx, db.UpsertJobVoteParams{
			UserID: userID, JobID: jobID,
			Vote: pgtype.Int2{Int16: int16(dir), Valid: true},
		})
	})
}

// ClearJob removes the caller's vote on a job (no-op when none) and returns the
// resulting counters with my_vote = 0.
func (s *Service) ClearJob(ctx context.Context, userID int64, slug string) (Result, error) {
	jobID, err := s.jobIDBySlug(ctx, slug)
	if err != nil {
		return Result{}, err
	}
	return s.jobTx(ctx, jobID, func(q *db.Queries) (int16, error) {
		if err := q.LockJobForVote(ctx, jobID); err != nil {
			return 0, err
		}
		return 0, q.ClearJobVote(ctx, db.ClearJobVoteParams{UserID: userID, JobID: jobID})
	})
}

// VoteCompany casts dir on a company with toggle/flip semantics. Unlike a job
// vote (a nullable column on the persistent user_jobs row), a company vote row
// exists solely to hold the vote, so clearing it deletes the row — the toggle
// decision is made here in Go.
func (s *Service) VoteCompany(ctx context.Context, userID int64, slug string, dir Direction) (Result, error) {
	if err := s.requireCompany(ctx, slug); err != nil {
		return Result{}, err
	}
	return s.companyTx(ctx, slug, func(q *db.Queries) (int16, error) {
		if err := q.LockCompanyForVote(ctx, slug); err != nil {
			return 0, err
		}
		cur, err := q.GetCompanyVote(ctx, db.GetCompanyVoteParams{UserID: userID, CompanySlug: slug})
		if err != nil {
			return 0, err
		}
		if cur == int16(dir) { // re-cast same direction clears it
			return 0, q.DeleteCompanyVote(ctx, db.DeleteCompanyVoteParams{UserID: userID, CompanySlug: slug})
		}
		return int16(dir), q.UpsertCompanyVote(ctx, db.UpsertCompanyVoteParams{
			UserID: userID, CompanySlug: slug, Vote: int16(dir),
		})
	})
}

// ClearCompany removes the caller's vote on a company (no-op when none) and
// returns the resulting counters with my_vote = 0.
func (s *Service) ClearCompany(ctx context.Context, userID int64, slug string) (Result, error) {
	if err := s.requireCompany(ctx, slug); err != nil {
		return Result{}, err
	}
	return s.companyTx(ctx, slug, func(q *db.Queries) (int16, error) {
		if err := q.LockCompanyForVote(ctx, slug); err != nil {
			return 0, err
		}
		return 0, q.DeleteCompanyVote(ctx, db.DeleteCompanyVoteParams{UserID: userID, CompanySlug: slug})
	})
}

// jobTx runs write (the vote mutation, returning the caller's resulting vote) and
// the job counter recompute in one transaction.
func (s *Service) jobTx(ctx context.Context, jobID int64, write func(*db.Queries) (int16, error)) (Result, error) {
	return s.inTx(ctx, func(q *db.Queries) (Result, error) {
		myVote, err := write(q)
		if err != nil {
			return Result{}, err
		}
		counts, err := q.RecountJobVotes(ctx, jobID)
		if err != nil {
			return Result{}, err
		}
		return Result{UpvoteCount: counts.UpvoteCount, DownvoteCount: counts.DownvoteCount, MyVote: myVote}, nil
	})
}

// companyTx runs write and the company counter recompute in one transaction.
func (s *Service) companyTx(ctx context.Context, slug string, write func(*db.Queries) (int16, error)) (Result, error) {
	return s.inTx(ctx, func(q *db.Queries) (Result, error) {
		myVote, err := write(q)
		if err != nil {
			return Result{}, err
		}
		counts, err := q.RecountCompanyVotes(ctx, slug)
		if err != nil {
			return Result{}, err
		}
		return Result{UpvoteCount: counts.UpvoteCount, DownvoteCount: counts.DownvoteCount, MyVote: myVote}, nil
	})
}

// inTx runs fn inside a transaction, committing on success and rolling back on
// error.
func (s *Service) inTx(ctx context.Context, fn func(*db.Queries) (Result, error)) (Result, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	res, err := fn(s.q.WithTx(tx))
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return res, nil
}

func (s *Service) jobIDBySlug(ctx context.Context, slug string) (int64, error) {
	id, err := s.q.GetJobIDBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func (s *Service) requireCompany(ctx context.Context, slug string) error {
	ok, err := s.q.CompanySlugExists(ctx, slug)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}
