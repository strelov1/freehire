package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/engage/reminder"
	"github.com/strelov1/freehire/internal/job/applydate"
	"github.com/strelov1/freehire/internal/platform/db"
)

// trackingHandlers serves per-user job interactions (view/apply/save/dismiss/track), the
// tracking reads (Kanban listing, slug sets, pipeline, swipe deck) and the reminder controls.
// jobtracking.Service owns the use cases and the reminder side effect of save/unsave/apply;
// reminder.Service is held here only for the notification-settings endpoints.
type trackingHandlers struct {
	tracking *jobtracking.Service
	reminder *reminder.Service
	search   searcher
	// queries backs the ghost-signal attach step (me_tracking.go): ghostEvidenceFor and
	// ListJobGhostStamps use it directly, since the tracking service stays ghost-agnostic.
	queries *db.Queries
}

func newTrackingHandlers(queries *db.Queries, tracking *jobtracking.Service, rem *reminder.Service, search searcher) *trackingHandlers {
	return &trackingHandlers{
		tracking: tracking,
		reminder: rem,
		search:   search,
		queries:  queries,
	}
}

func (h *trackingHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Cookie or API key. Jobs are addressed by public slug, resolved to the internal id
	// before writing user_jobs. All writes are idempotent upserts.
	api.Post("/jobs/:slug/view", mw.key, contract)
	api.Post("/jobs/:slug/apply", mw.key, contract)
	api.Post("/jobs/:slug/save", mw.key, contract)
	api.Delete("/jobs/:slug/save", mw.key, contract)
	api.Post("/jobs/:slug/dismiss", mw.key, contract)
	api.Delete("/jobs/:slug/dismiss", mw.key, contract)
	api.Patch("/jobs/:slug/track", mw.key, contract)
	api.Delete("/jobs/:slug/stage", mw.key, contract)
	api.Delete("/jobs/:slug/track", mw.key, contract)

	// The same three writes, addressed by the row id the tracking listing served: an
	// application whose posting was deleted has no slug, so the routes above cannot move its card.
	//
	// Not /me/tracking/:id: that path is already mounted with a posting slug (see the inbox
	// handlers), and one segment meaning different things per method is a trap.
	// The slug routes stay for freehire-cli and freehire-mcp.
	api.Patch("/me/applications/:id", mw.key, contract)
	api.Delete("/me/applications/:id", mw.key, contract)
	api.Delete("/me/applications/:id/stage", mw.key, contract)

	// User-scoped reads under /me. viewed-slugs lets the SPA dim seen cards without authenticating the public list.
	api.Get("/me/tracking", mw.key, contract)
	api.Get("/me/tracking/viewed", mw.key, contract)
	api.Get("/me/tracking/saved", mw.key, contract)
	api.Get("/me/tracking/dismissed", mw.key, contract)
	api.Get("/me/tracking/applied-companies", mw.key, contract)
	api.Get("/me/tracking/pipeline", mw.key, contract)
	api.Get("/me/tracking/swipe", mw.key, h.SwipeDeck)

	// The account-level notification rule shared by saved-job reminders and both lifecycle
	// nudges. Cookie-only like subscriptions.
	api.Get("/me/notification-settings", mw.cookie, contract)
	api.Put("/me/notification-settings", mw.cookie, contract)
}

// interactionResponse is the wire shape of an interaction for the assistant's tools, which
// answer with this Go type (the route's own is apigen.Interaction, see interactionOut).
type interactionResponse struct {
	JobID       int64      `json:"job_id"`
	ViewedAt    *time.Time `json:"viewed_at"`
	SavedAt     *time.Time `json:"saved_at"`
	AppliedAt   *time.Time `json:"applied_at"`
	DismissedAt *time.Time `json:"dismissed_at"`
	Stage       *string    `json:"stage"`
	Notes       *string    `json:"notes"`
}

// toResponse maps the domain Interaction onto the assistant tools' wire shape.
func toResponse(i jobtracking.Interaction) interactionResponse {
	return interactionResponse{
		JobID: i.JobID, ViewedAt: i.ViewedAt, SavedAt: i.SavedAt,
		AppliedAt: i.AppliedAt, DismissedAt: i.DismissedAt, Stage: i.Stage, Notes: i.Notes,
	}
}

// interactionOut is the public shape of a user's interaction with a job. It omits user_id and
// carries saved_at/applied_at/stage/notes as null until set.
func interactionOut(i jobtracking.Interaction) apigen.Interaction {
	return apigen.Interaction{
		JobID: i.JobID, ViewedAt: nilDateTime(i.ViewedAt), SavedAt: nilDateTime(i.SavedAt),
		AppliedAt: nilDateTime(i.AppliedAt), DismissedAt: nilDateTime(i.DismissedAt),
		Stage: nilString(i.Stage), Notes: nilString(i.Notes),
	}
}

