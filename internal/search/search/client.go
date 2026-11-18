// Package search provides Meilisearch-backed full-text and faceted keyword search
// over jobs: the document shape, the index configuration and read/write helpers. The
// index carries userProvided skill vectors for the match sort; TEI description vectors
// (embed.go) stay in Postgres (job_semantic_chunks) and never reach Meilisearch.
package search

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/MoeForge/moego/json"
	"github.com/meilisearch/meilisearch-go"
)

const (
	// facetIndexUID is the production index: keyword search, facets, and the `userProvided`
	// skill embedder behind the match sort.
	facetIndexUID = "jobs"
	// facetRebuildUID is the index a full rebuild streams into before swapping over the
	// live one (see Rebuild).
	facetRebuildUID = "jobs_rebuild"
	primaryKey      = "id"
	// embedderModel identifies the embedding model and passage shape, stored beside a job's
	// vector. Changing it marks vectors stale, so EnqueuePendingSemanticJobs re-embeds the
	// catalogue; bump it on a passage-shape change even when the model is the same.
	// Embedding runs on a self-hosted TEI service (not in-engine, not OpenAI) over its native
	// /embed route (embedderURL), which keeps it off Meilisearch's task queue.
	embedderModel = "intfloat/multilingual-e5-base-chunked-v1"
	// embedderURL is the default TEI /embed endpoint (loopback on the production host).
	// WithEmbedURL overrides it, but the backend must serve the same e5 model.
	embedderURL = "http://127.0.0.1:8090/embed"

	// maxTotalHits caps the counted results; below it estimatedTotalHits is the true filtered
	// total. Keep it well above the open-job catalogue or every count saturates at it. Deep
	// paging is bounded separately by maxPageWindow in the handler.
	maxTotalHits = 10000000

	// maxValuesPerFacet raises Meili's per-facet cap of 100. A cap below a facet's value count
	// silently drops values, so keep generous headroom.
	maxValuesPerFacet = 1200

	taskPollInterval = 50 * time.Millisecond
)

// rawClient serves raw Meilisearch calls that bypass the SDK (swap-indexes).
// http.DefaultClient has no timeout, so a hung engine would block a reindex.
var rawClient = &http.Client{Timeout: 30 * time.Second}

// Client wraps the Meilisearch service scoped to the facet job index. url/key are kept
// for the raw swap-indexes call the SDK cannot make (see swapIndexes).
type Client struct {
	manager meilisearch.ServiceManager
	facet   meilisearch.IndexManager
	url     string
	key     string
	// embedURL is the TEI /embed endpoint for jobs and CVs. Defaults to embedderURL; it must
	// serve the same e5 model.
	embedURL string
	// embedKey is the optional bearer token for embedURL (WithEmbedAPIKey).
	embedKey string
	// embedConcurrency is how many embed calls a batch runs in flight (default 1). Only a
	// remote GPU endpoint gains from more.
	embedConcurrency int
	// router sends facet requests without query text to a second engine; nil means
	// Meilisearch only (WithFacetEngine).
	router *facetRouter
}

// Option customizes a Client at construction (see NewClient).
type Option func(*Client)

// WithEmbedURL points embedding at another TEI-compatible /embed endpoint serving the
// same e5 model. An empty url keeps the default.
func WithEmbedURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.embedURL = url
		}
	}
}

// WithEmbedAPIKey sets the bearer token for the embedding endpoint; empty means none.
func WithEmbedAPIKey(key string) Option {
	return func(c *Client) { c.embedKey = key }
}

// WithEmbedConcurrency sets how many embed calls a batch runs in flight (default 1;
// remote GPU endpoints gain from more). A value below 1 keeps the default.
func WithEmbedConcurrency(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.embedConcurrency = n
		}
	}
}

