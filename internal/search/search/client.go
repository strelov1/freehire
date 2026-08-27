// Package search provides Meilisearch-backed full-text and faceted keyword search
// over jobs. It owns the document shape and the single facet/keyword index
// configuration — the always-fresh production index, carrying one `userProvided`
// embedder for the skill vectors behind the match sort and no model-backed one — plus the
// read/write helpers, so callers (the search handler and the reindex command) never
// touch the meilisearch-go SDK directly. It also owns the TEI embedding calls (see
// embed.go) that feed the pgvector-backed job_semantic_chunks table
// (internal/ai/embed's open-job path) — not a Meilisearch document; the semantic (jobs_semantic) index
// this package used to also build and query was removed in
// openspec/changes/drop-hybrid-search-pgvector-similar in favor of a precomputed
// pgvector lookup (cmd/similar-backfill, internal/search/similarjobs).
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

	"github.com/strelov1/freehire/internal/dict/skillvec"
)

const (
	// facetIndexUID is the production search index: keyword + facets, plus the
	// `userProvided` skill embedder behind the match sort. No model is called at index
	// time — that is what kept a full rebuild ~25x faster than embedding every document
	// — but building the vector index is no longer free, so a rebuild is materially
	// slower and larger than when the index carried no embedder at all. It serves all
	// default (keyword) traffic and the facet analytics.
	facetIndexUID = "jobs"
	// facetRebuildUID is the throwaway index a full rebuild streams into before
	// atomically swapping over the live index (see Rebuild).
	facetRebuildUID = "jobs_rebuild"
	primaryKey      = "id"
	// embedderModel is the identity of the embedding model+passage-shape, stored beside a
	// job's vector so a change marks it stale and drives EnqueuePendingSemanticJobs'
	// staleness check — bumping this string is what forces a full re-embed through the
	// current pipeline (openspec/changes/drop-hybrid-search-pgvector-similar's ops step
	// 8.3), not a separate backfill tool.
	// Embedding runs on a self-hosted Text-Embeddings-Inference (TEI) service — NOT
	// in-engine and NOT OpenAI — reached over TEI's native /embed route (embedderURL).
	// Multilingual e5 gives far sharper skill matching than the old in-engine MiniLM, and
	// offloading the compute keeps it off Meilisearch's single task queue.
	// "-chunked-v1": the passage shape changed (HTML-stripped, full-length, chunked —
	// see internal/search/plaintext.go/chunk.go) even though the underlying e5 model did
	// not, so this identity must change too or a job already stamped under the OLD
	// (truncated, HTML-laden) passage would never be re-enqueued. Bumped 2026-08-14 as
	// its own scheduled deploy (openspec/changes/drop-hybrid-search-pgvector-similar's
	// ops step 8.3), triggering the full-catalogue re-embed through the chunking
	// pipeline — not bundled into the deploy that shipped the pipeline itself.
	embedderModel = "intfloat/multilingual-e5-base-chunked-v1"
	// embedderURL is the default embedding backend: the host2 TEI's native /embed route
	// (see embedChunk) — the co-located loopback TEI of the production topology. A worker
	// can override it (WithEmbedURL, wired from EMBED_URL) to point at a faster backend
	// serving the same e5 model — e.g. an HF Inference Endpoint for a bulk reindex —
	// without changing the vector space.
	embedderURL = "http://127.0.0.1:8090/embed"

	// maxTotalHits caps how high a search counts its results: below it,
	// estimatedTotalHits is the true filtered total, so it is set well above the
	// index size to keep the reported count honest. It is NOT the pagination guard
	// — deep offset paging is bounded separately by maxSearchWindow in the search
	// handler — so a large value here costs nothing beyond an accurate total.
	// Keep it comfortably above the open-job catalogue (which crossed 1M in
	// 2026-06): once the real total exceeds this cap, every count saturates at it.
	maxTotalHits = 10000000

	// maxValuesPerFacet raises the per-facet value cap above Meili's default of
	// 100 so the analytics facet distribution is not truncated for high-cardinality
	// facets (skills, countries, and especially roles — ~800 graded/named values).
	// Meili truncates to this cap ALPHABETICALLY (sortFacetValuesBy defaults to
	// "alpha") BEFORE the client sorts by count, so a cap below the value count
	// silently drops the busiest values past the alphabetical cut (e.g. "senior_*"
	// roles). Keep it comfortably above the role catalogue.
	maxValuesPerFacet = 1200

	taskPollInterval = 50 * time.Millisecond
)

