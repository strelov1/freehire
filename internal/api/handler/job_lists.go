package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/application/joblists"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// jobListHandlers serves the per-user job lists (create/rename/delete a named set of
// jobs, add/remove membership, publish/unpublish as a public page) plus the public
// read of a shared list by its slug. The use cases live in joblists.Service.
type jobListHandlers struct {
	joblists *joblists.Service
}

func newJobListHandlers(queries *db.Queries) *jobListHandlers {
	return &jobListHandlers{joblists: joblists.New(joblists.NewQueriesRepository(queries))}
}

func (h *jobListHandlers) register(api fiber.Router, mw middleware) {
	// Public read of a shared job list by its slug — unauthenticated, like the
	// public board read it replaces. Owner identity is never exposed (see
	// publicJobListResponse).
	api.Get("/lists/:slug", h.GetPublicList)

	// Job lists are cookie-only (RequireAuth), like saved searches: a browser
	// convenience, not a scripting primitive. Each operation is owner-scoped; an id
	// that is not the caller's is a 404.
	api.Get("/me/lists", mw.cookie, h.ListJobLists)
	api.Get("/me/lists/membership", mw.cookie, h.ListJobListMembership)
	api.Post("/me/lists", mw.cookie, h.CreateJobList)
	api.Patch("/me/lists/:id", mw.cookie, h.UpdateJobList)
	api.Delete("/me/lists/:id", mw.cookie, h.DeleteJobList)
	api.Post("/me/lists/:id/jobs", mw.cookie, h.AddJobToList)
	api.Delete("/me/lists/:id/jobs/:job_slug", mw.cookie, h.RemoveJobFromList)
	api.Post("/me/lists/:id/share", mw.cookie, h.ShareJobList)
	api.Delete("/me/lists/:id/share", mw.cookie, h.UnshareJobList)
}

