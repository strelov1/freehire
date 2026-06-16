// Package search provides Meilisearch-backed full-text and hybrid (keyword +
// semantic) search over jobs. It owns the document shape and two index
// configurations — a facet/keyword index (no embedder, the fast default) and a
// semantic index that adds the in-engine huggingFace embedder — plus the
// read/write helpers, so callers (the search handler and the reindex command)
// never touch the meilisearch-go SDK directly.
package search

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

const (
	// facetIndexUID is the production search index: keyword + facets, NO embedder,
	// so a full rebuild is ~25x faster than embedding every document. It serves all
	// default (keyword) traffic and the facet analytics.
	facetIndexUID = "jobs"
	// semanticIndexUID is the optional hybrid index: the same documents plus the
	// in-engine embedder. It is built by a separate, slower pass (reindex
	// --semantic) and only queried when SemanticRatio > 0, so embedding never
	// blocks facet/keyword freshness.
	semanticIndexUID = "jobs_semantic"
	primaryKey       = "id"
	embedderName     = "default"
	// embedderModel runs inside Meilisearch (source huggingFace), so hybrid
	// search needs no external API key. Multilingual + CPU-friendly.
	embedderModel = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"

	// maxTotalHits caps how high a search counts its results: below it,
	// estimatedTotalHits is the true filtered total, so it is set well above the
	// index size to keep the reported count honest. It is NOT the pagination guard
	// — deep offset paging is bounded separately by maxSearchWindow in the search
	// handler — so a large value here costs nothing beyond an accurate total.
	maxTotalHits = 1000000

	// maxValuesPerFacet raises the per-facet value cap above Meili's default of
	// 100 so the analytics facet distribution is not truncated for
	// high-cardinality facets (skills, countries).
	maxValuesPerFacet = 300

	taskPollInterval = 50 * time.Millisecond
)

// Client is a thin wrapper over the Meilisearch service scoped to the two job
// indexes: facet (keyword + facets, no embedder) and semantic (adds the embedder).
type Client struct {
	manager  meilisearch.ServiceManager
	facet    meilisearch.IndexManager
	semantic meilisearch.IndexManager
}

// NewClient connects to Meilisearch at url authenticated by key. It does no I/O
// — the connection is exercised lazily by the first request (or EnsureIndex).
func NewClient(url, key string) *Client {
	m := meilisearch.New(url, meilisearch.WithAPIKey(key))
	return &Client{manager: m, facet: m.Index(facetIndexUID), semantic: m.Index(semanticIndexUID)}
}

// EnsureIndex creates the facet/keyword jobs index (no embedder) and applies its
// settings. It is idempotent — safe to call on every reindex. This is the fast
// production index that all default (keyword) traffic and faceting hit.
func (c *Client) EnsureIndex(ctx context.Context) error {
	return c.ensure(ctx, c.facet, facetIndexUID, facetSettings())
}

// EnsureSemanticIndex creates the hybrid jobs index (with the in-engine embedder)
// and applies its settings. It is built by the separate reindex --semantic pass;
// querying it embeds documents, so it is kept off the default reindex path.
func (c *Client) EnsureSemanticIndex(ctx context.Context) error {
	return c.ensure(ctx, c.semantic, semanticIndexUID, semanticSettings())
}

// ensure creates the named index (keyed by the internal id) if absent and applies
// its settings. Shared by the facet and semantic ensure paths.
func (c *Client) ensure(ctx context.Context, idx meilisearch.IndexManager, uid string, settings *meilisearch.Settings) error {
	create, err := c.manager.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{
		Uid:        uid,
		PrimaryKey: primaryKey,
	})
	if err != nil {
		return fmt.Errorf("search: create index: %w", err)
	}
	created, err := idx.WaitForTaskWithContext(ctx, create.TaskUID, taskPollInterval)
	if err != nil {
		return fmt.Errorf("search: await create index: %w", err)
	}
	// An already-existing index is the idempotent happy path, not a failure.
	if created.Status == meilisearch.TaskStatusFailed && created.Error.Code != "index_already_exists" {
		return fmt.Errorf("search: create index failed: %s", created.Error.Message)
	}

	st, err := idx.UpdateSettingsWithContext(ctx, settings)
	if err != nil {
		return fmt.Errorf("search: update settings: %w", err)
	}
	return c.awaitTask(ctx, idx, st.TaskUID)
}