// rawClient serves the package's raw Meilisearch calls that bypass the SDK (the
// swap-indexes POST). http.DefaultClient has no timeout of its own, so a hung
// engine would otherwise block a reindex indefinitely. (The TEI embed calls in
// embed.go are a different backend with their own per-attempt timeout — see
// embedAttemptTimeout.)
var rawClient = &http.Client{Timeout: 30 * time.Second}

// Client is a thin wrapper over the Meilisearch service scoped to the facet
// (keyword + facets, no embedder) job index. url/key are kept for the one raw call
// (swap-indexes) the SDK cannot make against our engine version — see swapIndexes.
type Client struct {
	manager meilisearch.ServiceManager
	facet   meilisearch.IndexManager
	url     string
	key     string
	// embedURL is the TEI native /embed endpoint this client embeds against (jobs
	// and CVs alike). It defaults to embedderURL (the host2 TEI); WithEmbedURL points a
	// worker at a faster backend — e.g. a GPU endpoint for a bulk reindex — as long as
	// it serves the SAME e5 model (same vector space). Tests set it directly.
	embedURL string
	// embedKey is the optional bearer token for embedURL (WithEmbedAPIKey). Empty for
	// the authless host2 TEI; set when pointing at an authenticated endpoint (HF, etc.).
	embedKey string
	// embedConcurrency is how many embed calls a batch runs in flight
	// (WithEmbedConcurrency, default 1). The CPU-bound host2 TEI gains nothing from
	// concurrency, but a remote GPU endpoint does (it hides per-call latency) — a bulk
	// reindex sets it high.
	embedConcurrency int
}

// Option customizes a Client at construction (see NewClient).
type Option func(*Client)

// WithEmbedURL points the embedding backend at a TEI-compatible /embed endpoint other
// than the default host2 TEI — e.g. an HF Inference Endpoint for a bulk reindex. The
// endpoint must serve the SAME e5 model (same vector space). An empty url keeps the
// default.
func WithEmbedURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.embedURL = url
		}
	}
}

// WithEmbedAPIKey sets the bearer token for the embedding endpoint. Empty (the
// default) suits the authless host2 TEI; set it when pointing at an authenticated
// endpoint (HF, etc.).
func WithEmbedAPIKey(key string) Option {
	return func(c *Client) { c.embedKey = key }
}

// WithEmbedConcurrency sets how many embed calls a batch runs in flight (default 1).
// The CPU-bound host2 TEI gains nothing from concurrency, but a remote GPU endpoint
// does (it hides per-call latency) — a bulk reindex sets it high. A value below 1
// keeps the default.
func WithEmbedConcurrency(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.embedConcurrency = n
		}
	}
}

