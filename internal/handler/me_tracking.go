package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobview"
)

// myJobResponse is one item of the my-jobs listing: the job in the shared
// jobview wire shape with the caller's interaction timestamps riding alongside
// (not flattened in — the job shape stays identical to every other job surface).
type myJobResponse struct {
	// ID addresses the row. The board keys, routes and opens by it rather than by the
	// posting's slug, which an application whose posting was pruned does not have.
	ID string `json:"id"`
	// Company and RoleTitle ride on every row, read from the application record, so a
	// card renders whether or not a posting is still there.
	Company   string `json:"company_slug"`
	RoleTitle string `json:"role_title"`
	// Job is null once the catalogue has removed the posting. The application is a fact
	// about the candidate's life and outlives our inventory of it.
	Job            *jobview.Job `json:"job"`
	ViewedAt       *time.Time   `json:"viewed_at"`
	SavedAt        *time.Time   `json:"saved_at"`
	AppliedAt      *time.Time   `json:"applied_at"`
	Stage          *string      `json:"stage"`
	Notes          *string      `json:"notes"`
	EmailCount     int          `json:"email_count"`
	ReminderFireAt *time.Time   `json:"reminder_fire_at"`
	// The silence fields are null together on any row that is not an application
	// awaiting a reply — a job merely viewed or saved, or one in a settled stage.
	// Null means "nothing is owed here", which the board must be able to tell
	// apart from "owed and answered promptly".
	LastActivityAt *time.Time `json:"last_activity_at"`
	DaysSilent     *int       `json:"days_silent"`
	SilenceState   *string    `json:"silence_state"`
	// FollowedUpAt is when the caller last recorded chasing, or null for never. It is
	// independent of the silence fields above — a chased application stays silent —
	// so the board can show both readings at once.
	FollowedUpAt *time.Time `json:"followed_up_at"`
	// CVOpenedAt is when somebody last opened a CV the caller sent for this job, or null. Like
	// FollowedUpAt it sits beside the silence fields and not inside them: a recruiter reading a CV
	// is not a reply, and the card shows both — still unanswered, and read yesterday.
	CVOpenedAt *time.Time `json:"cv_opened_at"`
}

// ListTrackedJobs returns the authenticated user's job interactions joined with the
// jobs, most recently touched first, narrowed by ?filter=all|viewed|saved|applied|board
// (default all; viewed is the view-only subset — neither saved nor applied;
// board is the Kanban view — jobs with saved_at, applied_at, or stage set).
// meta carries total/limit/offset for the active filter plus the per-filter
// counts for the tab badges — which is also why this writes its own envelope
// instead of listResponse. Closed jobs stay listed: a user's history must not
// shrink when a posting closes.
func (h *trackingHandlers) ListTrackedJobs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	limit, offset := pageParamsBounded(c, defaultLimit, trackingMaxLimit)
	listing, err := h.tracking.ListTracked(c.Context(), userID, c.Query("filter"), int32(limit), int32(offset))
	if err != nil {
		return trackingError(err)
	}

	// One clock for the whole page: silence is derived against now(), and two rows
	// read a moment apart must not disagree about what "now" was.
	now := time.Now()
	items := make([]myJobResponse, 0, len(listing.Items))
	for _, it := range listing.Items {
		item := myJobResponse{
			ID:             it.ID,
			Company:        it.CompanySlug,
			RoleTitle:      it.RoleTitle,
			Job:            it.Job,
			ViewedAt:       it.ViewedAt,
			SavedAt:        it.SavedAt,
			AppliedAt:      it.AppliedAt,
			Stage:          it.Stage,
			Notes:          it.Notes,
			EmailCount:     it.EmailCount,
			ReminderFireAt: it.ReminderFireAt,
			FollowedUpAt:   it.FollowedUpAt,
			CVOpenedAt:     it.CVOpenedAt,
		}
		if s := it.Silence(now); s != nil {
			item.LastActivityAt = &s.LastActivityAt
			item.DaysSilent = &s.DaysSilent
			item.SilenceState = &s.State
		}
		items = append(items, item)
	}

	return c.JSON(fiber.Map{
		"data": items,
		"meta": fiber.Map{
			"total":  listing.Total(),
			"limit":  limit,
			"offset": offset,
			"counts": fiber.Map{
				"all":       listing.Counts.All,
				"viewed":    listing.Counts.Viewed,
				"saved":     listing.Counts.Saved,
				"applied":   listing.Counts.Applied,
				"board":     listing.Counts.Board,
				"dismissed": listing.Counts.Dismissed,
			},
		},
	})
}

// TrackingPipeline returns the authenticated caller's application-pipeline snapshot:
// the total application count and the count at each stage of the vocabulary, aggregated
// server-side over all of the caller's applications. The SPA Pipeline tab groups those
// stages with the generated STAGE_GROUPS to draw the Sankey, and derives the
// interview/offer rate cards from the same counts.
func (h *trackingHandlers) TrackingPipeline(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	pipeline, err := h.tracking.Pipeline(c.Context(), userID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": pipeline})
}

// ListViewedSlugs returns the set of public job slugs the authenticated caller
// has interacted with (every user_jobs row counts as viewed). The SPA reads this
// to dim already-seen cards in the browse list and search results without
// authenticating the public job-read path — viewed state is cross-referenced
// client-side, never joined into ListJobs/SearchJobs. The response is a flat
// {"data": [slug, ...]} list scoped to the caller.
func (h *trackingHandlers) ListViewedSlugs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	slugs, err := h.tracking.ViewedSlugs(c.Context(), userID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": slugs})
}

// ListSavedSlugs returns the set of public job slugs the authenticated caller has
// saved (bookmarked). The SPA reads this to render the save toggle as filled on
// already-saved cards in the browse list and search results without
// authenticating the public job-read path — saved state is cross-referenced
// client-side, never joined into ListJobs/SearchJobs. The response is a flat
// {"data": [slug, ...]} list scoped to the caller.
func (h *trackingHandlers) ListSavedSlugs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	slugs, err := h.tracking.SavedSlugs(c.Context(), userID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": slugs})
}

// ListDismissedSlugs returns the set of public job slugs the authenticated caller
// has hidden (dismissed). The SPA reads this to exclude hidden jobs from the
// browse feed client-side, mirroring ListSavedSlugs — dismissed state is
// cross-referenced client-side, never joined into ListJobs/SearchJobs. The
// response is a flat {"data": [slug, ...]} list scoped to the caller.
func (h *trackingHandlers) ListDismissedSlugs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	slugs, err := h.tracking.DismissedSlugs(c.Context(), userID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": slugs})
}
