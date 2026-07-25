package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/vote"
)

// voteHandlers serves thumbs up/down on jobs and companies: the per-user vote
// write and the target's public counter recompute, delegated to vote.Service.
type voteHandlers struct {
	votes *vote.Service
}

func newVoteHandlers(queries *db.Queries, pool *pgxpool.Pool) *voteHandlers {
	return &voteHandlers{votes: vote.New(queries, pool)}
}

func (h *voteHandlers) register(api fiber.Router, mw middleware) {
	// Thumbs up/down: a signed-in vote (toggle/flip); the public counters it drives
	// are read by everyone on the job/company shapes.
	api.Post("/jobs/:slug/vote", mw.key, h.VoteJob)
	api.Delete("/jobs/:slug/vote", mw.key, h.ClearJobVote)
	api.Post("/companies/:slug/vote", mw.key, h.VoteCompany)
	api.Delete("/companies/:slug/vote", mw.key, h.ClearCompanyVote)
}

// voteRequest is the cast-vote body: the direction of the thumb tapped.
type voteRequest struct {
	Vote string `json:"vote"` // "up" | "down"
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

// parseVoteDirection decodes and validates the cast-vote body before any DB touch.
func parseVoteDirection(c *fiber.Ctx) (vote.Direction, error) {
	var in voteRequest
	if err := c.BodyParser(&in); err != nil {
		return 0, vote.ErrInvalidDirection
	}
	return vote.ParseDirection(in.Vote)
}

// VoteJob casts the caller's thumbs vote on a job (toggle/flip) and returns the
// job's resulting public counters and the caller's own vote.
func (h *voteHandlers) VoteJob(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	dir, err := parseVoteDirection(c)
	if err != nil {
		return voteError(err)
	}
	res, err := h.votes.VoteJob(c.Context(), userID, c.Params("slug"), dir)
	if err != nil {
		return voteError(err)
	}
	return c.JSON(fiber.Map{"data": res})
}

// ClearJobVote removes the caller's thumbs vote on a job (no-op when none).
func (h *voteHandlers) ClearJobVote(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	res, err := h.votes.ClearJob(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return voteError(err)
	}
	return c.JSON(fiber.Map{"data": res})
}

// VoteCompany casts the caller's thumbs vote on a company (toggle/flip).
func (h *voteHandlers) VoteCompany(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	dir, err := parseVoteDirection(c)
	if err != nil {
		return voteError(err)
	}
	res, err := h.votes.VoteCompany(c.Context(), userID, c.Params("slug"), dir)
	if err != nil {
		return voteError(err)
	}
	return c.JSON(fiber.Map{"data": res})
}

// ClearCompanyVote removes the caller's thumbs vote on a company (no-op when none).
func (h *voteHandlers) ClearCompanyVote(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	res, err := h.votes.ClearCompany(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return voteError(err)
	}
	return c.JSON(fiber.Map{"data": res})
}
