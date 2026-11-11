package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
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

func (h *savedSearchHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Saved searches are cookie-only (RequireAuth) like API-key management: they are a
	// browser convenience (the "My filters" picker), not a scripting primitive. Each
	// operation is owner-scoped; an id that is not the caller's is a 404. The handlers
	// below implement the contract (typespec/alerts.tsp).
	api.Get("/me/searches", mw.cookie, contract)
	api.Post("/me/searches", mw.cookie, contract)
	api.Patch("/me/searches/:id", mw.cookie, contract)
	api.Delete("/me/searches/:id", mw.cookie, contract)
}

// setNilDateTime fills a `utcDateTime | null` field that is always present: null when the
// row carries no stamp.
func setNilDateTime(dst *apigen.NilDateTime, at *time.Time) {
	if at != nil {
		*dst = apigen.NewNilDateTime(*at)
		return
	}
	dst.SetToNull()
}

// savedSearchOut maps a stored saved search to its wire shape. user_id is omitted
// (ownership, internal); query is the canonical search query string the SPA replays
// into the filter URL.
func savedSearchOut(s savedsearch.SavedSearch) apigen.SavedSearch {
	out := apigen.SavedSearch{ID: s.ID, Name: s.Name, Query: s.Query, DerivedFromProfile: s.DerivedFromProfile}
	setNilDateTime(&out.CreatedAt, s.CreatedAt)
	setNilDateTime(&out.UpdatedAt, s.UpdatedAt)
	return out
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

// CreateSavedSearch stores a named filter snapshot for the authenticated user. Behind
// RequireAuth (cookie-only): saved searches are a browser feature, not a scripting
// primitive. A bad name is a 400, a duplicate name or the per-user cap is a 409. The body is
// a required display name and the canonical search query string (an empty query is the valid
// "show all" snapshot).
func (h *savedSearchHandlers) CreateSavedSearch(ctx context.Context, req *apigen.SavedSearchCreate) (*apigen.CreateSavedSearchCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	saved, err := h.savedSearch.Create(ctx, userID, req.Name.Value, req.Query.Value, req.DerivedFromProfile.Value)
	if err != nil {
		return nil, savedSearchError(err)
	}
	return &apigen.CreateSavedSearchCreated{Data: savedSearchOut(saved)}, nil
}

// ListSavedSearches returns the authenticated user's saved searches, most recently updated
// first. Owner-scoped, so it never reveals another user's. Cookie-only.
func (h *savedSearchHandlers) ListSavedSearches(ctx context.Context) (*apigen.SavedSearchList, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.savedSearch.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.SavedSearch, len(rows))
	for i, r := range rows {
		out[i] = savedSearchOut(r)
	}
	return &apigen.SavedSearchList{Data: out, Meta: apigen.AlertListMeta{Total: int64(len(out))}}, nil
}

// UpdateSavedSearch overwrites a saved search's name and/or query (partial), scoped to its
// owner. A missing or non-owned id is a 404; a bad name is a 400; a name collision is a 409.
// An absent or null field is left unchanged, so a caller can rename, overwrite the filters,
// or both; an empty (non-null) query is a real "show all" value.
func (h *savedSearchHandlers) UpdateSavedSearch(ctx context.Context, req *apigen.SavedSearchUpdate, params apigen.UpdateSavedSearchParams) (*apigen.UpdateSavedSearchOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	saved, err := h.savedSearch.Update(ctx, userID, id, ptrOf(req.Name.Get), ptrOf(req.Query.Get))
	if err != nil {
		return nil, savedSearchError(err)
	}
	return &apigen.UpdateSavedSearchOK{Data: savedSearchOut(saved)}, nil
}

// DeleteSavedSearch removes one of the authenticated user's saved searches by id.
// Owner-scoped: an id that does not exist or belongs to another user is a 404. Cookie-only.
func (h *savedSearchHandlers) DeleteSavedSearch(ctx context.Context, params apigen.DeleteSavedSearchParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}
	return savedSearchError(h.savedSearch.Delete(ctx, userID, id))
}
