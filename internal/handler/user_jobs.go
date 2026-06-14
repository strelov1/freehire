package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/jobtracking"
)

// interactionResponse is the public shape of a user's interaction with a job. It
// omits user_id (the caller is the user) and carries saved_at/applied_at/stage/
// notes as null until the job is saved, applied to, or tracked.
type interactionResponse struct {
	JobID     int64      `json:"job_id"`
	ViewedAt  *time.Time `json:"viewed_at"`
	SavedAt   *time.Time `json:"saved_at"`
	AppliedAt *time.Time `json:"applied_at"`
	Stage     *string    `json:"stage"`
	Notes     *string    `json:"notes"`
}

// toResponse maps the domain Interaction onto the public wire shape.
func toResponse(i jobtracking.Interaction) interactionResponse {
	return interactionResponse{
		JobID: i.JobID, ViewedAt: i.ViewedAt, SavedAt: i.SavedAt,
		AppliedAt: i.AppliedAt, Stage: i.Stage, Notes: i.Notes,
	}
}

// trackingError maps the jobtracking sentinels onto HTTP statuses. Anything else
// (e.g. a DB failure) falls through to the central ErrorHandler as a 500.
func trackingError(err error) error {
	switch {
	case errors.Is(err, jobtracking.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "job not found")
	case errors.Is(err, jobtracking.ErrInvalidStage):
		return fiber.NewError(fiber.StatusBadRequest, "invalid stage")
	case errors.Is(err, jobtracking.ErrEmptyTrack):
		return fiber.NewError(fiber.StatusBadRequest, "provide stage and/or notes")
	default:
		return err
	}
}

// RecordView records that the authenticated user viewed a job and returns the
// resulting interaction, including whether they have already applied.
func (h *Handler) RecordView(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	interaction, err := h.tracking.RecordView(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// MarkApplied marks a job as applied for the authenticated user and returns the
// updated interaction.
func (h *Handler) MarkApplied(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	interaction, err := h.tracking.MarkApplied(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// SaveJob saves (bookmarks) a job for the authenticated user and returns the
// updated interaction.
func (h *Handler) SaveJob(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	interaction, err := h.tracking.SaveJob(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// UnsaveJob clears a job's saved mark for the authenticated user. The interaction
// row (view/apply history) survives; if no row exists at all, unsaving is a no-op
// that answers with the zero interaction state — DELETE is idempotent, so "already
// not saved" is success, not an error (the service resolves that case).
func (h *Handler) UnsaveJob(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	interaction, err := h.tracking.Unsave(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// ClearStage drops a job's pipeline progress (stage and applied_at) for the
// authenticated user while keeping saved_at, viewed_at, and notes intact. Used
// when dragging a Kanban card back to the "Saved" column.
func (h *Handler) ClearStage(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	interaction, err := h.tracking.ClearProgress(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// Untrack removes a job from the board for the authenticated user: clears
// saved_at, applied_at, stage, and notes while keeping viewed_at so the job
// stays in the user's view history.
func (h *Handler) Untrack(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	interaction, err := h.tracking.Untrack(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// trackRequest is the track body: an optional stage and/or notes. A nil field is
// left unchanged by the upsert; at least one must be present.
type trackRequest struct {
	Stage *string `json:"stage"`
	Notes *string `json:"notes"`
}

// TrackJob sets the application stage and/or notes for the authenticated user's
// interaction with a job (session cookie or API key). The body is validated by
// the service before the slug lookup, so a bad request never touches the DB: an
// empty body or an unknown stage is a 400. A nil field is left unchanged by the
// upsert. Returns the updated interaction.
func (h *Handler) TrackJob(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	var in trackRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	interaction, err := h.tracking.Track(c.Context(), userID, c.Params("slug"), in.Stage, in.Notes)
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}