// trackingError maps the jobtracking sentinels onto HTTP statuses; anything else is a 500.
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
	case errors.Is(err, jobtracking.ErrApplicationNotFound):
		return fiber.NewError(fiber.StatusNotFound, "application not found")
	case errors.Is(err, applydate.ErrOutOfRange):
		// The message names which bound was crossed.
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}

// RecordView records that the authenticated user viewed a job and returns the
// resulting interaction, including whether they have already applied.
func (h *trackingHandlers) RecordView(ctx context.Context, params apigen.RecordViewParams) (*apigen.RecordViewOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	interaction, err := h.tracking.RecordView(ctx, userID, params.Slug)
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.RecordViewOK{Data: interactionOut(interaction)}, nil
}

// MarkApplied marks a job as applied for the authenticated user and returns the
// updated interaction.
//
// The body is optional: absent, empty or without a date it is an undated apply stamped now().
// A date present is the caller's account of when they applied and overrides one already recorded.
//
// A body that is not JSON, or whose `applied_on` is not a string, is refused by the contract
// before this runs; treating it as "no date" would silently stamp today for a caller who named a day.
func (h *trackingHandlers) MarkApplied(ctx context.Context, req apigen.OptApplyJob, params apigen.MarkAppliedParams) (*apigen.MarkAppliedOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	day, err := statedApplyDay(req)
	if err != nil {
		return nil, err
	}
	var interaction jobtracking.Interaction
	if day != nil {
		// The service owns the believable-date window; an out-of-range day arrives as an error.
		interaction, err = h.tracking.MarkAppliedOn(
			ctx, userID, params.Slug, *day, time.Now().UTC(), appevent.SourceUser)
	} else {
		interaction, err = h.tracking.MarkApplied(ctx, userID, params.Slug, appevent.SourceUser)
	}
	if err != nil {
		return nil, trackingError(err)
	}
	i := interactionOut(interaction)
	out := apigen.AppliedInteraction{
		JobID: i.JobID, ViewedAt: i.ViewedAt, SavedAt: i.SavedAt, AppliedAt: i.AppliedAt,
		DismissedAt: i.DismissedAt, Stage: i.Stage, Notes: i.Notes,
	}
	if interaction.ApplicationID != 0 {
		out.ApplicationID.SetTo(applicationRefPrefix + strconv.FormatInt(interaction.ApplicationID, 10))
	}
	return &apigen.MarkAppliedOK{Data: out}, nil
}

// statedApplyDay reads the optional day from the request. It answers nil when the caller sent
// no body or no `applied_on`: an empty request means today, not a mistake.
//
// The day is returned as a day; placing it at the storage hour is the service's job, because
// the believable-date window checked against the derived instant would refuse "today" all morning.
func statedApplyDay(req apigen.OptApplyJob) (*time.Time, error) {
	in, ok := req.Get()
	if !ok {
		return nil, nil
	}
	raw, ok := in.AppliedOn.Get()
	if !ok || raw == "" {
		return nil, nil
	}
	day, err := time.Parse(appliedOnLayout, raw)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "applied_on must be a date like 2026-07-29")
	}
	return &day, nil
}

// SaveJob bookmarks a job and returns the updated interaction. The account's notification
// rule decides whether a reminder is scheduled.
func (h *trackingHandlers) SaveJob(ctx context.Context, params apigen.SaveJobParams) (*apigen.SaveJobOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	interaction, err := h.tracking.SaveJob(ctx, userID, params.Slug)
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.SaveJobOK{Data: interactionOut(interaction)}, nil
}

// UnsaveJob clears a job's saved mark; the interaction row (view/apply history) survives.
// With no row it is a no-op answering the zero state: DELETE is idempotent (the service resolves that).
func (h *trackingHandlers) UnsaveJob(ctx context.Context, params apigen.UnsaveJobParams) (*apigen.UnsaveJobOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	interaction, err := h.tracking.Unsave(ctx, userID, params.Slug)
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.UnsaveJobOK{Data: interactionOut(interaction)}, nil
}

// DismissJob marks a job dismissed (swiped away). It only keeps the job out of the swipe
// deck; it stays in the public /jobs list and search.
func (h *trackingHandlers) DismissJob(ctx context.Context, params apigen.DismissJobParams) (*apigen.DismissJobOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	interaction, err := h.tracking.Dismiss(ctx, userID, params.Slug)
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.DismissJobOK{Data: interactionOut(interaction)}, nil
}

