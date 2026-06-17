// Package search provides Meilisearch-backed full-text and hybrid (keyword +
// semantic) search over jobs. It owns the document shape and two index
// configurations — a facet/keyword index (no embedder, the fast default) and a
// semantic index that adds the in-engine huggingFace embedder — plus the
// read/write helpers, so callers (the search handler and the reindex command)
// never touch the meilisearch-go SDK directly.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	// facetRebuildUID / semanticRebuildUID are the throwaway indexes a full rebuild
	// streams into before atomically swapping over the live index (see Rebuild).
	facetRebuildUID    = "jobs_rebuild"
	semanticRebuildUID = "jobs_semantic_rebuild"
	primaryKey         = "id"
	embedderName       = "default"
	// embedderModel runs inside Meilisearch (source huggingFace), so hybrid
	// search needs no external API key. Multilingual + CPU-friendly.
	embedderModel = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"

	// maxTotalHits caps how high a search counts its results: below it,
	// estimatedTotalHits is the true filtered total, so it is set well above the
	// index size to keep the reported count honest. It is NOT the pagination guard
	// — deep offset paging is bounded separately by maxSearchWindow in the search
	// handler — so a large value here costs nothing beyond an accurate total.
	// Keep it comfortably above the open-job catalogue (which crossed 1M in
	// 2026-06): once the real total exceeds this cap, every count saturates at it.
	maxTotalHits = 10000000

	// maxValuesPerFacet raises the per-facet value cap above Meili's default of
	// 100 so the analytics facet distribution is not truncated for
	// high-cardinality facets (skills, countries).
	maxValuesPerFacet = 300

	taskPollInterval = 50 * time.Millisecond
)

// Client is a thin wrapper over the Meilisearch service scoped to the two job
// indexes: facet (keyword + facets, no embedder) and semantic (adds the embedder).
// url/key are kept for the one raw call (swap-indexes) the SDK cannot make against
// our engine version — see swapIndexes.
type Client struct {
	manager  meilisearch.ServiceManager
	facet    meilisearch.IndexManager
	semantic meilisearch.IndexManager
	url      string
	key      string
}

// NewClient connects to Meilisearch at url authenticated by key. It does no I/O
// — the connection is exercised lazily by the first request (or EnsureIndex).
func NewClient(url, key string) *Client {
	m := meilisearch.New(url, meilisearch.WithAPIKey(key))
	return &Client{manager: m, facet: m.Index(facetIndexUID), semantic: m.Index(semanticIndexUID), url: url, key: key}
}

// EnsureIndex creates the facet/keyword jobs index (no embedder) and applies its
// settings. It is idempotent — safe to call on every reindex. This is the fast
// production index that all default (keyword) traffic and faceting hit.
func (c *Client) EnsureIndex(ctx context.Context) error {
	if err := c.ensure(ctx, c.facet, facetIndexUID, facetSettings()); err != nil {
		return err
	}
	// Meilisearch settings updates MERGE: facetSettings omits the embedder, but an
	// omitted (nil or empty) embedders map LEAVES any embedder a prior version put
	// on this index in place — so a `jobs` index that once carried the embedder
	// would keep embedding on every facet reindex, defeating the split. Reset it
	// explicitly. On an index that never had one this is a harmless no-op.
	task, err := c.facet.ResetEmbeddersWithContext(ctx)
	if err != nil {
		return fmt.Errorf("search: reset facet embedders: %w", err)
	}
	return c.awaitTask(ctx, c.facet, task.TaskUID)
}

// EnsureSemanticIndex creates the hybrid jobs index (with the in-engine embedder)
// and applies its settings. It is built by the separate reindex --semantic pass;
// querying it embeds documents, so it is kept off the default reindex path.
func (c *Client) EnsureSemanticIndex(ctx context.Context) error {
	return c.ensure(ctx, c.semantic, semanticIndexUID, semanticSettings())
}

