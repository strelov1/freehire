package search

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/meilisearch/meilisearch-go"
)

// The suggestion dictionary index: the search box's completions, built offline by
// cmd/build-suggestions from mined posting titles plus the facet vocabularies.
//
// A SEPARATE index, not a facet on `jobs`. `title` is searchable there but not
// filterable, and promoting it would not work: distinct titles number in the
// millions, MaxValuesPerFacet truncates a facet's distribution, and the engine would
// carry a high-cardinality filterable attribute it could never serve completely. The
// dictionary is bounded offline instead — measured over 2,500 live titles, 562 are
// distinct and 72 occur three times or more.
//
// Its smallness is also the performance argument. The box queries it on every settled
// keystroke, and doing that against the 8M-document `jobs` index would put a
// per-keystroke query on the index that serves the whole site.
const (
	suggestIndexUID   = "suggestions"
	suggestRebuildUID = "suggestions_rebuild"
)

// suggestSettings configures the dictionary index.
//
// Typo tolerance is left at Meilisearch's defaults, deliberately — it is the entire
// reason a typo reaches its target here. The previous, client-side matcher had to
// DROP typo-only matches because it ranked them by vacancy count, which put Marketing
// Specialist (55,768, reached by edit distance against its `growth hacker` alias)
// above Backend Engineer for `backedn`. Ranking by relevance to the query removes the
// reason that rule existed.
func suggestSettings() *meilisearch.Settings {
	return &meilisearch.Settings{
		// One searchable attribute: the phrase itself. Nothing else in a suggestion is
		// text a visitor would be typing.
		SearchableAttributes: []string{"text"},
		// `kind` so the endpoint can exclude a kind the recognised prefix already
		// filled; `slug` so a specific suggestion can be looked up by identity.
		FilterableAttributes: []string{"kind", "slug"},
		// Demand first, then supply — the endpoint's tie-breakers, applied by the
		// engine so they cost nothing at query time.
		SortableAttributes: []string{"searches", "jobs"},
		// The default rules minus `sort` reordering, plus the two custom rules that
		// carry the ranking this index exists to apply. `searches` leads because what
		// people actually ask for beats what merely exists a lot of.
		RankingRules: []string{
			"words", "typo", "proximity", "attribute", "exactness",
			"searches:desc", "jobs:desc",
		},
	}
}

// EnsureSuggestIndex creates the dictionary index and applies its settings. Idempotent.
func (c *Client) EnsureSuggestIndex(ctx context.Context) error {
	idx := c.manager.Index(suggestIndexUID)
	return c.ensure(ctx, idx, suggestIndexUID, "id", suggestSettings())
}

// NewSuggestRebuild starts a full rebuild of the dictionary index. The dictionary is
// derived wholesale from a catalogue pass, so there is no incremental path and no
// outbox: every run replaces it, and the swap is what keeps a reader from ever seeing
// a half-built dictionary.
func (c *Client) NewSuggestRebuild() *Rebuild {
	return &Rebuild{
		c: c, liveUID: suggestIndexUID, rebuildUID: suggestRebuildUID, settings: suggestSettings(),
	}
}

// PushAny is Push for an index whose documents are not JobDocuments. Same contract:
// the batch is enqueued without waiting, and Promote awaits every task, so
// Meilisearch auto-batches consecutive document tasks instead of indexing each in
// isolation.
//
// It takes `any` rather than a second typed method because the document type belongs
// to the package that MINES it — this package would otherwise need to import that one,
// and that one already needs nothing from here.
func (r *Rebuild) PushAny(ctx context.Context, docs any) error {
	pk := r.c.primaryKeyFor(r.rebuildUID)
	task, err := r.rebuild.UpdateDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return fmt.Errorf("search: rebuild push: %w", err)
	}
	r.tasks = append(r.tasks, task.TaskUID)
	return nil
}

// primaryKeyFor reports the primary key an index is built on. The jobs index keys on
// `id` as a job id; the dictionary keys on `id` as a kind-namespaced suggestion id —
// the same field name for two different identities, which is why this is a lookup
// rather than the bare `primaryKey` constant.
func (c *Client) primaryKeyFor(uid string) string {
	if uid == suggestRebuildUID || uid == suggestIndexUID {
		return "id"
	}
	return primaryKey
}

// maxDictionary bounds the whole-dictionary read behind the recognition set. The
// dictionary is tens of thousands of rows by construction — a floor keeps it there —
// so this is a guard against a misconfigured floor rather than a paging limit: reading
// a dictionary that large into a long-lived process is a memory problem, and silently
// truncating it is better than the alternative of holding all of it.
const maxDictionary = 200_000

// AllSuggestions reads the dictionary whole, for the in-process recognition set.
//
// An empty query with the engine's default ranking, which for this index is
// `searches:desc, jobs:desc` — so a truncation keeps the phrases people ask for and
// drops the tail nobody has typed.
func AllSuggestions[T any](ctx context.Context, c *Client) ([]T, error) {
	return SearchSuggestions[T](ctx, c, "", "", maxDictionary)
}

// isIndexMissing reports whether the dictionary index does not exist.
//
// By status rather than by the engine's error code: the SDK keeps that code in an
// unexported type, so a check written against it could never be tested. A 404 from a
// search against this one index has a single meaning anyway.
func isIndexMissing(err error) bool {
	var me *meilisearch.Error
	return errors.As(err, &me) && me.StatusCode == http.StatusNotFound
}

// SearchSuggestions runs a query against the dictionary and decodes the hits into the
// caller's document type.
//
// Generic, and a function rather than a method, so the document type can live in the
// package that mines it. `filter` narrows by kind (the endpoint excludes a kind the
// recognised prefix already filled); an empty filter matches everything.
func SearchSuggestions[T any](ctx context.Context, c *Client, query, filter string, limit int) ([]T, error) {
	req := &meilisearch.SearchRequest{Limit: int64(limit)}
	if filter != "" {
		req.Filter = filter
	}
	resp, err := c.manager.Index(suggestIndexUID).SearchWithContext(ctx, query, req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("search: suggest: %w", ctxErr)
		}
		// A dictionary that has not been built yet is "no suggestions", not a fault.
		// Between a deploy and the first cmd/build-suggestions run the index does not
		// exist, and the box asks for a completion on every settled keystroke — so
		// surfacing this as an error made that whole window a broken-looking dropdown
		// and a stream of 500s. Found in production, not in a test.
		if isIndexMissing(err) {
			return nil, nil
		}
		if isBadRequest(err) {
			return nil, fmt.Errorf("search: suggest: %w: %v", ErrBadQuery, err)
		}
		return nil, fmt.Errorf("search: suggest: %w", err)
	}
	var hits []T
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return nil, fmt.Errorf("search: suggest: decode hits: %w", err)
	}
	return hits, nil
}
