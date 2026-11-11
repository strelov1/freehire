package handler

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
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

func (h *suggestHandlers) register(api fiber.Router, contract fiber.Handler) {
	api.Get("/suggest", contract)
}

// Suggest completes what the visitor is typing.
//
// An empty `q` is answered with an empty list rather than a curated one. What an empty
// box should offer is the filter modal's own category grouping, which lives in the web
// and is checked there against the category vocabulary at compile time; serving it from
// here would be a second copy of that order — see suggest.Service.Suggest.
func (h *suggestHandlers) Suggest(ctx context.Context, params apigen.SuggestParams) (*apigen.SuggestOK, error) {
	if h.suggest == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not configured")
	}

	items, err := h.suggest.Suggest(ctx, params.Q.Value, suggestLimit(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	// Always an array, never null (the generated encoder writes a nil slice as []): a
	// client that renders `data.map(...)` should not have to tell "no matches" from "no field".
	out := make([]apigen.Suggestion, len(items))
	for i, item := range items {
		parts := make([]apigen.SuggestionPart, len(item.Parts))
		for j, p := range item.Parts {
			parts[j] = apigen.SuggestionPart{Kind: apigen.SuggestionKind(p.Kind), Slug: optString(p.Slug), Text: p.Text}
		}
		out[i] = apigen.Suggestion{Text: item.Text, Parts: parts, Jobs: int64(item.Jobs)}
	}
	return &apigen.SuggestOK{Data: out, Meta: apigen.SuggestMeta{Count: int64(len(out))}}, nil
}

// suggestLimit reads the row cap, clamped. An unparseable or out-of-range value falls
// back to the default rather than failing the request: this endpoint is typed into, and
// a 400 in the middle of a dropdown is worse than a sensible number of rows.
func suggestLimit(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return suggestDefaultLimit
	}
	return min(n, suggestMaxLimit)
}
