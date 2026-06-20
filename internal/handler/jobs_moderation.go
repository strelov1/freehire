package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/moderation"
)

// createJobRequest is the moderator create-job body. url/title/company are required
// (validated by the service); source is the posting's real origin (defaults to "manual"
// when absent); remote defaults to false when absent; posted_at is an optional RFC3339
// timestamp.
type createJobRequest struct {
	URL         string     `json:"url"`
	Source      string     `json:"source"`
	Title       string     `json:"title"`
	Company     string     `json:"company"`
	Location    string     `json:"location"`
	Remote      bool       `json:"remote"`
	Description string     `json:"description"`
	PostedAt    *time.Time `json:"posted_at"`
}

// toCreateInput maps the wire body onto the moderation create input. Shared by the
// moderator create path and the public submission path, which carry identical content.
func (r createJobRequest) toCreateInput() moderation.CreateInput {
	return moderation.CreateInput{
		URL:         r.URL,
		Source:      r.Source,
		Title:       r.Title,
		Company:     r.Company,
		Location:    r.Location,
		Remote:      r.Remote,
		Description: r.Description,
		PostedAt:    r.PostedAt,
	}
}

// updateJobRequest is the moderator edit body: every field is optional, and a field
// left out (nil) is unchanged. The source identity (url) is not editable.
type updateJobRequest struct {
	Title       *string    `json:"title"`
	Company     *string    `json:"company"`
	Location    *string    `json:"location"`
	Remote      *bool      `json:"remote"`
	Description *string    `json:"description"`
	PostedAt    *time.Time `json:"posted_at"`
}

// moderationError maps the moderation sentinels onto HTTP statuses. ErrInvalid carries a
// user-facing message surfaced in the 400 body; anything else (e.g. a DB failure) falls
// through to the central RenderError as a 500.
func moderationError(err error) error {
	switch {
	case errors.Is(err, moderation.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "job not found")
	case errors.Is(err, moderation.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}

// CreateJob creates a hand-curated vacancy (moderator only). The body is validated by the
// service, so a missing required field or a bad URL is a 400 before any DB write. Returns
// the created job in the public wire shape with 201.
func (a *API) CreateJob(c *fiber.Ctx) error {
	actorID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in createJobRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	job, err := a.moderation.Create(c.Context(), actorID, in.toCreateInput())
	if err != nil {
		return moderationError(err)
	}

	view, err := jobview.FromRow(job)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": view})
}

// UpdateJob partially edits a manual vacancy (moderator only), addressed by public slug.
// A non-manual or unknown slug is a 404. Returns the updated job in the public wire shape.
func (a *API) UpdateJob(c *fiber.Ctx) error {
	actorID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in updateJobRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	job, err := a.moderation.Update(c.Context(), actorID, c.Params("slug"), moderation.UpdatePatch{
		Title:       in.Title,
		Company:     in.Company,
		Location:    in.Location,
		Remote:      in.Remote,
		Description: in.Description,
		PostedAt:    in.PostedAt,
	})
	if err != nil {
		return moderationError(err)
	}

	view, err := jobview.FromRow(job)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": view})
}
