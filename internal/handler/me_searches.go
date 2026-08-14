package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/savedsearch"
)

// savedSearchHandlers serves the per-user saved searches (list/create/update/delete
// named filter snapshots, publish/unpublish as a public board) plus the public read
// of a shared board by its slug. The use cases live in savedsearch.Service.
type savedSearchHandlers struct {
	savedSearch *savedsearch.Service
}

func newSavedSearchHandlers(queries *db.Queries) *savedSearchHandlers {
	return &savedSearchHandlers{savedSearch: savedsearch.New(savedsearch.NewQueriesRepository(queries))}
}

func (h *savedSearchHandlers) register(api fiber.Router, mw middleware) {
	// Public read of a shared saved-search "board" by its slug — unauthenticated, like
	// the job/company reads. Owner identity is never exposed (see boardResponse).
	api.Get("/boards/:slug", h.GetBoard)

	// Saved searches are cookie-only (RequireAuth) like API-key management: they are a
	// browser convenience (the "My filters" picker), not a scripting primitive. Each
	// operation is owner-scoped; an id that is not the caller's is a 404.
	api.Get("/me/searches", mw.cookie, h.ListSavedSearches)
	api.Post("/me/searches", mw.cookie, h.CreateSavedSearch)
	api.Patch("/me/searches/:id", mw.cookie, h.UpdateSavedSearch)
	api.Delete("/me/searches/:id", mw.cookie, h.DeleteSavedSearch)
	// Publish/unpublish a saved search as a public board. Cookie-only (same as the rest
	// of /me/searches); the public read is GET /boards/:slug above.
	api.Post("/me/searches/:id/share", mw.cookie, h.ShareSavedSearch)
	api.Delete("/me/searches/:id/share", mw.cookie, h.UnshareSavedSearch)
}

// savedSearchResponse is the public shape of a saved search. user_id is omitted
// (ownership, internal); query is the canonical search query string the SPA replays
// into the filter URL.
type savedSearchResponse struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Query              string     `json:"query"`
	PublicSlug         string     `json:"public_slug"`  // empty when the set is private (not shared)
	AuthorLabel        string     `json:"author_label"` // empty when the board is anonymous
	DerivedFromProfile bool       `json:"derived_from_profile"`
	CreatedAt          *time.Time `json:"created_at"`
	UpdatedAt          *time.Time `json:"updated_at"`
}

// toSavedSearchResponse maps a stored saved search to its wire shape (no user id).
// PublicSlug/AuthorLabel are empty strings when the board is private / anonymous.
func toSavedSearchResponse(s savedsearch.SavedSearch) savedSearchResponse {
	return savedSearchResponse{
		ID:                 s.ID,
		Name:               s.Name,
		Query:              s.Query,
		PublicSlug:         s.PublicSlug,
		AuthorLabel:        s.AuthorLabel,
		DerivedFromProfile: s.DerivedFromProfile,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
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
	case errors.Is(err, savedsearch.ErrInvalidAuthorLabel):
		return fiber.NewError(fiber.StatusBadRequest, "author label must be at most 60 characters")
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

// shareSavedSearchRequest is the share body: an optional author label shown on the public
// board (blank/omitted renders the board anonymously).
type shareSavedSearchRequest struct {
	AuthorLabel string `json:"author_label"`
}

// ShareSavedSearch publishes one of the authenticated user's saved searches as a public
// board, minting (or keeping) its slug and setting the optional author label. Owner-scoped
// and cookie-only: a missing/non-owned id is a 404, an over-long label is a 400. Returns the
// updated saved search (now carrying public_slug).
func (h *savedSearchHandlers) ShareSavedSearch(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	var in shareSavedSearchRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	saved, err := h.savedSearch.Share(c.Context(), userID, id, in.AuthorLabel)
	if err != nil {
		return savedSearchError(err)
	}
	return c.JSON(fiber.Map{"data": toSavedSearchResponse(saved)})
}

// UnshareSavedSearch makes one of the authenticated user's shared boards private again.
// Owner-scoped and cookie-only; idempotent (already-private is a no-op), a missing/non-owned
// id is a 404.
func (h *savedSearchHandlers) UnshareSavedSearch(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	if err := h.savedSearch.Unshare(c.Context(), userID, id); err != nil {
		return savedSearchError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