// Rebuild is a fresh-index build session for a full reindex. Documents are streamed
// into a throwaway index (Push enqueues each batch WITHOUT waiting, so Meilisearch
// auto-batches consecutive tasks — the throughput lever), then Promote waits for the
// pushes, atomically swaps the rebuild index over the live one, and drops the old
// one. Two properties matter: search keeps serving the live index untouched until
// the single swap (no half-built window), and indexing stays fast because the
// rebuild index grows from empty instead of re-merging into an already-full one.
type Rebuild struct {
	c              *Client
	liveUID        string
	rebuildUID     string
	settings       *meilisearch.Settings
	resetEmbedders bool
	rebuild        meilisearch.IndexManager
	tasks          []int64
}

// NewFacetRebuild starts a full rebuild of the facet/keyword production index.
func (c *Client) NewFacetRebuild() *Rebuild {
	return &Rebuild{c: c, liveUID: facetIndexUID, rebuildUID: facetRebuildUID, settings: facetSettings(), resetEmbedders: true}
}

// NewSemanticRebuild starts a full rebuild of the hybrid semantic index.
func (c *Client) NewSemanticRebuild() *Rebuild {
	return &Rebuild{c: c, liveUID: semanticIndexUID, rebuildUID: semanticRebuildUID, settings: semanticSettings()}
}

// Prepare creates a fresh, empty rebuild index with this pass's settings, ready to
// receive documents. It also ensures the live index exists, since the swap in
// Promote needs both — on a first-ever run the live index is created empty and the
// swap replaces its contents and settings wholesale.
func (r *Rebuild) Prepare(ctx context.Context) error {
	if err := r.c.createIndex(ctx, r.c.manager.Index(r.liveUID), r.liveUID); err != nil {
		return err
	}
	// Discard any leftover rebuild index from an aborted prior run, then create it
	// fresh so the build always starts from empty.
	if err := r.c.dropIndex(ctx, r.rebuildUID); err != nil {
		return err
	}
	r.rebuild = r.c.manager.Index(r.rebuildUID)
	if err := r.c.ensure(ctx, r.rebuild, r.rebuildUID, r.settings); err != nil {
		return err
	}
	// The facet index carries no embedder; reset it in case a prior version left one
	// (mirrors EnsureIndex). The semantic rebuild keeps the embedder its settings add.
	if r.resetEmbedders {
		task, err := r.rebuild.ResetEmbeddersWithContext(ctx)
		if err != nil {
			return fmt.Errorf("search: reset rebuild embedders: %w", err)
		}
		return r.c.awaitTask(ctx, r.rebuild, task.TaskUID)
	}
	return nil
}

// Push enqueues a batch into the rebuild index WITHOUT waiting for it to finish —
// the task uid is kept and awaited in Promote, so Meilisearch auto-batches the
// consecutive document tasks instead of indexing each in isolation.
func (r *Rebuild) Push(ctx context.Context, docs []JobDocument) error {
	if len(docs) == 0 {
		return nil
	}
	pk := primaryKey
	task, err := r.rebuild.UpdateDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk})
	if err != nil {
		return fmt.Errorf("search: rebuild push: %w", err)
	}
	r.tasks = append(r.tasks, task.TaskUID)
	return nil
}

// Promote waits for every enqueued document batch, then atomically swaps the
// rebuild index over the live one and drops the now-old index. After this the live
// uid serves the freshly built data.
func (r *Rebuild) Promote(ctx context.Context) error {
	for _, uid := range r.tasks {
		if err := r.c.awaitTask(ctx, r.rebuild, uid); err != nil {
			return err
		}
	}
	if err := r.c.swapIndexes(ctx, r.liveUID, r.rebuildUID); err != nil {
		return err
	}
	// After the swap the old data lives under the rebuild uid; drop it.
	return r.c.dropIndex(ctx, r.rebuildUID)
}

