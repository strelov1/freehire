package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/vote"
)

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
func (a *API) VoteJob(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	dir, err := parseVoteDirection(c)
	if err != nil {
		return voteError(err)
	}
	res, err := a.votes.VoteJob(c.Context(), userID, c.Params("slug"), dir)
	if err != nil {
		return voteError(err)
	}
	return c.JSON(fiber.Map{"data": res})
}

// ClearJobVote removes the caller's thumbs vote on a job (no-op when none).
func (a *API) ClearJobVote(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	res, err := a.votes.ClearJob(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return voteError(err)
	}
	return c.JSON(fiber.Map{"data": res})
}

// VoteCompany casts the caller's thumbs vote on a company (toggle/flip).
func (a *API) VoteCompany(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	dir, err := parseVoteDirection(c)
	if err != nil {
		return voteError(err)
	}
	res, err := a.votes.VoteCompany(c.Context(), userID, c.Params("slug"), dir)
	if err != nil {
		return voteError(err)
	}
	return c.JSON(fiber.Map{"data": res})
}

// ClearCompanyVote removes the caller's thumbs vote on a company (no-op when none).
func (a *API) ClearCompanyVote(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	res, err := a.votes.ClearCompany(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return voteError(err)
	}
	return c.JSON(fiber.Map{"data": res})
}
