package suggest

import (
	"context"

	"github.com/strelov1/freehire/internal/search/search"
)

// meiliIndex adapts the search client to the narrow Index the service needs.
//
// It lives on this side of the boundary because the DOCUMENT type is mined here: the
// engine package stays generic over it (SearchSuggestions is generic for exactly that
// reason) and never learns what a suggestion is.
type meiliIndex struct{ c *search.Client }

// NewIndex wraps a search client as the service's index.
func NewIndex(c *search.Client) Index { return meiliIndex{c: c} }

func (m meiliIndex) SearchSuggestions(ctx context.Context, query, filter string, limit int) ([]Document, error) {
	return search.SearchSuggestions[Document](ctx, m.c, query, filter, limit)
}

func (m meiliIndex) AllSuggestions(ctx context.Context) ([]Document, error) {
	return search.AllSuggestions[Document](ctx, m.c)
}
