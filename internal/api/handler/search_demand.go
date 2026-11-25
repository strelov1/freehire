package handler

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/search/suggest"
)

// searchRecordTimeout bounds the fire-and-forget demand write. Short: it is one
// upsert on a small table, and a write still running past this is a database in
// trouble rather than a slow row.
const searchRecordTimeout = 5 * time.Second

// recordQuery notes what the catalogue was asked for, so the suggestion dictionary can
// rank by demand rather than only by supply.
//
// Three properties, all deliberate:
//
//   - It CANNOT fail the search. A write error is logged and discarded: the result is
//     what the visitor asked for and this is a by-product.
//   - It CANNOT delay the response. It runs on its own goroutine with its own bounded
//     context, because c.Context() is released when the handler returns.
//   - It identifies nobody. The row is the phrase and a counter; no user, no session,
//     no address — and `Recordable` refuses text that is not a search phrase at all,
//     so a pasted email address or job description never reaches the table.
//
// The key is the same normalisation the builder applies to mined titles, so a typed
// query and the title it names land on one row.
func (h *searchHandlers) recordQuery(raw string) {
	q := demandKey(raw)
	if !suggest.Recordable(q) || h.queries == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), searchRecordTimeout)
		defer cancel()
		if err := h.queries.RecordSearchQuery(ctx, q); err != nil {
			log.Printf("search: record query: %v", err)
		}
	}()
}

// demandKey normalises a raw `q` into the same key suggest.Title gives a mined posting title. A query wrapped in a matching pair of `"` (a title suggestion click) is unwrapped first, since that is Meilisearch quoting, not part of the phrase; any other quoting is left alone.
func demandKey(raw string) string {
	if len(raw) >= 2 && strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
		raw = raw[1 : len(raw)-1]
	}
	return suggest.Title(raw)
}