// NewClient connects to Meilisearch at url with key; no I/O happens until the first
// request. Embedding defaults to embedderURL with no auth and concurrency 1; the
// WithEmbed* options override it, keeping this package env-free.
func NewClient(url, key string, opts ...Option) *Client {
	// SDK retries are off: v0.36.3 re-sends a consumed POST body, turning a transient
	// 502/503/504 into 400 missing_payload. idempotentRetryTransport retries body-less
	// requests; a failed POST surfaces its real status for workers to retry or requeue.
	m := meilisearch.New(url, meilisearch.WithAPIKey(key), meilisearch.WithCustomClient(newEngineHTTPClient()), meilisearch.DisableRetries(),
		meilisearch.WithCustomJsonMarshaler(json.Marshal), meilisearch.WithCustomJsonUnmarshaler(json.Unmarshal))
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

// EnsureIndex creates the jobs index and applies its settings, including the skill
// embedder. Idempotent.
func (c *Client) EnsureIndex(ctx context.Context) error {
	// ensure clears inherited embedders first, so the live set is exactly what
	// facetSettings declares.
	return c.ensure(ctx, c.facet, facetIndexUID, primaryKey, facetSettings())
}

// ensure creates the named index (keyed by pk) if absent and applies its settings.
// Shared by the jobs and companies indexes.
func (c *Client) ensure(ctx context.Context, idx meilisearch.IndexManager, uid, pk string, settings *meilisearch.Settings) error {
	if err := c.createIndex(ctx, idx, uid, pk); err != nil {
		return err
	}
	// Reset embedders BEFORE applying settings, never after. Meilisearch merges `embedders`
	// by key, so an index that once had a model-backed embedder would keep embedding every
	// document without the reset, while a reset afterwards deletes the embedder the settings
	// just declared.
	reset, err := idx.ResetEmbeddersWithContext(ctx)
	if err != nil {
		return fmt.Errorf("search: reset embedders on %s: %w", uid, err)
	}
	// Queue settings without awaiting the reset: the per-index task queue is serial, so
	// submission order is execution order, and awaiting would leave the index embedder-less
	// for a round trip (a match-sort query then 400s). All task ids are awaited below.
	st, err := idx.UpdateSettingsWithContext(ctx, settings)
	if err != nil {
		return fmt.Errorf("search: update settings: %w", err)
	}
	// facetSearch goes on its own: the SDK tags it omitempty, so false never goes out, and a
	// fresh index defaults to true. Nothing uses /facet-search, and while on, Meili rebuilds
	// its data on every write.
	fs, err := idx.UpdateFacetSearchWithContext(ctx, settings.FacetSearch)
	if err != nil {
		return fmt.Errorf("search: update facet search on %s: %w", uid, err)
	}
	return errors.Join(
		c.awaitTask(ctx, idx, reset.TaskUID),
		c.awaitTask(ctx, idx, st.TaskUID),
		c.awaitTask(ctx, idx, fs.TaskUID),
	)
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

// IndexJobs replaces complete documents in the facet index by primary key. Every
// producer supplies the full projection and both vector keys. Replacement clears
// omitted facets after an edit; a partial update would retain their old values.
func (c *Client) IndexJobs(ctx context.Context, docs []JobDocument) error {
	return c.indexInto(ctx, c.facet, docs)
}

func (c *Client) indexInto(ctx context.Context, idx meilisearch.IndexManager, docs []JobDocument) error {
	parts, err := ndjsonParts(docs, maxPushBytes)
	if err != nil {
		return err
	}
	pk := primaryKey
	// Submit every part before awaiting any: Meilisearch merges only additions already
	// queued, so an idle engine usually spends a separate indexing pass per part. Still far
	// cheaper than a rejected request.
	var firstErr error
	uids := make([]int64, 0, len(parts))
	for _, part := range parts {
		task, err := idx.AddDocumentsNdjsonWithContext(ctx, part, &meilisearch.DocumentOptions{PrimaryKey: &pk})
		if err != nil {
			firstErr = fmt.Errorf("search: index documents: %w", err)
			break
		}
		uids = append(uids, task.TaskUID)
	}
	// Await every accepted part even after a failure, so a caller's per-item retry
	// cannot race a part that is still pending and would land after it.
	for _, uid := range uids {
		if err := c.awaitTask(ctx, idx, uid); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// maxPushBytes keeps one documents request under Meilisearch's http_payload_size_limit
// (100 MB default).
var maxPushBytes = 64 << 20 // a var only so a test can force a split

// ndjsonParts encodes docs as NDJSON bodies of at most limit bytes each. A document
// larger than limit on its own still gets a body of its own.
func ndjsonParts(docs []JobDocument, limit int) ([][]byte, error) {
	var parts [][]byte
	var cur []byte
	for _, d := range docs {
		line, err := json.Marshal(d)
		if err != nil {
			return nil, fmt.Errorf("search: encode document %d: %w", d.ID, err)
		}
		if len(cur) > 0 && len(cur)+len(line)+1 > limit {
			parts = append(parts, cur)
			cur = nil
		}
		cur = append(append(cur, line...), '\n')
	}
	if len(cur) > 0 {
		parts = append(parts, cur)
	}
	return parts, nil
}

// SubmitJobs replaces complete documents in the live index without awaiting the task, so
// the ingest indexer never blocks the crawl on Meilisearch. Only the submission error is
// returned; a task failing server-side is reconciled by the batch reindex.
func (c *Client) SubmitJobs(ctx context.Context, docs []JobDocument) error {
	if len(docs) == 0 {
		return nil
	}
	pk := primaryKey
	if _, err := c.facet.AddDocumentsWithContext(ctx, docs, &meilisearch.DocumentOptions{PrimaryKey: &pk}); err != nil {
		return fmt.Errorf("search: submit documents: %w", err)
	}
	return nil
}

// DeleteJobs physically removes private or missing jobs from the facet index.
// Closed and duplicate public jobs use visibility markers instead. Deleting an
// id that is not indexed is a no-op, keeping retries idempotent.
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
	// LiteralQuery treats stored text as words, without the user-query DSL or
	// Meilisearch's phrase/exclusion syntax. Similar jobs pass an employer's title.
	LiteralQuery bool
	// PhraseQuery sends Query as one Meilisearch phrase: words adjacent, in order, no typo
	// tolerance ("W-2", "contract to hire"). As separate words "W-2" matches every "2".
	PhraseQuery bool
	Query       string
	Filter      any
	Sort        []string
	Limit       int
	Offset      int
	// Vector ranks results by cosine against it using SkillEmbedder instead of text
	// relevance (the match sort); it composes with Filter in one query.
	// Sort must be empty with it: an explicit sort silently overrides vector ranking.
	Vector []float32
	// QFields restricts Query matching to these searchable attributes (see
	// QFieldsFromValues). Nil means Meilisearch's default: all of them.
	QFields []string
	// Prefix lets the last word of Query match as a word prefix, for a query still being
	// typed. Off, the last word is read as complete, which is cheaper (see wholeWords).
	Prefix bool
}

// SearchResult holds the matched documents and Meilisearch's estimated total.
type SearchResult struct {
	Hits  []JobDocument
	Total int64
	// TotalIsLowerBound marks a Total that is "at least this many" rather than an estimate
	// of the whole match. Search never sets it; a caller that filters the ranking itself
	// and stops early does (the handler's min_skill_match scan).
	TotalIsLowerBound bool
}

// ErrBadQuery marks a request Meilisearch rejected as malformed (400, e.g. an unparseable
// filter value from client input). A client mistake, not an engine fault: handlers map it
// to 400 without reporting it (see handler.classify). Match with errors.Is.
var ErrBadQuery = errors.New("bad query")

// isBadRequest reports whether err is a Meilisearch API rejection with a 400
// status — the engine refusing a malformed query or filter — as opposed to a
// transport/engine failure. Such rejections are caused by the request itself.
func isBadRequest(err error) bool {
	var me *meilisearch.Error
	return errors.As(err, &me) && me.StatusCode == http.StatusBadRequest
}

// queryErr reports a query failure under the caller's op prefix. A cancelled or expired
// context arrives wrapped in a Meilisearch communication error that does not chain to
// context.Canceled (the SDK's *Error has no Unwrap), so the context error is re-raised
// directly; otherwise a client disconnect reads as a fault. A 400 is tagged ErrBadQuery.
// A free function because disjunctiveFacetCounts is one and has no client in scope.
func queryErr(ctx context.Context, op string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", op, ctxErr)
	}
	if isBadRequest(err) {
		return fmt.Errorf("%s: %w: %v", op, ErrBadQuery, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// buildSearchRequest translates SearchParams into the SDK request. Split out so the
// vector/embedder pairing is testable: a Vector without an embedder name is a 400.
func buildSearchRequest(p SearchParams) *meilisearch.SearchRequest {
	req := &meilisearch.SearchRequest{
		Filter:               p.Filter,
		Sort:                 p.Sort,
		Limit:                int64(p.Limit),
		Offset:               int64(p.Offset),
		AttributesToSearchOn: p.QFields,
	}
	if len(p.Vector) > 0 {
		req.Vector = p.Vector
		// SemanticRatio 1 is pure vector ranking; less blends in keyword score, which is noise
		// for this sort's empty query.
		req.Hybrid = &meilisearch.SearchRequestHybrid{Embedder: SkillEmbedder, SemanticRatio: 1}
	}
	return req
}

// Search runs a query against the jobs (facet/keyword) index and decodes the hits.
func (c *Client) Search(ctx context.Context, p SearchParams) (SearchResult, error) {
	p.Filter = withOpenJobs(p.Filter)
	alts := []Alternative{{Text: literalSearchText(p.Query)}}
	if p.PhraseQuery {
		alts = []Alternative{{Text: `"` + literalSearchText(p.Query) + `"`}}
	} else if !p.LiteralQuery {
		var err error
		alts, err = ParseQuery(p.Query)
		if err != nil {
			return SearchResult{}, err
		}
	}
	if !p.Prefix {
		for i := range alts {
			alts[i].Text = wholeWords(alts[i].Text)
		}
	}
	var (
		hits  meilisearch.Hits
		total int64
	)
	if len(alts) == 1 {
		p.Query = alts[0].Text
		if alts[0].Fields != nil {
			p.QFields = alts[0].Fields
		}
		resp, err := c.facet.SearchWithContext(ctx, p.Query, buildSearchRequest(p))
		if err != nil {
			return SearchResult{}, queryErr(ctx, "search: query", err)
		}
		hits, total = resp.Hits, resp.EstimatedTotalHits
	} else {
		resp, err := c.manager.MultiSearchWithContext(ctx, buildFederatedSearch(p, alts))
		if err != nil {
			return SearchResult{}, queryErr(ctx, "search: federated query", err)
		}
		hits, total = resp.Hits, resp.EstimatedTotalHits
	}

	var docs []JobDocument
	if err := hits.DecodeInto(&docs); err != nil {
		return SearchResult{}, fmt.Errorf("search: decode hits: %w", err)
	}
	return SearchResult{Hits: docs, Total: total}, nil
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
