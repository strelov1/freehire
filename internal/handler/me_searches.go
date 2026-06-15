package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/savedsearch"
)

// savedSearchResponse is the public shape of a saved search. user_id is omitted
// (ownership, internal); query is the canonical search query string the SPA replays
// into the filter URL.
type savedSearchResponse struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Query     string     `json:"query"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// toSavedSearchResponse maps a stored saved search to its wire shape (no user id).
func toSavedSearchResponse(s db.SavedSearch) savedSearchResponse {
	return savedSearchResponse{
		ID:        s.ID,
		Name:      s.Name,
		Query:     s.Query,
		CreatedAt: timePtr(s.CreatedAt),
		UpdatedAt: timePtr(s.UpdatedAt),
	}
}

// savedSearchError maps the saved-search sentinels onto HTTP statuses: a bad name is a
// 400, a duplicate name or the per-user cap is a 409, a missing/non-owned row is a 404.
// Anything else falls through to RenderError as a 500.
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
	default:
		return err
	}
}

// createSavedSearchRequest is the create body: a required display name and the canonical
// search query string (an empty query is the valid "show all" snapshot).
type createSavedSearchRequest struct {
	Name  string `json:"name"`
	Query string `json:"query"`
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
func (a *API) CreateSavedSearch(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	var in createSavedSearchRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	saved, err := a.savedSearch.Create(c.Context(), userID, in.Name, in.Query)
	if err != nil {
		return savedSearchError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toSavedSearchResponse(saved)})
}

// ListSavedSearches returns the authenticated user's saved searches, most recently updated
// first. Owner-scoped, so it never reveals another user's. Cookie-only.
func (a *API) ListSavedSearches(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	rows, err := a.savedSearch.List(c.Context(), userID)
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
func (a *API) UpdateSavedSearch(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid saved-search id")
	}

	var in updateSavedSearchRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	saved, err := a.savedSearch.Update(c.Context(), userID, int64(id), in.Name, in.Query)
	if err != nil {
		return savedSearchError(err)
	}
	return c.JSON(fiber.Map{"data": toSavedSearchResponse(saved)})
}

// DeleteSavedSearch removes one of the authenticated user's saved searches by id.
// Owner-scoped: an id that does not exist or belongs to another user is a 404. Cookie-only.
func (a *API) DeleteSavedSearch(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid saved-search id")
	}

	if err := a.savedSearch.Delete(c.Context(), userID, int64(id)); err != nil {
		return savedSearchError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
