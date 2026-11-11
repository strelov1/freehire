package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/vote"
	"github.com/strelov1/freehire/internal/platform/db"
)

// voteHandlers serves thumbs up/down on jobs and companies: the per-user vote
// write and the target's public counter recompute, delegated to vote.Service.
type voteHandlers struct {
	votes *vote.Service
}

func newVoteHandlers(queries *db.Queries, pool *pgxpool.Pool) *voteHandlers {
	return &voteHandlers{votes: vote.New(queries, pool)}
}

func (h *voteHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Thumbs up/down: a signed-in vote (toggle/flip); the public counters it drives
	// are read by everyone on the job/company shapes.
	api.Post("/jobs/:slug/vote", mw.key, contract)
	api.Delete("/jobs/:slug/vote", mw.key, contract)
	api.Post("/companies/:slug/vote", mw.key, contract)
	api.Delete("/companies/:slug/vote", mw.key, contract)
}

// voteError maps the vote sentinels onto HTTP statuses. Anything else (e.g. a DB
// failure) falls through to the central RenderError as a 500.
func voteError(err error) error {
	switch {
	case errors.Is(err, vote.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "not found")
	case errors.Is(err, vote.ErrInvalidDirection):
		return fiber.NewError(fiber.StatusBadRequest, `vote must be "up" or "down"`)
	default:
		return err
	}
}

// VoteJob casts the caller's thumbs vote on a job (toggle/flip) and returns the
// job's resulting public counters and the caller's own vote.
func (h *voteHandlers) VoteJob(ctx context.Context, req *apigen.VoteRequest, params apigen.VoteJobParams) (*apigen.VoteJobOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	dir, err := vote.ParseDirection(req.Vote.Value)
	if err != nil {
		return nil, voteError(err)
	}
	res, err := h.votes.VoteJob(ctx, userID, params.Slug, dir)
	if err != nil {
		return nil, voteError(err)
	}
	return &apigen.VoteJobOK{Data: voteResultOut(res)}, nil
}

// ClearJobVote removes the caller's thumbs vote on a job (no-op when none).
func (h *voteHandlers) ClearJobVote(ctx context.Context, params apigen.ClearJobVoteParams) (*apigen.ClearJobVoteOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	res, err := h.votes.ClearJob(ctx, userID, params.Slug)
	if err != nil {
		return nil, voteError(err)
	}
	return &apigen.ClearJobVoteOK{Data: voteResultOut(res)}, nil
}

// VoteCompany casts the caller's thumbs vote on a company (toggle/flip).
func (h *voteHandlers) VoteCompany(ctx context.Context, req *apigen.VoteRequest, params apigen.VoteCompanyParams) (*apigen.VoteCompanyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	dir, err := vote.ParseDirection(req.Vote.Value)
	if err != nil {
		return nil, voteError(err)
	}
	res, err := h.votes.VoteCompany(ctx, userID, params.Slug, dir)
	if err != nil {
		return nil, voteError(err)
	}
	return &apigen.VoteCompanyOK{Data: voteResultOut(res)}, nil
}

// ClearCompanyVote removes the caller's thumbs vote on a company (no-op when none).
func (h *voteHandlers) ClearCompanyVote(ctx context.Context, params apigen.ClearCompanyVoteParams) (*apigen.ClearCompanyVoteOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	res, err := h.votes.ClearCompany(ctx, userID, params.Slug)
	if err != nil {
		return nil, voteError(err)
	}
	return &apigen.ClearCompanyVoteOK{Data: voteResultOut(res)}, nil
}

func voteResultOut(r vote.Result) apigen.VoteResult {
	return apigen.VoteResult{UpvoteCount: int64(r.UpvoteCount), DownvoteCount: int64(r.DownvoteCount), MyVote: int64(r.MyVote)}
}
