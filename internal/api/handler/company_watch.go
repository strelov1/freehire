package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
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

func (h *companyWatchHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Cookie-only like saved searches and subscriptions: a browser convenience whose answers
	// are one person's own and never cacheable. The handlers below implement the contract
	// (typespec/watchlist.tsp).
	api.Get("/me/watchlist", mw.cookie, contract)
	api.Post("/me/watchlist/import", mw.cookie, contract)
	api.Get("/me/watchlist/:slug", mw.cookie, contract)
	api.Put("/me/watchlist/:slug", mw.cookie, contract)
	api.Delete("/me/watchlist/:slug", mw.cookie, contract)
}

// watchlistNoStore is the caching rule of both reads: one person's own, never cached.
const watchlistNoStore = "private, no-store"

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

// ListWatchlist returns one page of the caller's watchlist, newest first, optionally
// filtered by a substring of the company name.
func (h *companyWatchHandlers) ListWatchlist(ctx context.Context, params apigen.ListWatchlistParams) (*apigen.WatchlistPageHeaders, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := contractPageBounded(ctx, 50, 100)
	entries, total, err := h.watch.List(ctx, userID, params.Q.Value, limit, offset)
	if err != nil {
		return nil, companyWatchError(err)
	}
	data := make([]apigen.WatchedCompany, len(entries))
	for i, e := range entries {
		data[i] = watchedCompanyOut(e)
	}
	return &apigen.WatchlistPageHeaders{
		CacheControl: watchlistNoStore,
		Response: apigen.WatchlistPage{Data: data, Meta: apigen.WatchlistMeta{
			Total: int64(total), Limit: int64(limit), Offset: int64(offset), Max: companywatch.MaxEntries,
		}},
	}, nil
}

// GetWatching reports whether the caller watches the company a page's slug names.
func (h *companyWatchHandlers) GetWatching(ctx context.Context, params apigen.GetWatchingParams) (*apigen.GetWatchingOKHeaders, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	watched, err := h.watch.Watching(ctx, userID, params.Slug)
	if err != nil {
		return nil, companyWatchError(err)
	}
	return &apigen.GetWatchingOKHeaders{
		CacheControl: watchlistNoStore,
		Response:     apigen.GetWatchingOK{Data: apigen.Watching{Watched: watched}},
	}, nil
}

// AddToWatchlist puts a company on the caller's watchlist.
func (h *companyWatchHandlers) AddToWatchlist(ctx context.Context, params apigen.AddToWatchlistParams) (*apigen.AddToWatchlistOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := h.watch.Add(ctx, userID, params.Slug)
	if err != nil {
		return nil, companyWatchError(err)
	}
	return &apigen.AddToWatchlistOK{Data: watchedCompanyOut(entry)}, nil
}

// RemoveFromWatchlist takes a company off the caller's watchlist; one that was not on it is
// not an error.
func (h *companyWatchHandlers) RemoveFromWatchlist(ctx context.Context, params apigen.RemoveFromWatchlistParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	return companyWatchError(h.watch.Remove(ctx, userID, params.Slug))
}

// ImportWatchlist adds the companies a list of names or slugs identifies and reports what it
// could not resolve.
func (h *companyWatchHandlers) ImportWatchlist(ctx context.Context, req *apigen.WatchlistImport) (*apigen.ImportWatchlistOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	res, err := h.watch.Import(ctx, userID, req.Names)
	if err != nil {
		return nil, companyWatchError(err)
	}
	return &apigen.ImportWatchlistOK{Data: apigen.WatchlistImportResult{
		Added: int64(res.Added), Already: int64(res.Already), Unmatched: res.Unmatched,
	}}, nil
}

func watchedCompanyOut(e companywatch.Entry) apigen.WatchedCompany {
	return apigen.WatchedCompany{Slug: e.Slug, Name: e.Name, AddedAt: e.AddedAt}
}
