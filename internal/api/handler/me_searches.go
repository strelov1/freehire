package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/savedsearch"
)

// savedSearchHandlers serves the per-user saved searches: list/create/update/delete
// named filter snapshots. The use cases live in savedsearch.Service.
type savedSearchHandlers struct {
	savedSearch *savedsearch.Service
}

func newSavedSearchHandlers(queries *db.Queries) *savedSearchHandlers {
	return &savedSearchHandlers{savedSearch: savedsearch.New(savedsearch.NewQueriesRepository(queries))}
}

func (h *savedSearchHandlers) register(api fiber.Router, mw middleware) {
	// Saved searches are cookie-only (RequireAuth) like API-key management: they are a
	// browser convenience (the "My filters" picker), not a scripting primitive. Each
	// operation is owner-scoped; an id that is not the caller's is a 404.
	api.Get("/me/searches", mw.cookie, h.ListSavedSearches)
	api.Post("/me/searches", mw.cookie, h.CreateSavedSearch)
	api.Patch("/me/searches/:id", mw.cookie, h.UpdateSavedSearch)
	api.Delete("/me/searches/:id", mw.cookie, h.DeleteSavedSearch)
}

// savedSearchResponse is the public shape of a saved search. user_id is omitted
// (ownership, internal); query is the canonical search query string the SPA replays
// into the filter URL.
type savedSearchResponse struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Query              string     `json:"query"`
	DerivedFromProfile bool       `json:"derived_from_profile"`
	CreatedAt          *time.Time `json:"created_at"`
	UpdatedAt          *time.Time `json:"updated_at"`
}

// toSavedSearchResponse maps a stored saved search to its wire shape (no user id).
func toSavedSearchResponse(s savedsearch.SavedSearch) savedSearchResponse {
	return savedSearchResponse{
		ID:                 s.ID,
		Name:               s.Name,
		Query:              s.Query,
		DerivedFromProfile: s.DerivedFromProfile,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
	}
}

// savedSearchError maps the saved-search sentinels onto HTTP statuses: a bad name is a
// 400, a duplicate name or the per-user cap is a 409, a missing/non-owned row is a 404.
// Anything else falls through to RenderError as a 500.
//
// Every sentinel the package declares belongs here, and a test walks the list: one left
// out does not merely render the wrong status, it tells the caller their own mistake was
// our fault and files a fault report for ordinary traffic.
func savedSearchError(err error) error {
	switch {
	case errors.Is(err, savedsearch.ErrInvalidName):
		return fiber.NewError(fiber.StatusBadRequest, "name must be 1-100 characters")
	case errors.Is(err, savedsearch.ErrDuplicateName):
		return fiber.NewError(fiber.StatusConflict, "a saved search with this name already exists")
	case errors.Is(err, savedsearch.ErrCapExceeded):
		return fiber.NewError(fiber.StatusConflict, "saved-search limit reached")
	case errors.Is(err, savedsearch.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "saved search not found")
	case errors.Is(err, savedsearch.ErrQueryTooLong):
		return fiber.NewError(fiber.StatusBadRequest, "query is too long")
	case errors.Is(err, savedsearch.ErrProfileSearchExists):
		return fiber.NewError(fiber.StatusConflict, "a profile-derived search already exists")
	default:
		return err
	}
}

// createSavedSearchRequest is the create body: a required display name and the canonical
// search query string (an empty query is the valid "show all" snapshot).
type createSavedSearchRequest struct {
	Name               string `json:"name"`
	Query              string `json:"query"`
	DerivedFromProfile bool   `json:"derived_from_profile"`
}

// updateSavedSearchRequest is the partial-update body: a nil field is left unchanged, so a
// caller can rename, overwrite the filters, or both. An empty (non-nil) query is a real
// "show all" value.
type updateSavedSearchRequest struct {
	Name  *string `json:"name"`
	Query *string `json:"query"`
}

// CreateSavedSearch stores a named filter snapshot for the authenticated user. Behind
// RequireAuth (cookie-only): saved searches are a browser feature, not a scripting
// primitive. A bad name is a 400, a duplicate name or the per-user cap is a 409.
func (h *savedSearchHandlers) CreateSavedSearch(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in createSavedSearchRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	saved, err := h.savedSearch.Create(c.Context(), userID, in.Name, in.Query, in.DerivedFromProfile)
	if err != nil {
		return savedSearchError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toSavedSearchResponse(saved)})
}

// ListSavedSearches returns the authenticated user's saved searches, most recently updated
// first. Owner-scoped, so it never reveals another user's. Cookie-only.
func (h *savedSearchHandlers) ListSavedSearches(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	rows, err := h.savedSearch.List(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]savedSearchResponse, len(rows))
	for i, r := range rows {
		out[i] = toSavedSearchResponse(r)
	}
	return c.JSON(fiber.Map{"data": out, "meta": fiber.Map{"total": len(out)}})
}

// UpdateSavedSearch overwrites a saved search's name and/or query (partial), scoped to its
// owner. A missing or non-owned id is a 404; a bad name is a 400; a name collision is a 409.
func (h *savedSearchHandlers) UpdateSavedSearch(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	var in updateSavedSearchRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	saved, err := h.savedSearch.Update(c.Context(), userID, id, in.Name, in.Query)
	if err != nil {
		return savedSearchError(err)
	}
	return c.JSON(fiber.Map{"data": toSavedSearchResponse(saved)})
}

// DeleteSavedSearch removes one of the authenticated user's saved searches by id.
// Owner-scoped: an id that does not exist or belongs to another user is a 404. Cookie-only.
func (h *savedSearchHandlers) DeleteSavedSearch(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	if err := h.savedSearch.Delete(c.Context(), userID, id); err != nil {
		return savedSearchError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
