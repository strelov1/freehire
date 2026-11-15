package suggest

import (
	"context"

	"github.com/strelov1/freehire/internal/search/search"
)

// meiliIndex adapts the search client to Index. It lives here because Document is defined
// here; the engine package stays generic over it.
type meiliIndex struct{ c *search.Client }

// NewIndex wraps a search client as the service's index.
func NewIndex(c *search.Client) Index { return meiliIndex{c: c} }

func (m meiliIndex) SearchSuggestions(ctx context.Context, query, filter string, limit int) ([]Document, error) {
	return search.SearchSuggestions[Document](ctx, m.c, query, filter, limit)
}

func (m meiliIndex) AllSuggestions(ctx context.Context) ([]Document, error) {
	return search.AllSuggestions[Document](ctx, m.c)
}