// NewClient connects to Meilisearch at url authenticated by key. It does no I/O
// — the connection is exercised lazily by the first request (or EnsureIndex). The
// embedding backend defaults to the host2 TEI (embedderURL — the loopback TEI of the
// production topology) with no auth and concurrency 1; callers that embed override it
// via the WithEmbed* options (wired from the EMBED_* env in internal/platform/config, so this
// package stays env-free).
func NewClient(url, key string, opts ...Option) *Client {
	m := meilisearch.New(url, meilisearch.WithAPIKey(key))
	c := &Client{
		manager:          m,
		facet:            m.Index(facetIndexUID),
		url:              url,
		key:              key,
		embedURL:         embedderURL,
		embedConcurrency: 1,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// EnsureIndex creates the facet/keyword jobs index and applies its settings,
// including the userProvided skill embedder behind the match sort. It is idempotent — safe to call on every reindex. This is the fast
// production index that all default (keyword) traffic and faceting hit.
func (c *Client) EnsureIndex(ctx context.Context) error {
	// ensure() clears inherited embedders before applying these settings, so the live
	// set ends up as exactly the one facetSettings declares — the skill embedder and
	// nothing else. See ensure() for why the order is load-bearing.
	return c.ensure(ctx, c.facet, facetIndexUID, primaryKey, facetSettings())
}

// Rebuild is a fresh-index build session for a full reindex. Documents are streamed
// into a throwaway index (Push enqueues each batch WITHOUT waiting, so Meilisearch
// auto-batches consecutive tasks — the throughput lever), then Promote waits for the
// pushes, atomically swaps the rebuild index over the live one, and drops the old
// one. Two properties matter: search keeps serving the live index untouched until
// the single swap (no half-built window), and indexing stays fast because the
// rebuild index grows from empty instead of re-merging into an already-full one.
type Rebuild struct {
	c          *Client
	liveUID    string
	rebuildUID string
	settings   *meilisearch.Settings
	rebuild    meilisearch.IndexManager
	tasks      []int64
}

// NewFacetRebuild starts a full rebuild of the facet/keyword production index.
func (c *Client) NewFacetRebuild() *Rebuild {
	return &Rebuild{c: c, liveUID: facetIndexUID, rebuildUID: facetRebuildUID, settings: facetSettings()}
}

// Prepare creates a fresh, empty rebuild index with this pass's settings, ready to
// receive documents. It also ensures the live index exists, since the swap in
// Promote needs both — on a first-ever run the live index is created empty and the
// swap replaces its contents and settings wholesale.
func (r *Rebuild) Prepare(ctx context.Context) error {
	if err := r.c.createIndex(ctx, r.c.manager.Index(r.liveUID), r.liveUID, primaryKey); err != nil {
		return err
	}
	// Discard any leftover rebuild index from an aborted prior run, then create it
	// fresh so the build always starts from empty.
	if err := r.c.dropIndex(ctx, r.rebuildUID); err != nil {
		return err
	}
	r.rebuild = r.c.manager.Index(r.rebuildUID)
	// ensure() clears inherited embedders first, so the rebuild index carries exactly
	// the embedders these settings declare — which is how the skill embedder reaches
	// production at all.
	return r.c.ensure(ctx, r.rebuild, r.rebuildUID, primaryKey, r.settings)
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

// Cleanup drops the rebuild index, tolerating its absence. reindexFull defers it so a
// run that aborts before Promote (whose swap-and-drop is the normal teardown) does not
// leave an orphan rebuild index — which otherwise eats ~index-size of disk until the
// next run's Prepare clears it. Idempotent.
func (r *Rebuild) Cleanup(ctx context.Context) error {
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
	resp, err := rawClient.Do(req)
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

// ensure creates the named index (keyed by pk) if absent and applies its settings.
// Shared by every ensure/prepare path — the jobs indexes (id-keyed) and the companies
// index (slug-keyed, see company.go) differ only in uid, primary key, and settings.
func (c *Client) ensure(ctx context.Context, idx meilisearch.IndexManager, uid, pk string, settings *meilisearch.Settings) error {
	if err := c.createIndex(ctx, idx, uid, pk); err != nil {
		return err
	}
	// Reset embedders BEFORE applying the settings, never after.
	//
	// Meilisearch merges settings updates, and it merges `embedders` BY KEY: an update
	// naming one embedder leaves any other in place. So the reset is still needed — an
	// index that a prior version gave a model-backed embedder would otherwise keep
	// embedding every document forever — but doing it afterwards deletes the embedder
	// the settings just declared. That is exactly what used to happen here, and it made
	// a rebuild ship an index with no skill embedder while every unit test still passed.
	//
	// Ordering it first means the live embedder set ends up as EXACTLY what `settings`
	// declares: nothing inherited, nothing dropped. An index with no embedder is
	// unaffected either way.
	reset, err := idx.ResetEmbeddersWithContext(ctx)
	if err != nil {
		return fmt.Errorf("search: reset embedders on %s: %w", uid, err)
	}
	// Queue the settings WITHOUT awaiting the reset. Meilisearch runs one serial task
	// queue per index, so submission order IS execution order — the reset still lands
	// first. Awaiting it here instead would leave a live index embedder-less for the
	// round trip between the two, and a match-sort query arriving in that window gets a
	// 400. Both task ids are awaited below, in the order they were queued.
	st, err := idx.UpdateSettingsWithContext(ctx, settings)
	if err != nil {
		return fmt.Errorf("search: update settings: %w", err)
	}
	if err := c.awaitTask(ctx, idx, reset.TaskUID); err != nil {
		return err
	}
	return c.awaitTask(ctx, idx, st.TaskUID)
}

// createIndex creates the index (keyed by pk) if absent. An already-existing index is
// the idempotent happy path, not a failure.
func (c *Client) createIndex(ctx context.Context, idx meilisearch.IndexManager, uid, pk string) error {
	create, err := c.manager.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{
		Uid:        uid,
		PrimaryKey: pk,
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

// SubmitJobs upserts a batch into the live facet index WITHOUT awaiting the
// indexing task. The incremental ingest indexer uses this instead of IndexJobs so
// it never blocks the crawl on Meilisearch's per-batch indexing (which the reindex
// passes do await); Meilisearch auto-batches the submitted tasks and processes them
// even after the worker exits. The submission (HTTP) error is returned and tallied;
// the async task result is not awaited — best-effort, the batch reindex reconciles
// a task that fails server-side.
func (c *Client) SubmitJobs(ctx context.Context, docs []JobDocument) error {
	if len(docs) == 0 {
		return nil
	}
	pk := primaryKey
	if _, err := c.facet.UpdateDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk}); err != nil {
		return fmt.Errorf("search: submit documents: %w", err)
	}
	return nil
}

// DeleteJobs removes documents from the facet index by primary key. Used by
// reindex to drop closed jobs; deleting an id that is not indexed is a no-op,
// keeping re-runs idempotent.
func (c *Client) DeleteJobs(ctx context.Context, ids []int64) error {
	return c.deleteFrom(ctx, c.facet, ids)
}

func (c *Client) deleteFrom(ctx context.Context, idx meilisearch.IndexManager, ids []int64) error {
	task, err := c.submitDelete(ctx, idx, ids)
	if err != nil || task == 0 {
		return err
	}
	return c.awaitTask(ctx, idx, task)
}

// SubmitJobDeletion enqueues a facet-index deletion WITHOUT waiting for it.
//
// Meilisearch runs one task per index at a time and a delete-by-id rebuilds the
// affected parts of the inverted index, so its cost tracks the size of the index
// rather than the number of ids: measured on prod, one batch took minutes while the
// database sat idle waiting for it. A bulk campaign that awaits every batch spends
// almost all of its time not deleting.
//
// The trade this accepts is real and visible: until the tasks drain, search serves
// documents whose rows are gone and those results 404. Use it only for a bulk pass
// that ends in a full reindex, never on a path a user waits on.
func (c *Client) SubmitJobDeletion(ctx context.Context, ids []int64) error {
	_, err := c.submitDelete(ctx, c.facet, ids)
	return err
}

// submitDelete enqueues the deletion and returns its task id, or 0 when there was
// nothing to delete. Deleting an id that is not indexed is a no-op, so re-runs stay
// idempotent.
func (c *Client) submitDelete(ctx context.Context, idx meilisearch.IndexManager, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = strconv.FormatInt(id, 10)
	}
	task, err := idx.DeleteDocumentsWithContext(ctx, keys, nil)
	if err != nil {
		return 0, fmt.Errorf("search: delete documents: %w", err)
	}
	return task.TaskUID, nil
}

// SearchParams is a backend-agnostic search request. Filter is the value built
// by Filter (nil for none).
type SearchParams struct {
	Query  string
	Filter any
	Sort   []string
	Limit  int
	Offset int
	// Vector, when set, ranks results by cosine against it using SkillEmbedder rather
	// than by text relevance — the match sort. It composes with Filter: the engine
	// applies both in one query, so the facets need no separate pass and pagination
	// stays ordinary.
	//
	// Sort must be empty when this is set. An explicit sort directive takes precedence
	// over vector ranking in Meilisearch, so sending both silently discards the match
	// order rather than erroring.
	Vector []float32
}

// SearchResult holds the matched documents and Meilisearch's estimated total.
type SearchResult struct {
	Hits  []JobDocument
	Total int64
}

// ErrBadQuery marks a search request Meilisearch rejected as malformed — a 400,
// e.g. an unparseable filter value passed through from client input. It is a
// client mistake, not an engine fault, so handlers map it to 400 and skip Sentry
// (see internal/api/handler.classify). Callers match it with errors.Is.
var ErrBadQuery = errors.New("bad query")

// isBadRequest reports whether err is a Meilisearch API rejection with a 400
// status — the engine refusing a malformed query or filter — as opposed to a
// transport/engine failure. Such rejections are caused by the request itself.
func isBadRequest(err error) bool {
	var me *meilisearch.Error
	return errors.As(err, &me) && me.StatusCode == http.StatusBadRequest
}

// buildSearchRequest translates SearchParams into the SDK's request. It is split out
// of Search so the vector/hybrid pairing is unit-testable without a live engine: a
// Vector sent without an embedder name is a 400, so the two must never drift apart.
func buildSearchRequest(p SearchParams) *meilisearch.SearchRequest {
	req := &meilisearch.SearchRequest{
		Filter: p.Filter,
		Sort:   p.Sort,
		Limit:  int64(p.Limit),
		Offset: int64(p.Offset),
	}
	if len(p.Vector) > 0 {
		req.Vector = p.Vector
		// SemanticRatio 1.0 asks for pure vector ranking. Anything less blends in the
		// keyword score, which for the empty query this sort runs under is noise.
		req.Hybrid = &meilisearch.SearchRequestHybrid{Embedder: SkillEmbedder, SemanticRatio: 1}
	}
	return req
}

// Search runs a query against the jobs (facet/keyword) index and decodes the hits.
func (c *Client) Search(ctx context.Context, p SearchParams) (SearchResult, error) {
	resp, err := c.facet.SearchWithContext(ctx, p.Query, buildSearchRequest(p))
	if err != nil {
		// A cancelled/expired context surfaces here wrapped in a Meilisearch
		// communication error that does NOT chain to context.Canceled (the SDK's
		// *Error has no Unwrap), so re-raise the context sentinel directly to keep
		// errors.Is working upstream — a client disconnect must not read as a fault.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return SearchResult{}, fmt.Errorf("search: query: %w", ctxErr)
		}
		if isBadRequest(err) {
			return SearchResult{}, fmt.Errorf("search: query: %w: %v", ErrBadQuery, err)
		}
		return SearchResult{}, fmt.Errorf("search: query: %w", err)
	}

	var hits []JobDocument
	if err := resp.Hits.DecodeInto(&hits); err != nil {
		return SearchResult{}, fmt.Errorf("search: decode hits: %w", err)
	}
	return SearchResult{Hits: hits, Total: resp.EstimatedTotalHits}, nil
}

// CurrentEmbedderModel is the identity of the embedder currently embedding jobs.
// Persisted alongside a CV vector so a model change marks the vector stale.
func CurrentEmbedderModel() string { return embedderModel }

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
// configuration. Indexing into it costs no per-document embedding, so a full
// rebuild is fast.
func facetSettings() *meilisearch.Settings {
	return &meilisearch.Settings{
		SearchableAttributes: []string{"title", "company", "description", "location"},
		// Enrichment facets are nested, so they are filtered via dot paths. The
		// resolved geography facet (regions/countries), work_mode, and skills are
		// served top-level — the union of parsed-location/column and enrichment
		// values — so they are filtered on a bare attribute, not the enrichment
		// dot path.
		FilterableAttributes: []string{
			// id backs the swipe deck's `id NOT IN [...]` per-user exclusion. Adding
			// a new filterable attribute needs a reindex before it takes effect —
			// reindex first on deploy, or /jobs 500s on the new attribute.
			"id",
			"source", "company_slug",
			// company_slug_folded backs the ingest-time coverage gate only (never a user
			// facet): it is what lets that gate match an employer the aggregator and the
			// ATS spell differently. See JobDocument.CompanySlugFolded.
			"company_slug_folded",
			"work_mode", "regions", "countries", "cities", "skills", "collections",
			// is_tech is the served top-level tech/non-tech facet (jobview), filtered on
			// the bare attribute; an unknown value is absent so it filters as empty.
			"is_tech",
			// roles is derived at index time (roletag) and served top-level like the
			// other bare facets, so it filters on the plain attribute, not a dot path.
			"roles",
			// ai_archetype is derived at index time (aiarchetype) from skills+category,
			// same pattern as roles.
			"ai_archetype",
			// role_type is derived at index time (roletype) from the title alone, same
			// pattern again — no jobs column, so a reindex is what reaches existing
			// postings. Adding a filterable attribute is the one hazardous part of
			// shipping this: until the LIVE index declares it, a binary that requests
			// the facet hard-500s /api/v1/jobs/facets for every caller, so the settings
			// patch goes out BEFORE the binary flips.
			"role_type",
			"enrichment.employment_type", "enrichment.education_level", "enrichment.seniority",
			"enrichment.category", "enrichment.domains",
			"enrichment.company_type", "enrichment.company_size", "enrichment.visa_sponsorship",
			"enrichment.salary_currency", "enrichment.salary_period",
			"enrichment.salary_min", "enrichment.salary_max", "enrichment.experience_years_min",
			"enrichment.relocation", "enrichment.english_level", "enrichment.posting_language",
			// posted_ts is the effective posting date in unix seconds — the numeric
			// field the "posted within N days" range filter needs (Meili range operators
			// require a number; the string posted_at below is sort-only).
			"posted_ts",
			// reality.class is the job-reality signal (fresh/stale/likely-evergreen),
			// nested under the served reality object; the "hide likely-evergreen" filter
			// matches on this dot path.
			"reality.class",
		},
		// posted_at / created_at are RFC3339 UTC strings and sort chronologically as text.
		SortableAttributes: []string{"posted_at", "created_at", "enrichment.salary_min", "enrichment.salary_max"},
		// posted_ts:desc is a freshness tie-breaker appended AFTER exactness: relevance
		// (and any explicit sort) always decides first, and among results otherwise tied
		// on every relevance rule the fresher posting wins. It uses the numeric
		// effective-posting field (posted_ts, unix seconds) — the reliable date jobview
		// derives, not the raw posted_at — and needs no sortable declaration (custom
		// ranking rules are independent of SortableAttributes).
		RankingRules: []string{"words", "sort", "typo", "proximity", "attribute", "exactness", "posted_ts:desc"},
		// The match sort's embedder. `userProvided` means Meilisearch never calls a
		// model: the vectors are arithmetic over a finite dictionary
		// (internal/dict/skillvec), written by the indexers into each document's
		// reserved `_vectors` object.
		//
		// Dimensions is the registry's declared width and CANNOT change without a full
		// rebuild — until one completes, the index rejects every document carrying the
		// new width. Adding this embedder to a live index is itself a rebuild-only
		// operation, and it must land in the LIVE settings BEFORE a binary that sends a
		// query vector: a vector search against an index with no such embedder is a 400,
		// which surfaces as a failing /jobs/search for everyone who picked the sort.
		// Same ordering hazard role_type documents below.
		//
		// BinaryQuantized is deliberately absent (false). These vectors carry 2-12
		// non-zeros out of 749, and a sign-bit quantiser leaves 749 bits in which nearly
		// every zero agrees across every document: measured recall@20 collapsed from 95%
		// to 10%, and the rare skills the weighting exists to surface disappeared
		// entirely.
		Embedders: map[string]meilisearch.Embedder{
			SkillEmbedder: {
				Source:     meilisearch.UserProvidedEmbedderSource,
				Dimensions: skillvec.Dimensions,
			},
		},
		// Typo tolerance is left at Meilisearch's defaults (on, with sensible min
		// word sizes). We deliberately do not send a TypoTolerance struct: the SDK
		// always serializes newer fields (e.g. disableOnNumbers) that older
		// Meilisearch versions reject, and the spec only requires typo tolerance to
		// exist, not specific thresholds. Re-add explicit tuning when the pinned
		// server and SDK fields align.
		Pagination: &meilisearch.Pagination{MaxTotalHits: maxTotalHits},
		// Raise the per-facet value cap above Meili's default of 100 so the
		// distribution is not truncated for high-cardinality facets. And keep the
		// TOP values BY COUNT, not alphabetically: cities has far more than
		// maxValuesPerFacet distinct values, so with the default alpha ordering Meili
		// drops the busiest cities that sort late (e.g. "Florianópolis", 1000+ jobs)
		// from the distribution even though filtering by them works — client-side
		// count sorting can't recover a value the engine never returned. `count`
		// ordering makes the cap keep the most-common values and also sheds the
		// long-tail noise (zip fragments, one-off strings). sortFacetValuesBy is a
		// query-time setting supported since Meili v0.28, so unlike the TypoTolerance
		// note above it is safe on the pinned server.
		Faceting: &meilisearch.Faceting{
			MaxValuesPerFacet: maxValuesPerFacet,
			SortFacetValuesBy: map[string]meilisearch.SortFacetType{"*": meilisearch.SortFacetTypeCount},
		},
		// byAttribute skips computing exact word-to-word distance across the index —
		// a local benchmark showed Meilisearch's "merging word proximity" indexing
		// phase alone costing up to ~10s per 200-document batch (search-drain's push
		// size), backed by a wordPairProximityDocids structure larger than the
		// documents themselves. Job descriptions are long-form prose, not short
		// phrases where word adjacency is load-bearing for ranking, so the relevancy
		// trade-off is negligible. NOTE: prefixSearch was deliberately NOT disabled
		// here despite being the other half of that same benchmark's savings —
		// HeaderSearch.svelte and the /jobs list's filters.ts both debounce a
		// query-as-you-type search through this same index, relying on Meilisearch's
		// default last-word prefix matching to return results mid-word.
		//
		// Like FilterableAttributes above, this setting only takes effect on data
		// written AFTER a reindex — a full `cmd/reindex` run is required post-deploy.
		ProximityPrecision: meilisearch.ByAttribute,
	}
}
