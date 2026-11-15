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
// A SEPARATE index, not a facet on `jobs`: distinct titles number in the millions, a facet
// distribution truncates at MaxValuesPerFacet, and a high-cardinality filterable attribute
// could never be served completely. It is also small enough to query on every settled
// keystroke without loading the index that serves the whole site.
const (
	suggestIndexUID   = "suggestions"
	suggestRebuildUID = "suggestions_rebuild"

	// maxDictionary bounds BOTH how many documents a whole-dictionary read asks for and how
	// many the index returns; the two must agree. The dictionary is tens of thousands of
	// rows by construction, so this guards against a misconfigured floor.
	maxDictionary = 200_000
)

// suggestSettings configures the dictionary index.
//
// Typo tolerance is left at the defaults on purpose: it is how a typo reaches its target.
// Ranking by relevance to the query, not vacancy count, keeps typo-only matches from
// outranking exact ones.
func suggestSettings() *meilisearch.Settings {
	return &meilisearch.Settings{
		// Only the phrase itself is text a visitor types.
		SearchableAttributes: []string{"text"},
		// `kind` lets the endpoint exclude a kind the recognised prefix already filled;
		// `slug` looks a suggestion up by identity.
		FilterableAttributes: []string{"kind", "slug"},
		// Demand, then supply: the endpoint's tie-breakers.
		SortableAttributes: []string{"searches", "jobs"},
		// Meilisearch's default maxTotalHits of 1000 silently truncates the recognition set
		// the API loads at startup, dropping the tail of the `searches:desc` order. The
		// ceiling sits above the dictionary's own bound so it constrains nothing.
		Pagination: &meilisearch.Pagination{MaxTotalHits: maxDictionary},
		// Default rules minus `sort`, plus the custom rules; `searches` leads because what
		// people ask for beats what merely exists a lot.
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

// NewSuggestRebuild starts a full rebuild of the dictionary index. It is derived wholesale
// from a catalogue pass, so there is no incremental path or outbox; the swap keeps readers
// off a half-built dictionary.
func (c *Client) NewSuggestRebuild() *Rebuild {
	return &Rebuild{
		c: c, liveUID: suggestIndexUID, rebuildUID: suggestRebuildUID, settings: suggestSettings(),
	}
}

// PushAny is Push for documents that are not JobDocuments: enqueued without waiting,
// awaited by Promote. It takes `any` because the document type belongs to the package that
// mines it, which this package must not import.
func (r *Rebuild) PushAny(ctx context.Context, docs any) error {
	pk := r.c.primaryKeyFor(r.rebuildUID)
	task, err := r.rebuild.UpdateDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return fmt.Errorf("search: rebuild push: %w", err)
	}
	r.tasks = append(r.tasks, task.TaskUID)
	return nil
}

// primaryKeyFor reports an index's primary key. Both indexes key on `id`, but as a job id
// in one and a kind-namespaced suggestion id in the other, hence a lookup.
func (c *Client) primaryKeyFor(uid string) string {
	if uid == suggestRebuildUID || uid == suggestIndexUID {
		return "id"
	}
	return primaryKey
}

// AllSuggestions reads the dictionary whole for the in-process recognition set. An empty
// query uses the default `searches:desc, jobs:desc` ranking, so a truncation drops the
// tail nobody types.
func AllSuggestions[T any](ctx context.Context, c *Client) ([]T, error) {
	return SearchSuggestions[T](ctx, c, "", "", maxDictionary)
}

// isIndexMissing reports whether the dictionary index does not exist. It checks the status
// because the SDK keeps the engine's error code in an unexported type.
func isIndexMissing(err error) bool {
	var me *meilisearch.Error
	return errors.As(err, &me) && me.StatusCode == http.StatusNotFound
}

// SearchSuggestions queries the dictionary and decodes the hits into T. It is a generic
// function so the document type can live in the mining package. `filter` narrows by kind;
// empty matches everything.
func SearchSuggestions[T any](ctx context.Context, c *Client, query, filter string, limit int) ([]T, error) {
	req := &meilisearch.SearchRequest{Limit: int64(limit)}
	if filter != "" {
		req.Filter = filter
	}
	resp, err := c.manager.Index(suggestIndexUID).SearchWithContext(ctx, query, req)
	if err != nil {
		// An unbuilt dictionary (before the first cmd/build-suggestions run) means no
		// suggestions, not a fault. Checked under a live context only, so a cancelled
		// caller still gets its sentinel.
		if ctx.Err() == nil && isIndexMissing(err) {
			return nil, nil
		}
		return nil, queryErr(ctx, "search: suggest", err)
	}
	var hits []T
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return nil, fmt.Errorf("search: suggest: decode hits: %w", err)
	}
	return hits, nil
}
