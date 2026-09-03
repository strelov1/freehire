package handler

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/suggest"
)

// suggester is the completion service the handler depends on. *suggest.Service
// satisfies it; tests inject a fake. Nil means the dictionary is not configured (no
// MEILI_MASTER_KEY), and the endpoint reports 503 like the other search reads.
type suggester interface {
	Suggest(ctx context.Context, query string, limit int) ([]suggest.Suggestion, error)
}

// The completion list is a shortcut, not a results page: past a handful the rows stop
// being scannable and stop fitting on a phone. The cap is enforced server-side too, so
// a caller asking for 500 gets a dropdown's worth rather than a dictionary dump.
const (
	suggestDefaultLimit = 5
	suggestMaxLimit     = 20
)

type suggestHandlers struct{ suggest suggester }

func (h *suggestHandlers) register(api fiber.Router, mw middleware) {
	// Its own budget: this is called once per settled keystroke, so sharing the
	// public-read allowance would mean typing a query throttles the page that answers
	// it. See suggestLimiter.
	api.Get("/suggest", suggestLimiter(mw.throttler), h.Suggest)
}

// Suggest completes what the visitor is typing.
//
// An empty `q` is answered with an empty list rather than a curated one. What an empty
// box should offer is the filter modal's own category grouping, which lives in the web
// and is checked there against the category vocabulary at compile time; serving it from
// here would be a second copy of that order — see suggest.Service.Suggest.
func (h *suggestHandlers) Suggest(c *fiber.Ctx) error {
	if h.suggest == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not configured")
	}

	items, err := h.suggest.Suggest(c.Context(), c.Query("q"), suggestLimit(c))
	if err != nil {
		return err
	}
	// Always an array, never null: a client that renders `data.map(...)` should not
	// have to distinguish "no matches" from "no field".
	if items == nil {
		items = []suggest.Suggestion{}
	}
	return c.JSON(fiber.Map{
		"data": items,
		"meta": fiber.Map{"count": len(items)},
	})
}

// suggestLimit reads the row cap, clamped. An unparseable or out-of-range value falls
// back to the default rather than failing the request: this endpoint is typed into, and
// a 400 in the middle of a dropdown is worse than a sensible number of rows.
func suggestLimit(c *fiber.Ctx) int {
	n, err := strconv.Atoi(c.Query("limit"))
	if err != nil || n < 1 {
		return suggestDefaultLimit
	}
	return min(n, suggestMaxLimit)
}
