package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/engage/reminder"
	"github.com/strelov1/freehire/internal/job/applydate"
	"github.com/strelov1/freehire/internal/platform/db"
)

// trackingHandlers serves the per-user job interactions (view/apply/save/dismiss/
// track), the user-scoped tracking reads (the Kanban listing, slug sets, pipeline,
// swipe deck), and the saved-job reminder controls. The interaction use cases live
// in jobtracking.Service, which also owns the reminder side effect of save/unsave/
// apply — reminder.Service is handed to it as a port, and is held here only for the
// notification-settings endpoints. The cmd/remind worker fires the scheduled
// reminders.
type trackingHandlers struct {
	tracking *jobtracking.Service
	reminder *reminder.Service
	search   searcher
	// queries backs the tracker's own ghost-signal attach step (me_tracking.go), the
	// same reason jobsHandlers/searchHandlers each hold one: ghostEvidenceFor and
	// ListJobGhostStamps need it directly, not through the tracking service (which
	// stays search/ghost-agnostic — see openspec/changes/tracker-ghost-badge).
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
	// Per-user job interactions accept either the session cookie or an API key
	// (RequireAuthOrKey), so a script holding a key can drive the same flow as the
	// browser. Jobs are addressed by their public slug; the handlers resolve it to
	// the internal id before writing user_jobs. All writes are idempotent upserts.
	api.Post("/jobs/:slug/view", mw.key, contract)
	api.Post("/jobs/:slug/apply", mw.key, contract)
	api.Post("/jobs/:slug/save", mw.key, contract)
	api.Delete("/jobs/:slug/save", mw.key, contract)
	api.Post("/jobs/:slug/dismiss", mw.key, contract)
	api.Delete("/jobs/:slug/dismiss", mw.key, contract)
	api.Patch("/jobs/:slug/track", mw.key, contract)
	api.Delete("/jobs/:slug/stage", mw.key, contract)
	api.Delete("/jobs/:slug/track", mw.key, contract)

	// The same three writes, addressed by the row id the tracking listing served.
	// The board holds row ids, not slugs, and an application whose posting was
	// deleted has no slug to hold — so the routes above cannot move its card.
	//
	// Its own namespace rather than /me/tracking/:id: that path is already mounted
	// (see the inbox handlers) addressed by a posting slug, and one segment meaning
	// different things depending on the method is a trap for the next reader.
	//
	// The slug-addressed routes stay: freehire-cli and freehire-mcp name postings.
	api.Patch("/me/applications/:id", mw.key, contract)
	api.Delete("/me/applications/:id", mw.key, contract)
	api.Delete("/me/applications/:id/stage", mw.key, contract)

	// User-scoped reads live under /me (consistent with /auth/me): the tracking
	// listing joins the caller's interactions with the jobs they touch, viewed-slugs
	// lets the SPA dim already-seen cards without authenticating the public browse
	// list, and analyses lists the jobs the caller has run the AI fit analysis on.
	api.Get("/me/tracking", mw.key, contract)
	api.Get("/me/tracking/viewed", mw.key, contract)
	api.Get("/me/tracking/saved", mw.key, contract)
	api.Get("/me/tracking/dismissed", mw.key, contract)
	api.Get("/me/tracking/applied-companies", mw.key, contract)
	api.Get("/me/tracking/pipeline", mw.key, contract)
	api.Get("/me/tracking/swipe", mw.key, h.SwipeDeck)

	// The account-level notification rule (enable, channels) shared by saved-job
	// reminders and both lifecycle nudges. Cookie-only (RequireAuth) like
	// subscriptions — it configures a delivery preference.
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

// interactionOut is the public shape of a user's interaction with a job. It omits user_id
// (the caller is the user) and carries saved_at/applied_at/stage/notes as null until the job
// is saved, applied to, or tracked.
func interactionOut(i jobtracking.Interaction) apigen.Interaction {
	return apigen.Interaction{
		JobID: i.JobID, ViewedAt: nilDateTime(i.ViewedAt), SavedAt: nilDateTime(i.SavedAt),
		AppliedAt: nilDateTime(i.AppliedAt), DismissedAt: nilDateTime(i.DismissedAt),
		Stage: nilString(i.Stage), Notes: nilString(i.Notes),
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
	case errors.Is(err, jobtracking.ErrApplicationNotFound):
		return fiber.NewError(fiber.StatusNotFound, "application not found")
	case errors.Is(err, applydate.ErrOutOfRange):
		// The message names which bound was crossed; it is the service's words, not a second
		// copy of the rule stated here.
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
// The body is optional in both directions: absent, empty, or carrying no date, the request is
// the undated apply it has always been, stamped now(). A date present is the caller's own
// account of when they applied, and overrides one already recorded.
//
// A body that is not JSON, or whose `applied_on` is not a string, is refused by the contract
// before this runs. Treating those as "no date" would stamp today for a caller who named a day
// and tell them it worked — the row would then record a different application than the one they
// described, silently.
func (h *trackingHandlers) MarkApplied(ctx context.Context, req apigen.OptApplyJob, params apigen.MarkAppliedParams) (*apigen.MarkAppliedOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	day, err := statedApplyDay(req)
	if err != nil {
		return nil, err
	}
	// The two paths differ only in which date the application takes; everything after is the
	// same apply, so they rejoin immediately rather than each carrying their own tail.
	var interaction jobtracking.Interaction
	if day != nil {
		// The believable-date window belongs to the service, so an out-of-range day arrives
		// as an error to render rather than as a rule restated here.
		interaction, err = h.tracking.MarkAppliedOn(
			ctx, userID, params.Slug, *day, time.Now().UTC(), appevent.SourceUser)
	} else {
		interaction, err = h.tracking.MarkApplied(ctx, userID, params.Slug, appevent.SourceUser)
	}
	if err != nil {
		return nil, trackingError(err)
	}
	return &apigen.MarkAppliedOK{Data: interactionOut(interaction)}, nil
}

// statedApplyDay reads the optional day out of the request. It answers nil only when the caller
// sent nothing at all, or sent a body carrying no `applied_on`: apply has always been callable
// with an empty request, and a client sending none is asking for today rather than making a
// mistake.
//
// The day is returned as a day. Placing it at the storage hour is the service's job, because the
// believable-date window is checked against the day and would refuse "today" all morning if it
// were checked against the derived instant.
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

// SaveJob saves (bookmarks) a job for the authenticated user and returns the
// updated interaction. The account's shared notification rule decides whether a
// reminder is scheduled.
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

// UnsaveJob clears a job's saved mark for the authenticated user. The interaction
// row (view/apply history) survives; if no row exists at all, unsaving is a no-op
// that answers with the zero interaction state — DELETE is idempotent, so "already
// not saved" is success, not an error (the service resolves that case).
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

// DismissJob marks a job dismissed (swiped away) for the authenticated user and
// returns the updated interaction. Dismissal only keeps the job out of the swipe
// deck; it stays visible in the public /jobs list and search.
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

// UndismissJob clears a job's dismissed mark for the authenticated user. The
// interaction row survives; if no row exists at all, undismissing is a no-op that
// answers with the zero interaction state — DELETE is idempotent, so "already not
// dismissed" is success, not an error (the service resolves that case).
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

// ClearStage drops a job's pipeline progress (stage and applied_at) for the
// authenticated user while keeping saved_at, viewed_at, and notes intact. Used
// when dragging a Kanban card back to the "Saved" column.
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

// Untrack removes a job from the board for the authenticated user: clears
// saved_at, applied_at, stage, and notes while keeping viewed_at so the job
// stays in the user's view history.
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

// TrackJob sets the application stage and/or notes for the authenticated user's
// interaction with a job (session cookie or API key). The body is validated by
// the service before the slug lookup, so a bad request never touches the DB: an
// empty body or an unknown stage is a 400. A nil field is left unchanged by the
// upsert. Returns the updated interaction.
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

// TrackApplication is TrackJob for a caller naming the row rather than the posting.
// The listing mints two id forms and this accepts both: the application form goes
// straight to the application, the slug form takes the path TrackJob already takes.
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