// UndismissJob clears a job's dismissed mark; the interaction row survives. With no row it is
// a no-op answering the zero state: DELETE is idempotent (the service resolves that).
func (h *trackingHandlers) UndismissJob(ctx context.Context, params apigen.UndismissJobParams) (*apigen.UndismissJobOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	interaction, err := h.tracking.Undismiss(ctx, userID, params.Slug)
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.UndismissJobOK{Data: interactionOut(interaction)}, nil
}

// ClearStage drops a job's pipeline progress (stage and applied_at), keeping saved_at,
// viewed_at and notes. Used when a Kanban card returns to "Saved".
func (h *trackingHandlers) ClearStage(ctx context.Context, params apigen.ClearStageParams) (*apigen.ClearStageOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	interaction, err := h.tracking.ClearProgress(ctx, userID, params.Slug)
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.ClearStageOK{Data: interactionOut(interaction)}, nil
}

// Untrack removes a job from the board: clears saved_at, applied_at, stage and notes,
// keeping viewed_at for the view history.
func (h *trackingHandlers) Untrack(ctx context.Context, params apigen.UntrackParams) (*apigen.UntrackOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	interaction, err := h.tracking.Untrack(ctx, userID, params.Slug)
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.UntrackOK{Data: interactionOut(interaction)}, nil
}

// TrackJob sets the application stage and/or notes (cookie or API key). The service validates
// before the slug lookup, so a bad request never touches the DB: an empty body or unknown
// stage is a 400. A nil field is left unchanged.
func (h *trackingHandlers) TrackJob(ctx context.Context, req *apigen.TrackJob, params apigen.TrackJobParams) (*apigen.TrackJobOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	interaction, err := h.tracking.Track(ctx, userID, params.Slug, ptrOf(req.Stage.Get), ptrOf(req.Notes.Get), appevent.SourceUser)
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.TrackJobOK{Data: interactionOut(interaction)}, nil
}

// TrackApplication is TrackJob for a caller naming the row. The listing mints two id forms:
// the application form goes straight to the application, the slug form takes TrackJob's path.
func (h *trackingHandlers) TrackApplication(ctx context.Context, req *apigen.TrackJob, params apigen.TrackApplicationParams) (*apigen.TrackApplicationOK, error) {
	userID, ref, err := applicationRefFor(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	stage, notes := ptrOf(req.Stage.Get), ptrOf(req.Notes.Get)
	var interaction jobtracking.Interaction
	if ref.AppID != 0 {
		interaction, err = h.tracking.TrackApplication(ctx, userID, ref.AppID, stage, notes, appevent.SourceUser)
	} else {
		interaction, err = h.tracking.Track(ctx, userID, ref.Slug, stage, notes, appevent.SourceUser)
	}
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.TrackApplicationOK{Data: interactionOut(interaction)}, nil
}

// UntrackApplication is Untrack addressed by the row id.
func (h *trackingHandlers) UntrackApplication(ctx context.Context, params apigen.UntrackApplicationParams) (*apigen.UntrackApplicationOK, error) {
	userID, ref, err := applicationRefFor(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	var interaction jobtracking.Interaction
	if ref.AppID != 0 {
		interaction, err = h.tracking.UntrackApplication(ctx, userID, ref.AppID)
	} else {
		interaction, err = h.tracking.Untrack(ctx, userID, ref.Slug)
	}
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.UntrackApplicationOK{Data: interactionOut(interaction)}, nil
}

// ClearApplicationStage is ClearStage addressed by the row id.
func (h *trackingHandlers) ClearApplicationStage(ctx context.Context, params apigen.ClearApplicationStageParams) (*apigen.ClearApplicationStageOK, error) {
	userID, ref, err := applicationRefFor(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	var interaction jobtracking.Interaction
	if ref.AppID != 0 {
		interaction, err = h.tracking.ClearApplicationProgress(ctx, userID, ref.AppID)
	} else {
		interaction, err = h.tracking.ClearProgress(ctx, userID, ref.Slug)
	}
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.ClearApplicationStageOK{Data: interactionOut(interaction)}, nil
}

// applicationRefFor authenticates the caller and reads the row id from the path.
func applicationRefFor(ctx context.Context, id string) (int64, applicationRef, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return 0, applicationRef{}, err
	}
	ref, err := parseApplicationRef(id)
	if err != nil {
		return 0, applicationRef{}, err
	}
	return userID, ref, nil
}
