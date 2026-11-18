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

// The completion list is a shortcut, not a results page; the cap is enforced server-side too.
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
// An empty `q` gets an empty list: what an empty box offers is the web's own category
// grouping, and serving it here would duplicate that order (see suggest.Service.Suggest).
func (h *suggestHandlers) Suggest(ctx context.Context, params apigen.SuggestParams) (*apigen.SuggestOK, error) {
	if h.suggest == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "search is not configured")
	}

	items, err := h.suggest.Suggest(ctx, params.Q.Value, suggestLimit(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	// Always an array, never null, so clients need not tell "no matches" from "no field".
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

// suggestLimit reads the row cap, clamped. An unparseable or out-of-range value falls back
// to the default: a 400 in the middle of a dropdown is worse than a sensible number of rows.
func suggestLimit(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return suggestDefaultLimit
	}
	return min(n, suggestMaxLimit)
}
