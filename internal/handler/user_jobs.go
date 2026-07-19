package handler

import (
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/reminder"
)

// saveJobRequest is the optional save body carrying a per-job reminder override.
// Absent (empty body) means "use the account default rule".
type saveJobRequest struct {
	Reminder *reminderOverrideRequest `json:"reminder"`
}

// reminderOverrideRequest is the save-time reminder choice: opt this job out
// (disabled) or set a custom delay in days; both unset means "keep the default".
type reminderOverrideRequest struct {
	Disabled  bool `json:"disabled"`
	DelayDays int  `json:"delay_days"`
}

// interactionResponse is the public shape of a user's interaction with a job. It
// omits user_id (the caller is the user) and carries saved_at/applied_at/stage/
// notes as null until the job is saved, applied to, or tracked.
type interactionResponse struct {
	JobID       int64      `json:"job_id"`
	ViewedAt    *time.Time `json:"viewed_at"`
	SavedAt     *time.Time `json:"saved_at"`
	AppliedAt   *time.Time `json:"applied_at"`
	DismissedAt *time.Time `json:"dismissed_at"`
	Stage       *string    `json:"stage"`
	Notes       *string    `json:"notes"`
}

// trackRequest is the track body: an optional stage and/or notes. A nil field is
// left unchanged by the upsert; at least one must be present.
type trackRequest struct {
	Stage *string `json:"stage"`
	Notes *string `json:"notes"`
}

// toResponse maps the domain Interaction onto the public wire shape.
func toResponse(i jobtracking.Interaction) interactionResponse {
	return interactionResponse{
		JobID: i.JobID, ViewedAt: i.ViewedAt, SavedAt: i.SavedAt,
		AppliedAt: i.AppliedAt, DismissedAt: i.DismissedAt, Stage: i.Stage, Notes: i.Notes,
	}
}

// trackingError maps the jobtracking sentinels onto HTTP statuses. Anything else
// (e.g. a DB failure) falls through to the central RenderError as a 500.
func trackingError(err error) error {
	switch {
	case errors.Is(err, jobtracking.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "job not found")
	case errors.Is(err, jobtracking.ErrInvalidStage):
		return fiber.NewError(fiber.StatusBadRequest, "invalid stage")
	case errors.Is(err, jobtracking.ErrInvalidFilter):
		return fiber.NewError(fiber.StatusBadRequest, "filter must be one of: all, viewed, saved, applied, board")
	case errors.Is(err, jobtracking.ErrEmptyTrack):
		return fiber.NewError(fiber.StatusBadRequest, "provide stage and/or notes")
	default:
		return err
	}
}

// RecordView records that the authenticated user viewed a job and returns the
// resulting interaction, including whether they have already applied.
func (a *API) RecordView(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	interaction, err := a.tracking.RecordView(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// MarkApplied marks a job as applied for the authenticated user and returns the
// updated interaction.
func (a *API) MarkApplied(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	interaction, err := a.tracking.MarkApplied(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	// Applying ends the "come back and apply" intent, so drop any pending reminder.
	a.cancelReminderBestEffort(c, userID, interaction.JobID)
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// SaveJob saves (bookmarks) a job for the authenticated user and returns the
// updated interaction. An optional body may carry a per-job reminder override;
// otherwise the account default rule decides whether to schedule a reminder.
func (a *API) SaveJob(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	interaction, err := a.tracking.SaveJob(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	a.scheduleReminderOnSave(c, userID, interaction.JobID)
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// scheduleReminderOnSave applies the reminder decision for a just-saved job. It is
// a best-effort side effect of the save: any failure (including a malformed override
// delay) is logged, never surfaced — the save already succeeded and is the primary
// action, and the worker's fire-time re-check backstops correctness. The UI sends
// only valid, fixed override delays.
func (a *API) scheduleReminderOnSave(c *fiber.Ctx, userID, jobID int64) {
	if a.reminder == nil {
		return
	}
	var in saveJobRequest
	// The body is optional (a bare save sends none); a parse failure just means no
	// override, so the account default applies.
	_ = c.BodyParser(&in)
	var ov *reminder.Override
	if in.Reminder != nil {
		ov = &reminder.Override{Disabled: in.Reminder.Disabled, DelayDays: in.Reminder.DelayDays}
	}
	if err := a.reminder.ScheduleOnSave(c.Context(), userID, jobID, ov); err != nil {
		log.Printf("reminder: schedule on save user=%d job=%d: %v", userID, jobID, err)
	}
}

// cancelReminderBestEffort cancels a job's pending reminder after the user applied
// or unsaved it. Best-effort: a failure is logged, not surfaced — the worker's
// fire-time re-check cancels a missed one anyway.
func (a *API) cancelReminderBestEffort(c *fiber.Ctx, userID, jobID int64) {
	if a.reminder == nil {
		return
	}
	if err := a.reminder.Cancel(c.Context(), userID, jobID); err != nil {
		log.Printf("reminder: cancel user=%d job=%d: %v", userID, jobID, err)
	}
}

// UnsaveJob clears a job's saved mark for the authenticated user. The interaction
// row (view/apply history) survives; if no row exists at all, unsaving is a no-op
// that answers with the zero interaction state — DELETE is idempotent, so "already
// not saved" is success, not an error (the service resolves that case).
func (a *API) UnsaveJob(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	interaction, err := a.tracking.Unsave(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	// Unsaving withdraws the intent the reminder was nudging toward, so cancel it.
	a.cancelReminderBestEffort(c, userID, interaction.JobID)
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// DismissJob marks a job dismissed (swiped away) for the authenticated user and
// returns the updated interaction. Dismissal only keeps the job out of the swipe
// deck; it stays visible in the public /jobs list and search.
func (a *API) DismissJob(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	interaction, err := a.tracking.Dismiss(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// UndismissJob clears a job's dismissed mark for the authenticated user. The
// interaction row survives; if no row exists at all, undismissing is a no-op that
// answers with the zero interaction state — DELETE is idempotent, so "already not
// dismissed" is success, not an error (the service resolves that case).
func (a *API) UndismissJob(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	interaction, err := a.tracking.Undismiss(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// ClearStage drops a job's pipeline progress (stage and applied_at) for the
// authenticated user while keeping saved_at, viewed_at, and notes intact. Used
// when dragging a Kanban card back to the "Saved" column.
func (a *API) ClearStage(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	interaction, err := a.tracking.ClearProgress(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// Untrack removes a job from the board for the authenticated user: clears
// saved_at, applied_at, stage, and notes while keeping viewed_at so the job
// stays in the user's view history.
func (a *API) Untrack(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	interaction, err := a.tracking.Untrack(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}

// TrackJob sets the application stage and/or notes for the authenticated user's
// interaction with a job (session cookie or API key). The body is validated by
// the service before the slug lookup, so a bad request never touches the DB: an
// empty body or an unknown stage is a 400. A nil field is left unchanged by the
// upsert. Returns the updated interaction.
func (a *API) TrackJob(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in trackRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	interaction, err := a.tracking.Track(c.Context(), userID, c.Params("slug"), in.Stage, in.Notes)
	if err != nil {
		return trackingError(err)
	}
	return c.JSON(fiber.Map{"data": toResponse(interaction)})
}