// IndexJobs upserts a batch of documents into the facet index by primary key. A
// re-run with the same data is a no-op upsert, keeping reindex idempotent.
func (c *Client) IndexJobs(ctx context.Context, docs []JobDocument) error {
	return c.indexInto(ctx, c.facet, docs)
}

// IndexSemanticJobs upserts a batch into the semantic index (which embeds new or
// changed documents). Used by the reindex --semantic pass.
func (c *Client) IndexSemanticJobs(ctx context.Context, docs []JobDocument) error {
	return c.indexInto(ctx, c.semantic, docs)
}

func (c *Client) indexInto(ctx context.Context, idx meilisearch.IndexManager, docs []JobDocument) error {
	if len(docs) == 0 {
		return nil
	}
	pk := primaryKey
	task, err := idx.UpdateDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return fmt.Errorf("search: index documents: %w", err)
	}
	return c.awaitTask(ctx, idx, task.TaskUID)
}

// DeleteJobs removes documents from the facet index by primary key. Used by
// reindex to drop closed jobs; deleting an id that is not indexed is a no-op,
// keeping re-runs idempotent.
func (c *Client) DeleteJobs(ctx context.Context, ids []int64) error {
	return c.deleteFrom(ctx, c.facet, ids)
}

// DeleteSemanticJobs removes documents from the semantic index. Used by the
// reindex --semantic pass.
func (c *Client) DeleteSemanticJobs(ctx context.Context, ids []int64) error {
	return c.deleteFrom(ctx, c.semantic, ids)
}

func (c *Client) deleteFrom(ctx context.Context, idx meilisearch.IndexManager, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = strconv.FormatInt(id, 10)
	}
	task, err := idx.DeleteDocumentsWithContext(ctx, keys, nil)
	if err != nil {
		return fmt.Errorf("search: delete documents: %w", err)
	}
	return c.awaitTask(ctx, idx, task.TaskUID)
}

// SearchParams is a backend-agnostic search request. Filter is the value built
// by Filter (nil for none). SemanticRatio blends keyword (0) and semantic (1);
// the hybrid embedder is only engaged when the ratio is above zero, so plain
// keyword search never depends on the embedder.
type SearchParams struct {
	Query         string
	Filter        any
	Sort          []string
	Limit         int
	Offset        int
	SemanticRatio float64
}

// SearchResult holds the matched documents and Meilisearch's estimated total.
type SearchResult struct {
	Hits  []JobDocument
	Total int64
}

// Search runs a query against the jobs index and decodes the hits.
func (c *Client) Search(ctx context.Context, p SearchParams) (SearchResult, error) {
	req := &meilisearch.SearchRequest{
		Filter: p.Filter,
		Sort:   p.Sort,
		Limit:  int64(p.Limit),
		Offset: int64(p.Offset),
	}
	// Default (keyword) traffic hits the facet index — always fresh, no embedder.
	// A semantic request routes to the semantic index and engages the embedder.
	idx := c.facet
	if p.SemanticRatio > 0 {
		idx = c.semantic
		req.Hybrid = &meilisearch.SearchRequestHybrid{
			Embedder:      embedderName,
			SemanticRatio: p.SemanticRatio,
		}
	}

	resp, err := idx.SearchWithContext(ctx, p.Query, req)
	if err != nil {
		return SearchResult{}, fmt.Errorf("search: query: %w", err)
	}

	var hits []JobDocument
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return SearchResult{}, fmt.Errorf("search: decode hits: %w", err)
	}
	return SearchResult{Hits: hits, Total: resp.EstimatedTotalHits}, nil
}