// swapIndexes atomically swaps two indexes and waits for the swap to finish. It
// calls POST /swap-indexes directly rather than via the SDK: the pinned
// meilisearch-go always serializes a `rename` field that our engine version
// (v1.13) rejects, so the SDK's SwapIndexes is unusable here.
func (c *Client) swapIndexes(ctx context.Context, a, b string) error {
	payload := []map[string][]string{{"indexes": {a, b}}}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("search: marshal swap: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/swap-indexes", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("search: swap request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("search: swap indexes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("search: swap indexes: unexpected status %d", resp.StatusCode)
	}
	var task struct {
		TaskUID int64 `json:"taskUid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return fmt.Errorf("search: decode swap task: %w", err)
	}
	return c.awaitManagerTask(ctx, task.TaskUID)
}

// ensure creates the named index (keyed by the internal id) if absent and applies
// its settings. Shared by the facet and semantic ensure paths.
func (c *Client) ensure(ctx context.Context, idx meilisearch.IndexManager, uid string, settings *meilisearch.Settings) error {
	if err := c.createIndex(ctx, idx, uid); err != nil {
		return err
	}
	st, err := idx.UpdateSettingsWithContext(ctx, settings)
	if err != nil {
		return fmt.Errorf("search: update settings: %w", err)
	}
	return c.awaitTask(ctx, idx, st.TaskUID)
}

// createIndex creates the index (keyed by the internal id) if absent. An
// already-existing index is the idempotent happy path, not a failure.
func (c *Client) createIndex(ctx context.Context, idx meilisearch.IndexManager, uid string) error {
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
	if created.Status == meilisearch.TaskStatusFailed && created.Error.Code != "index_already_exists" {
		return fmt.Errorf("search: create index failed: %s", created.Error.Message)
	}
	return nil
}

// dropIndex deletes an index, tolerating one that does not exist (idempotent), so
// it is safe to clear a leftover rebuild index from an aborted prior run.
func (c *Client) dropIndex(ctx context.Context, uid string) error {
	task, err := c.manager.DeleteIndexWithContext(ctx, uid)
	if err != nil {
		return fmt.Errorf("search: delete index %s: %w", uid, err)
	}
	t, err := c.manager.WaitForTaskWithContext(ctx, task.TaskUID, taskPollInterval)
	if err != nil {
		return fmt.Errorf("search: await delete index %s: %w", uid, err)
	}
	if t.Status == meilisearch.TaskStatusFailed && t.Error.Code != "index_not_found" {
		return fmt.Errorf("search: delete index %s failed: %s", uid, t.Error.Message)
	}
	return nil
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

// similarSourceMissingCode is the Meilisearch error code for "the similar-query
// source id is not a document in the index". The semantic index is built
// incrementally (reindex --semantic), so a job present in Postgres can still lack
// a vector — its /similar is then "no neighbours", not an error.
const similarSourceMissingCode = "not_found_similar_id"

// SimilarJobs returns the jobs nearest to job id in embedding space, queried
// against the semantic index by the document's stored vector (no query text, no
// re-embedding). The semantic index holds open jobs only, so neighbours are open
// jobs without any added filter. Meilisearch's similar endpoint already excludes
// the source document, but we over-fetch by one and drop it defensively rather
// than depend on that — and to avoid making the primary key a filterable
// attribute just to express "id != source".
//
// A job with no vector in the semantic index yet (the index lags ingest) yields
// an empty list, not an error: Meilisearch answers such a source id with
// not_found_similar_id, which we map to "no neighbours" so the detail-page
// section simply hides.
func (c *Client) SimilarJobs(ctx context.Context, id int64, limit int) ([]JobDocument, error) {
	var resp meilisearch.SimilarDocumentResult
	err := c.semantic.SearchSimilarDocumentsWithContext(ctx, &meilisearch.SimilarDocumentQuery{
		Id:       id,
		Embedder: embedderName,
		Limit:    int64(limit) + 1,
	}, &resp)
	if err != nil {
		var meiliErr *meilisearch.Error
		if errors.As(err, &meiliErr) && meiliErr.MeilisearchApiError.Code == similarSourceMissingCode {
			return nil, nil
		}
		return nil, fmt.Errorf("search: similar: %w", err)
	}

	var hits []JobDocument
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return nil, fmt.Errorf("search: decode similar hits: %w", err)
	}

	out := make([]JobDocument, 0, limit)
	for _, h := range hits {
		if h.ID == id {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, h)
	}
	return out, nil
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

// awaitManagerTask waits for an engine-level task (one not scoped to a single
// index, e.g. swap-indexes) by polling the global task endpoint.
func (c *Client) awaitManagerTask(ctx context.Context, taskUID int64) error {
	t, err := c.manager.WaitForTaskWithContext(ctx, taskUID, taskPollInterval)
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
			"enrichment.employment_type", "enrichment.education_level", "enrichment.seniority",
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
