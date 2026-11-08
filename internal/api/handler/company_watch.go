package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/companywatch"
	"github.com/strelov1/freehire/internal/platform/db"
)

// companyWatchHandlers serves a signed-in person's company watchlist: the list itself, one
// company's membership, add and remove, and a bulk import by name. The alert that reports
// new postings from it is a saved search of `watchlist=true` (see companywatch), so there is
// nothing to subscribe to here.
type companyWatchHandlers struct {
	watch *companywatch.Service
}

func newCompanyWatchHandlers(queries *db.Queries) *companyWatchHandlers {
	return &companyWatchHandlers{watch: companywatch.New(queries)}
}

func (h *companyWatchHandlers) register(api fiber.Router, mw middleware) {
	// Cookie-only like saved searches and subscriptions: a browser convenience whose answers
	// are one person's own and never cacheable.
	api.Get("/me/watchlist", mw.cookie, h.List)
	api.Post("/me/watchlist/import", mw.cookie, h.Import)
	api.Get("/me/watchlist/:slug", mw.cookie, h.Watching)
	api.Put("/me/watchlist/:slug", mw.cookie, h.Add)
	api.Delete("/me/watchlist/:slug", mw.cookie, h.Remove)
}

// companyWatchError maps the watchlist sentinels onto HTTP statuses. Anything else falls
// through to the central RenderError as a 500.
func companyWatchError(err error) error {
	switch {
	case errors.Is(err, companywatch.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "company not found")
	case errors.Is(err, companywatch.ErrFull):
		return fiber.NewError(fiber.StatusBadRequest, "your watchlist is full: remove a company before adding another")
	case errors.Is(err, companywatch.ErrNothingToImport):
		return fiber.NewError(fiber.StatusBadRequest, "names must list at least one company")
	case errors.Is(err, companywatch.ErrTooManyToImport):
		return fiber.NewError(fiber.StatusBadRequest, "names must list at most 5000 companies")
	default:
		return err
	}
}

// List returns one page of the caller's watchlist, newest first, optionally filtered by a
// substring of the company name.
func (h *companyWatchHandlers) List(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	limit, offset := pageParamsBounded(c, 50, 100)
	entries, total, err := h.watch.List(c.Context(), userID, c.Query("q"), limit, offset)
	if err != nil {
		return companyWatchError(err)
	}
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	return c.JSON(fiber.Map{
		"data": entries,
		"meta": fiber.Map{"total": total, "limit": limit, "offset": offset, "max": companywatch.MaxEntries},
	})
}

// Watching reports whether the caller watches the company a page's slug names.
func (h *companyWatchHandlers) Watching(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	watched, err := h.watch.Watching(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return companyWatchError(err)
	}
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	return c.JSON(fiber.Map{"data": fiber.Map{"watched": watched}})
}

// Add puts a company on the caller's watchlist.
func (h *companyWatchHandlers) Add(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	entry, err := h.watch.Add(c.Context(), userID, c.Params("slug"))
	if err != nil {
		return companyWatchError(err)
	}
	return c.JSON(fiber.Map{"data": entry})
}

// Remove takes a company off the caller's watchlist; one that was not on it is not an error.
func (h *companyWatchHandlers) Remove(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.watch.Remove(c.Context(), userID, c.Params("slug")); err != nil {
		return companyWatchError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// importWatchlistRequest is the bulk-import body: company names or slugs, one per entry.
type importWatchlistRequest struct {
	Names []string `json:"names"`
}

// Import adds the companies a list of names or slugs identifies and reports what it could
// not resolve.
func (h *companyWatchHandlers) Import(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in importWatchlistRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	res, err := h.watch.Import(c.Context(), userID, in.Names)
	if err != nil {
		return companyWatchError(err)
	}
	return c.JSON(fiber.Map{"data": res})
}