// awaitTask blocks until a Meilisearch task settles and reports a failed task as
// an error.
func (c *Client) awaitTask(ctx context.Context, idx meilisearch.IndexManager, taskUID int64) error {
	t, err := idx.WaitForTaskWithContext(ctx, taskUID, taskPollInterval)
	if err != nil {
		return fmt.Errorf("search: await task %d: %w", taskUID, err)
	}
	if t.Status == meilisearch.TaskStatusFailed {
		return fmt.Errorf("search: task %d failed: %s", taskUID, t.Error.Message)
	}
	return nil
}

// facetSettings is the single source of truth for the facet/keyword index
// configuration — everything EXCEPT the embedder. Indexing into it costs no
// per-document embedding, so a full rebuild runs ~25x faster than the semantic
// index. semanticSettings layers the embedder on top of this.
func facetSettings() *meilisearch.Settings {
	return &meilisearch.Settings{
		SearchableAttributes: []string{"title", "company", "description", "location"},
		// Enrichment facets are nested, so they are filtered via dot paths. The
		// resolved geography facet (regions/countries), work_mode, and skills are
		// served top-level — the union of parsed-location/column and enrichment
		// values — so they are filtered on a bare attribute, not the enrichment
		// dot path.
		FilterableAttributes: []string{
			"source", "company_slug",
			"work_mode", "regions", "countries", "skills",
			"enrichment.employment_type", "enrichment.seniority",
			"enrichment.category", "enrichment.domains",
			"enrichment.company_type", "enrichment.company_size", "enrichment.visa_sponsorship",
			"enrichment.salary_currency", "enrichment.salary_period",
			"enrichment.salary_min", "enrichment.salary_max", "enrichment.experience_years_min",
			"enrichment.relocation", "enrichment.english_level", "enrichment.posting_language",
		},
		// posted_at / created_at are RFC3339 UTC strings and sort chronologically as text.
		SortableAttributes: []string{"posted_at", "created_at", "enrichment.salary_min", "enrichment.salary_max"},
		RankingRules:       []string{"words", "sort", "typo", "proximity", "attribute", "exactness"},
		// Typo tolerance is left at Meilisearch's defaults (on, with sensible min
		// word sizes). We deliberately do not send a TypoTolerance struct: the SDK
		// always serializes newer fields (e.g. disableOnNumbers) that older
		// Meilisearch versions reject, and the spec only requires typo tolerance to
		// exist, not specific thresholds. Re-add explicit tuning when the pinned
		// server and SDK fields align.
		Pagination: &meilisearch.Pagination{MaxTotalHits: maxTotalHits},
		// Raise the per-facet value cap above Meili's default of 100 so the
		// analytics facet distribution is not truncated for high-cardinality
		// facets. Value ordering is done client-side by count; SortFacetValuesBy is
		// left unset (see the TypoTolerance note above on the SDK over-serializing
		// newer fields). Requires a reindex to take effect on an existing index.
		Faceting: &meilisearch.Faceting{MaxValuesPerFacet: maxValuesPerFacet},
	}
}

// semanticSettings is the hybrid index configuration: the facet/keyword settings
// plus the in-engine huggingFace embedder. Meilisearch embeds each new or changed
// document at index time (and skips unchanged ones), so this index is built by the
// separate, slower reindex --semantic pass and never on the default reindex path.
func semanticSettings() *meilisearch.Settings {
	s := facetSettings()
	s.Embedders = map[string]meilisearch.Embedder{
		embedderName: {
			Source:           "huggingFace",
			Model:            embedderModel,
			DocumentTemplate: "{{ doc.title }} at {{ doc.company }}. {{ doc.description }}",
		},
	}
	return s
}
