// Package search provides Meilisearch-backed full-text and hybrid (keyword +
// semantic) search over jobs. It owns the index document shape, the index
// settings (including the in-engine huggingFace embedder), and the read/write
// helpers, so callers (the search handler and the reindex command) never touch
// the meilisearch-go SDK directly.
package search

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

const (
	indexUID     = "jobs"
	primaryKey   = "id"
	embedderName = "default"
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

// Client is a thin wrapper over the Meilisearch service scoped to the jobs index.
type Client struct {
	manager meilisearch.ServiceManager
	index   meilisearch.IndexManager
}

// NewClient connects to Meilisearch at url authenticated by key. It does no I/O
// — the connection is exercised lazily by the first request (or EnsureIndex).
func NewClient(url, key string) *Client {
	m := meilisearch.New(url, meilisearch.WithAPIKey(key))
	return &Client{manager: m, index: m.Index(indexUID)}
}

// EnsureIndex creates the jobs index (keyed by the internal id) if absent and
// applies its settings: the searchable/filterable/sortable attributes, ranking
// rules, typo tolerance, pagination cap, and the hybrid embedder. It is
// idempotent — safe to call on every reindex.
func (c *Client) EnsureIndex(ctx context.Context) error {
	create, err := c.manager.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{
		Uid:        indexUID,
		PrimaryKey: primaryKey,
	})
	if err != nil {
		return fmt.Errorf("search: create index: %w", err)
	}
	created, err := c.index.WaitForTaskWithContext(ctx, create.TaskUID, taskPollInterval)
	if err != nil {
		return fmt.Errorf("search: await create index: %w", err)
	}
	// An already-existing index is the idempotent happy path, not a failure.
	if created.Status == meilisearch.TaskStatusFailed && created.Error.Code != "index_already_exists" {
		return fmt.Errorf("search: create index failed: %s", created.Error.Message)
	}

	settings, err := c.index.UpdateSettingsWithContext(ctx, indexSettings())
	if err != nil {
		return fmt.Errorf("search: update settings: %w", err)
	}
	return c.awaitTask(ctx, settings.TaskUID)
}

// IndexJobs upserts a batch of documents by primary key. A re-run with the same
// data is a no-op upsert, keeping reindex idempotent.
func (c *Client) IndexJobs(ctx context.Context, docs []JobDocument) error {
	if len(docs) == 0 {
		return nil
	}
	pk := primaryKey
	task, err := c.index.UpdateDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return fmt.Errorf("search: index documents: %w", err)
	}
	return c.awaitTask(ctx, task.TaskUID)
}

// DeleteJobs removes documents by primary key. Used by reindex to drop closed
// jobs from the index; deleting an id that is not indexed is a no-op, keeping
// re-runs idempotent.
func (c *Client) DeleteJobs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = strconv.FormatInt(id, 10)
	}
	task, err := c.index.DeleteDocumentsWithContext(ctx, keys, nil)
	if err != nil {
		return fmt.Errorf("search: delete documents: %w", err)
	}
	return c.awaitTask(ctx, task.TaskUID)
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
	if p.SemanticRatio > 0 {
		req.Hybrid = &meilisearch.SearchRequestHybrid{
			Embedder:      embedderName,
			SemanticRatio: p.SemanticRatio,
		}
	}

	resp, err := c.index.SearchWithContext(ctx, p.Query, req)
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
func (c *Client) awaitTask(ctx context.Context, taskUID int64) error {
	t, err := c.index.WaitForTaskWithContext(ctx, taskUID, taskPollInterval)
	if err != nil {
		return fmt.Errorf("search: await task %d: %w", taskUID, err)
	}
	if t.Status == meilisearch.TaskStatusFailed {
		return fmt.Errorf("search: task %d failed: %s", taskUID, t.Error.Message)
	}
	return nil
}

// indexSettings is the single source of truth for the jobs index configuration.
func indexSettings() *meilisearch.Settings {
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
		Embedders: map[string]meilisearch.Embedder{
			embedderName: {
				Source:           "huggingFace",
				Model:            embedderModel,
				DocumentTemplate: "{{ doc.title }} at {{ doc.company }}. {{ doc.description }}",
			},
		},
	}
}