// jobListResponse is the public shape of a job list. user_id is omitted (ownership,
// internal). JobCount is populated on the list endpoint only.
type jobListResponse struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	PublicSlug  string     `json:"public_slug"` // empty when the list is private (not shared)
	JobCount    int64      `json:"job_count"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

// toJobListResponse maps a stored job list to its wire shape (no user id).
func toJobListResponse(l joblists.JobList) jobListResponse {
	return jobListResponse{
		ID:          l.ID,
		Name:        l.Name,
		Description: l.Description,
		PublicSlug:  l.PublicSlug,
		JobCount:    l.JobCount,
		CreatedAt:   l.CreatedAt,
		UpdatedAt:   l.UpdatedAt,
	}
}

// publicJobListResponse is the public wire shape of a shared job list: only its
// display fields and jobs. It deliberately omits every owner-identifying column
// (user id, email) so publishing a list exposes no account PII.
type publicJobListResponse struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Jobs        []jobview.Card `json:"jobs"`
}

// toPublicJobListResponse maps a public-list read to its wire shape.
func toPublicJobListResponse(l joblists.PublicJobList) publicJobListResponse {
	jobs := l.Jobs
	if jobs == nil {
		jobs = []jobview.Card{}
	}
	return publicJobListResponse{Name: l.Name, Description: l.Description, Jobs: jobs}
}

// jobListError maps the job-list sentinels onto HTTP statuses: a bad name/description
// is a 400, a duplicate name or the per-user cap is a 409, a missing/non-owned list or
// an unknown job is a 404. Anything else falls through to RenderError as a 500.
func jobListError(err error) error {
	switch {
	case errors.Is(err, joblists.ErrInvalidName):
		return fiber.NewError(fiber.StatusBadRequest, "name must be 1-100 characters")
	case errors.Is(err, joblists.ErrInvalidDescription):
		return fiber.NewError(fiber.StatusBadRequest, "description must be at most 2000 characters")
	case errors.Is(err, joblists.ErrDuplicateName):
		return fiber.NewError(fiber.StatusConflict, "a job list with this name already exists")
	case errors.Is(err, joblists.ErrCapExceeded):
		return fiber.NewError(fiber.StatusConflict, "job-list limit reached")
	case errors.Is(err, joblists.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "job list not found")
	case errors.Is(err, joblists.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "job not found")
	case errors.Is(err, joblists.ErrListFull):
		return fiber.NewError(fiber.StatusConflict, "job-list is at its job limit")
	default:
		return err
	}
}

// createJobListRequest is the create body: a required display name and an optional
// description.
type createJobListRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// updateJobListRequest is the partial-update body: a nil field is left unchanged, so a
// caller can rename, edit the description, or both.
type updateJobListRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// addJobToListRequest is the add-job body. The job is addressed by its public slug,
// the wire identifier every job carries (save/unsave, tracking, ... all use it too).
type addJobToListRequest struct {
	JobSlug string `json:"job_slug"`
}

// CreateJobList stores a named job list for the authenticated user. Behind
// RequireAuth (cookie-only). A bad name/description is a 400, a duplicate name or the
// per-user cap is a 409.
func (h *jobListHandlers) CreateJobList(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in createJobListRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	list, err := h.joblists.Create(c.Context(), userID, in.Name, in.Description)
	if err != nil {
		return jobListError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toJobListResponse(list)})
}

// listMembershipResponse is one of the caller's lists, flagged with whether a given
// job (resolved from the `job_slug` query param) already belongs to it — what the
// job card's "Add to list" control reads to render its toggle state.
type listMembershipResponse struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	InList bool   `json:"in_list"`
}

// ListJobListMembership reports, for every one of the authenticated user's lists,
// whether the job named by the required `job_slug` query param already belongs to
// it. An unknown slug is a 404. Cookie-only.
func (h *jobListHandlers) ListJobListMembership(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	slug := c.Query("job_slug")
	if slug == "" {
		return fiber.NewError(fiber.StatusBadRequest, "job_slug is required")
	}

	rows, err := h.joblists.ListMembership(c.Context(), userID, slug)
	if err != nil {
		return jobListError(err)
	}
	out := make([]listMembershipResponse, len(rows))
	for i, r := range rows {
		out[i] = listMembershipResponse{ID: r.ID, Name: r.Name, InList: r.InList}
	}
	return c.JSON(fiber.Map{"data": out})
}

// ListJobLists returns the authenticated user's job lists, most recently updated
// first. Owner-scoped, so it never reveals another user's. Cookie-only.
func (h *jobListHandlers) ListJobLists(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	rows, err := h.joblists.List(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]jobListResponse, len(rows))
	for i, r := range rows {
		out[i] = toJobListResponse(r)
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"total": len(out)}})
}

// UpdateJobList overwrites a job list's name and/or description (partial), scoped to
// its owner. A missing or non-owned id is a 404; a bad name/description is a 400; a
// name collision is a 409.
func (h *jobListHandlers) UpdateJobList(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	var in updateJobListRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	list, err := h.joblists.Update(c.Context(), userID, id, in.Name, in.Description)
	if err != nil {
		return jobListError(err)
	}
	return c.JSON(fiber.Map{"data": toJobListResponse(list)})
}

// DeleteJobList removes one of the authenticated user's job lists by id. Owner-scoped:
// an id that does not exist or belongs to another user is a 404. Cookie-only.
func (h *jobListHandlers) DeleteJobList(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	if err := h.joblists.Delete(c.Context(), userID, id); err != nil {
		return jobListError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// AddJobToList adds a job to one of the authenticated user's lists. Owner-scoped: a
// missing/non-owned list id is a 404, an unknown job slug is a 404. Idempotent: adding
// an already-present job succeeds without duplicating membership.
func (h *jobListHandlers) AddJobToList(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	var in addJobToListRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if err := h.joblists.AddJob(c.Context(), userID, id, in.JobSlug); err != nil {
		return jobListError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RemoveJobFromList removes a job from one of the authenticated user's lists.
// Owner-scoped: a missing/non-owned list id is a 404. Idempotent: removing an absent
// job, or a slug that resolves to no job at all, succeeds without error.
func (h *jobListHandlers) RemoveJobFromList(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	if err := h.joblists.RemoveJob(c.Context(), userID, id, c.Params("job_slug")); err != nil {
		return jobListError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ShareJobList publishes one of the authenticated user's job lists as a public,
// read-only page, minting (or keeping) its slug. Owner-scoped and cookie-only: a
// missing/non-owned id is a 404. Returns the updated job list (now carrying
// public_slug).
func (h *jobListHandlers) ShareJobList(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	list, err := h.joblists.Share(c.Context(), userID, id)
	if err != nil {
		return jobListError(err)
	}
	return c.JSON(fiber.Map{"data": toJobListResponse(list)})
}

// UnshareJobList makes one of the authenticated user's shared lists private again.
// Owner-scoped and cookie-only; idempotent (already-private is a no-op), a
// missing/non-owned id is a 404.
func (h *jobListHandlers) UnshareJobList(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	if err := h.joblists.Unshare(c.Context(), userID, id); err != nil {
		return jobListError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// GetPublicList serves a shared job list by its public slug — unauthenticated, no
// owner-scoping. An unknown or unshared slug is a 404 (mapped from the job-list
// not-found sentinel).
func (h *jobListHandlers) GetPublicList(c *fiber.Ctx) error {
	list, err := h.joblists.GetPublicList(c.Context(), c.Params("slug"))
	if err != nil {
		return jobListError(err)
	}
	return c.JSON(fiber.Map{"data": toPublicJobListResponse(list)})
}
